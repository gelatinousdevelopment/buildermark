package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}
	if limit > 1000 {
		limit = 1000
	}
	hiddenOnly := strings.TrimSpace(r.URL.Query().Get("hidden")) == "true"

	conversations, err := db.ListConversations(r.Context(), s.DB, limit, hiddenOnly)
	if err != nil {
		log.Printf("error listing conversations: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list conversations")
		return
	}
	writeSuccess(w, http.StatusOK, conversations)
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "conversation id is required")
		return
	}

	conv, err := db.GetConversationDetail(r.Context(), s.DB, id)
	if err != nil {
		log.Printf("error getting conversation: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get conversation")
		return
	}
	if conv == nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}

	writeSuccess(w, http.StatusOK, conv)
}

func (s *Server) handleGetConversationsBatchDetail(w http.ResponseWriter, r *http.Request) {
	idsRaw := strings.TrimSpace(r.URL.Query().Get("ids"))
	if idsRaw == "" {
		writeError(w, http.StatusBadRequest, "ids query parameter is required")
		return
	}
	ids := strings.Split(idsRaw, ",")
	if len(ids) > 200 {
		ids = ids[:200]
	}

	details, err := db.GetConversationsBatchDetail(r.Context(), s.DB, ids)
	if err != nil {
		log.Printf("error getting batch conversation detail: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get conversation details")
		return
	}
	writeSuccess(w, http.StatusOK, details)
}

