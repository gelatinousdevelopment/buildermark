package handler

import (
	"log"
	"math"
	"net/http"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// CommitConversationLinks holds the bidirectional mapping between commit hashes
// and conversation IDs for a project.
type CommitConversationLinks struct {
	CommitToConversations map[string][]string `json:"commitToConversations"`
	ConversationToCommits map[string][]string `json:"conversationToCommits"`
}

func (s *Server) handleGetCommitConversationLinks(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	commitHashesRaw := strings.TrimSpace(r.URL.Query().Get("commitHashes"))
	conversationIDsRaw := strings.TrimSpace(r.URL.Query().Get("conversationIds"))

	if commitHashesRaw == "" {
		writeError(w, http.StatusBadRequest, "commitHashes query parameter is required")
		return
	}

	commitHashes := splitCSV(commitHashesRaw)
	if len(commitHashes) > 200 {
		commitHashes = commitHashes[:200]
	}

	var conversationIDFilter map[string]bool
	if conversationIDsRaw != "" {
		ids := splitCSV(conversationIDsRaw)
		if len(ids) > 200 {
			ids = ids[:200]
		}
		conversationIDFilter = make(map[string]bool, len(ids))
		for _, id := range ids {
			conversationIDFilter[id] = true
		}
	}

	// Resolve the project group to get all related project IDs.
	groups, err := listAllProjectGroups(r.Context(), s.DB)
	if err != nil {
		log.Printf("error listing project groups: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	group, ok := findProjectGroupByProjectID(groups, projectID)
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	pIDs := projectIDs(group)

	// Load commits with diff content (only those with agent attribution).
	commits, err := db.ListCommitsWithDiffByHashes(r.Context(), s.DB, pIDs, commitHashes)
	if err != nil {
		log.Printf("error loading commits with diff: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load commits")
		return
	}

	if len(commits) == 0 {
		writeSuccess(w, http.StatusOK, &CommitConversationLinks{
			CommitToConversations: map[string][]string{},
			ConversationToCommits: map[string][]string{},
		})
		return
	}

	// Compute overall time window across all loaded commits.
	var minAuthoredAt, maxAuthoredAt int64
	minAuthoredAt = math.MaxInt64
	for _, c := range commits {
		if c.AuthoredAt < minAuthoredAt {
			minAuthoredAt = c.AuthoredAt
		}
		if c.AuthoredAt > maxAuthoredAt {
			maxAuthoredAt = c.AuthoredAt
		}
	}
	overallMinTs := minAuthoredAt*1000 - defaultMessageWindowMs
	overallMaxTs := maxAuthoredAt*1000 + commitWindowLookaheadMs

	// Load messages with extracted diffs for the time window.
	messages, err := listDerivedDiffMessages(r.Context(), s.DB, pIDs, overallMinTs, overallMaxTs)
	if err != nil {
		log.Printf("error loading derived diff messages: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load messages")
		return
	}

	ignorePatterns := groupIgnoreDiffPatterns(group)

	// Build bidirectional maps using diff-matching attribution.
	c2c := make(map[string][]string)
	c2commit := make(map[string][]string)

	for _, commit := range commits {
		tokens := parseUnifiedDiffTokens(commit.DiffContent, ignorePatterns)
		if len(tokens) == 0 {
			continue
		}

		windowStart := commit.AuthoredAt*1000 - defaultMessageWindowMs
		windowEnd := commit.AuthoredAt*1000 + commitWindowLookaheadMs

		contribs, _, _, _, _ := attributeCommitToMessages(tokens, messages, windowStart, windowEnd)

		// Extract unique conversation IDs from contributions.
		seen := make(map[string]bool)
		for _, contrib := range contribs {
			if seen[contrib.ConversationID] {
				continue
			}
			seen[contrib.ConversationID] = true

			// Apply optional conversation ID filter.
			if conversationIDFilter != nil && !conversationIDFilter[contrib.ConversationID] {
				continue
			}

			c2c[commit.CommitHash] = append(c2c[commit.CommitHash], contrib.ConversationID)
			c2commit[contrib.ConversationID] = append(c2commit[contrib.ConversationID], commit.CommitHash)
		}
	}

	writeSuccess(w, http.StatusOK, &CommitConversationLinks{
		CommitToConversations: c2c,
		ConversationToCommits: c2commit,
	})
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