func (s *Server) handleSetConversationHidden(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "conversation id is required")
		return
	}

	var body struct {
		Hidden bool `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	conv, err := db.GetConversation(r.Context(), s.DB, id)
	if err != nil {
		log.Printf("error reading conversation: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load conversation")
		return
	}
	if conv == nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if conv.Hidden == body.Hidden {
		writeSuccess(w, http.StatusOK, map[string]any{
			"conversationId": conv.ID,
			"hidden":         conv.Hidden,
			"queued":         false,
		})
		return
	}

	if err := db.SetConversationHidden(r.Context(), s.DB, id, body.Hidden); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		log.Printf("error setting conversation hidden: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update conversation visibility")
		return
	}
	s.commitDetailCache.clearProject(conv.ProjectID)

	queued := s.enqueueConversationVisibilityRecompute(*conv, body.Hidden)
	writeSuccess(w, http.StatusAccepted, map[string]any{
		"conversationId": conv.ID,
		"hidden":         body.Hidden,
		"queued":         queued,
	})
}

func (s *Server) tryStartConversationVisibilityRecompute(conversationID string) bool {
	return s.visibilityJobs.tryStart(conversationID)
}

func (s *Server) finishConversationVisibilityRecompute(conversationID string) {
	s.visibilityJobs.finish(conversationID)
}

func (s *Server) enqueueConversationVisibilityRecompute(conv db.Conversation, hidden bool) bool {
	if !s.tryStartConversationVisibilityRecompute(conv.ID) {
		return false
	}

	go func() {
		defer s.finishConversationVisibilityRecompute(conv.ID)

		broadcast := func(state, message string) {
			if s.ws == nil {
				return
			}
			s.ws.broadcastEvent("job_status", jobStatusEvent{
				JobType: "diff_recompute",
				State:   state,
				Message: message,
			})
		}

		action := "Hiding"
		if !hidden {
			action = "Unhiding"
		}
		broadcast("running", fmt.Sprintf("%s conversation %s: finding affected commits...", action, conv.ID))

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()

		hashes, err := s.commitsToRecomputeForConversationVisibility(ctx, &conv, hidden)
		if err != nil {
			log.Printf("error finding commits for visibility update: %v", err)
			broadcast("error", fmt.Sprintf("%s conversation %s failed while finding commits", action, conv.ID))
			return
		}
		if len(hashes) == 0 {
			s.commitDetailCache.clearProject(conv.ProjectID)
			broadcast("complete", fmt.Sprintf("%s conversation %s: no affected commits", action, conv.ID))
			return
		}

		lastProgress := time.Time{}
		progress := func(message string, processed int) {
			now := time.Now()
			if now.Sub(lastProgress) < 50*time.Millisecond {
				return
			}
			lastProgress = now
			broadcast("running", message)
		}

		recomputed, err := s.recomputeCommitCoverageForHashes(ctx, conv.ProjectID, hashes, progress)
		if err != nil {
			log.Printf("error recomputing commit coverage: %v", err)
			broadcast("error", fmt.Sprintf("%s conversation %s failed while recomputing coverage", action, conv.ID))
			return
		}
		s.commitDetailCache.clearProject(conv.ProjectID)
		broadcast("complete", fmt.Sprintf("%s conversation %s: recomputed %d commit(s)", action, conv.ID, recomputed))
	}()
	return true
}

func (s *Server) commitsToRecomputeForConversationVisibility(
	ctx context.Context,
	conv *db.Conversation,
	newHidden bool,
) ([]string, error) {
	recomputeProjectID := conv.ProjectID
	groups, err := listAllProjectGroups(ctx, s.DB)
	if err == nil {
		if group, ok := findProjectGroupByProjectID(groups, conv.ProjectID); ok {
			if repoProject, repoErr := resolveRepoProject(ctx, group); repoErr == nil && repoProject != nil {
				recomputeProjectID = repoProject.ID
			}
		}
	}

	tsRows, err := s.DB.QueryContext(ctx,
		`SELECT timestamp, content, raw_json
		 FROM messages
		 WHERE conversation_id = ? AND role = 'agent'
		 ORDER BY timestamp, id`,
		conv.ID,
	)
	if err != nil {
		return nil, err
	}
	defer tsRows.Close()

	minTs := int64(0)
	maxTs := int64(0)
	for tsRows.Next() {
		var ts int64
		var content, rawJSON string
		if err := tsRows.Scan(&ts, &content, &rawJSON); err != nil {
			return nil, err
		}
		_, ok := agent.ExtractReliableDiff(content)
		if !ok {
			_, ok = agent.ExtractReliableDiffFromJSON(rawJSON)
		}
		if !ok {
			continue
		}
		if minTs == 0 || ts < minTs {
			minTs = ts
		}
		if ts > maxTs {
			maxTs = ts
		}
	}
	if err := tsRows.Err(); err != nil {
		return nil, err
	}
	if minTs == 0 || maxTs == 0 {
		return nil, nil
	}

	commitStartSec := (minTs - defaultMessageWindowMs) / 1000
	commitEndSec := (maxTs + commitWindowLookaheadMs + 999) / 1000
	if commitStartSec < 0 {
		commitStartSec = 0
	}

	candidateRows, err := s.DB.QueryContext(ctx,
		`SELECT DISTINCT commit_hash
		 FROM commits
		 WHERE project_id = ? AND authored_at BETWEEN ? AND ?
		 ORDER BY authored_at DESC`,
		recomputeProjectID, commitStartSec, commitEndSec,
	)
	if err != nil {
		return nil, err
	}
	defer candidateRows.Close()

	candidates := make([]string, 0, 16)
	for candidateRows.Next() {
		var hash string
		if err := candidateRows.Scan(&hash); err != nil {
			return nil, err
		}
		candidates = append(candidates, hash)
	}
	if err := candidateRows.Err(); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// Unhiding can create fresh matches, so recompute all candidate commits.
	if !newHidden {
		return candidates, nil
	}

	matched, err := s.listCommitHashesMatchedToConversation(ctx, recomputeProjectID, conv.ID, candidates, commitStartSec, commitEndSec)
	if err != nil {
		return nil, err
	}
	return matched, nil
}

func (s *Server) listCommitHashesMatchedToConversation(
	ctx context.Context,
	projectID, conversationID string,
	candidateHashes []string,
	minAuthoredAtSec, maxAuthoredAtSec int64,
) ([]string, error) {
	groups, err := listAllProjectGroups(ctx, s.DB)
	if err != nil {
		return nil, err
	}
	group, ok := findProjectGroupByProjectID(groups, projectID)
	if !ok {
		return nil, nil
	}

	commits, err := db.ListCommitsWithDiffByHashes(ctx, s.DB, projectIDs(group), candidateHashes)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, nil
	}

	overallMinTs := minAuthoredAtSec*1000 - defaultMessageWindowMs
	overallMaxTs := maxAuthoredAtSec*1000 + commitWindowLookaheadMs
	if overallMinTs <= 0 || overallMaxTs <= 0 {
		minAt := int64(math.MaxInt64)
		maxAt := int64(0)
		for _, c := range commits {
			if c.AuthoredAt < minAt {
				minAt = c.AuthoredAt
			}
			if c.AuthoredAt > maxAt {
				maxAt = c.AuthoredAt
			}
		}
		overallMinTs = minAt*1000 - defaultMessageWindowMs
		overallMaxTs = maxAt*1000 + commitWindowLookaheadMs
	}

	messages, err := listDerivedDiffMessagesWithHidden(ctx, s.DB, projectIDs(group), overallMinTs, overallMaxTs, true)
	if err != nil {
		return nil, err
	}

	ignorePatterns := groupIgnoreDiffPatterns(group)
	matched := make([]string, 0, len(commits))
	for _, commit := range commits {
		tokens := parseUnifiedDiffTokens(commit.DiffContent, ignorePatterns)
		if len(tokens) == 0 {
			continue
		}
		windowStart := commit.AuthoredAt*1000 - defaultMessageWindowMs
		windowEnd := commit.AuthoredAt*1000 + commitWindowLookaheadMs
		contribs, _, _, _ := attributeCommitToMessages(tokens, messages, windowStart, windowEnd)
		found := false
		for _, contrib := range contribs {
			if contrib.ConversationID == conversationID {
				found = true
				break
			}
		}
		if found {
			matched = append(matched, commit.CommitHash)
		}
	}
	return matched, nil
}

func (s *Server) recomputeCommitCoverageForHashes(
	ctx context.Context,
	projectID string,
	hashes []string,
	progress func(message string, processed int),
) (int, error) {
	if len(hashes) == 0 {
		return 0, nil
	}
	unique := make([]string, 0, len(hashes))
	seen := make(map[string]bool, len(hashes))
	for _, h := range hashes {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		unique = append(unique, h)
	}
	hashes = unique
	if len(hashes) == 0 {
		return 0, nil
	}
	project, err := getProjectByID(ctx, s.DB, projectID)
	if err != nil {
		return 0, err
	}
	if project == nil {
		return 0, db.ErrNotFound
	}

	groups, err := listAllProjectGroups(ctx, s.DB)
	if err != nil {
		return 0, err
	}
	group, ok := findProjectGroupByProjectID(groups, project.ID)
	if !ok {
		return 0, fmt.Errorf("project group not found")
	}
	repoProject, err := resolveRepoProject(ctx, group)
	if err != nil {
		return 0, err
	}

	identity, _ := resolveGitIdentity(ctx, repoProject.Path)
	extraEmails := s.loadExtraLocalUserEmails()
	return recomputeCommitCoverageForProjectHashes(ctx, s.DB, repoProject, group, hashes, progress, &identity, extraEmails)
}
