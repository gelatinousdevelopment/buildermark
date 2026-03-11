package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/handler/cloudimport"
)

func (s *Server) handleCheckConversationURL(w http.ResponseWriter, r *http.Request) {
	url := strings.TrimSpace(r.URL.Query().Get("url"))
	if url == "" {
		writeError(w, http.StatusBadRequest, "url query parameter is required")
		return
	}

	var id string
	err := s.DB.QueryRowContext(r.Context(), "SELECT id FROM conversations WHERE url = ?", url).Scan(&id)
	if err == sql.ErrNoRows {
		writeSuccess(w, http.StatusOK, map[string]any{
			"imported": false,
		})
		return
	}
	if err != nil {
		log.Printf("error checking conversation URL: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to check conversation URL")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{
		"imported":       true,
		"conversationId": id,
	})
}

type webConversationImportRequest struct {
	URL       string             `json:"url"`
	Agent     string             `json:"agent"`
	Title     string             `json:"title"`
	StartedAt int64              `json:"startedAt"`
	EndedAt   int64              `json:"endedAt"`
	RepoURL   string             `json:"repoUrl"`
	Messages  []webImportMessage `json:"messages"`

	// Cloud event fields (used when agent is "claude_cloud").
	SessionID string            `json:"sessionId"`
	Events    []json.RawMessage `json:"events"`

	// CloudData holds the full JSON response from the cloud API (used by
	// the generic browser extension interceptor for both Claude and Codex).
	CloudData json.RawMessage `json:"cloudData"`
}

type webImportMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
	Model     string `json:"model"`
}

func (s *Server) handleImportWebConversation(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var body webConversationImportRequest
	if err := json.Unmarshal(rawBody, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	body.URL = strings.TrimSpace(body.URL)
	body.Agent = strings.TrimSpace(body.Agent)

	if body.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	if body.Agent == "" {
		writeError(w, http.StatusBadRequest, "agent is required")
		return
	}
	if len(body.Events) == 0 && len(body.Messages) == 0 && len(body.CloudData) == 0 {
		writeError(w, http.StatusBadRequest, "messages are required")
		return
	}

	if err := db.InsertImportLog(r.Context(), s.DB, db.ImportLog{
		Type:      "web",
		Source:    importLogSourceFromAgent(body.Agent),
		SourceID:  body.URL,
		Timestamp: time.Now().UnixMilli(),
		Content:   string(rawBody),
	}); err != nil {
		log.Printf("error inserting import log: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to log import")
		return
	}

	// Cloud data path: the generic browser extension sends cloudData.
	if len(body.CloudData) > 0 {
		switch body.Agent {
		case "codex_cloud":
			log.Printf("[import-web] codex cloud path: agent=%q url=%q", body.Agent, body.URL)
			s.handleImportCodexCloudTask(w, r, body)
			return
		default:
			// Claude cloud (and any future agents): unwrap events from cloudData.
			log.Printf("[import-web] cloud data path: agent=%q url=%q", body.Agent, body.URL)
			s.handleImportCloudEvents(w, r, body)
			return
		}
	}

	// Legacy cloud event path: old extension sends events directly.
	if len(body.Events) > 0 {
		log.Printf("[import-web] cloud event path: agent=%q url=%q sessionId=%q events=%d", body.Agent, body.URL, body.SessionID, len(body.Events))
		s.handleImportCloudEvents(w, r, body)
		return
	}

	if len(body.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages are required")
		return
	}

	// Check if already imported.
	var existingID string
	err = s.DB.QueryRowContext(r.Context(), "SELECT id FROM conversations WHERE url = ?", body.URL).Scan(&existingID)
	if err == nil {
		writeSuccess(w, http.StatusOK, map[string]any{
			"imported":       true,
			"conversationId": existingID,
			"alreadyExisted": true,
		})
		return
	}
	if err != sql.ErrNoRows {
		log.Printf("error checking conversation URL: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to check conversation URL")
		return
	}

	// Match to a project by repo URL, fall back to web-imports.
	body.RepoURL = strings.TrimSpace(body.RepoURL)
	projectID, _, err := resolveProjectForImport(r.Context(), s.DB, body.RepoURL, "")
	if err != nil {
		log.Printf("error resolving project for web import: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to ensure project")
		return
	}

	conversationID := db.NewID()

	// Calculate timestamps if not provided.
	startedAt := body.StartedAt
	endedAt := body.EndedAt
	if startedAt == 0 && len(body.Messages) > 0 {
		startedAt = body.Messages[0].Timestamp
	}
	if endedAt == 0 && len(body.Messages) > 0 {
		endedAt = body.Messages[len(body.Messages)-1].Timestamp
	}
	now := time.Now().UnixMilli()
	if startedAt == 0 {
		startedAt = now
	}
	if endedAt == 0 {
		endedAt = now
	}

	// Insert conversation.
	_, err = s.DB.ExecContext(r.Context(),
		"INSERT INTO conversations (id, project_id, agent, title, started_at, ended_at, url, family_root_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		conversationID, projectID, body.Agent, body.Title, startedAt, endedAt, body.URL, conversationID,
	)
	if err != nil {
		log.Printf("error inserting conversation: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to insert conversation")
		return
	}

	// Insert messages.
	messages := make([]db.Message, 0, len(body.Messages))
	for _, m := range body.Messages {
		role := m.Role
		if role != "user" && role != "agent" {
			continue
		}
		ts := m.Timestamp
		if ts == 0 {
			ts = now
		}
		messages = append(messages, db.Message{
			Timestamp:      ts,
			ProjectID:      projectID,
			ConversationID: conversationID,
			Role:           role,
			Content:        m.Content,
			Model:          m.Model,
		})
	}

	if len(messages) > 0 {
		if err := db.InsertMessages(r.Context(), s.DB, messages); err != nil {
			log.Printf("error inserting messages: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to insert messages")
			return
		}
	}

	writeSuccess(w, http.StatusCreated, map[string]any{
		"imported":       true,
		"conversationId": conversationID,
		"alreadyExisted": false,
		"messageCount":   len(messages),
	})
}

func importLogSourceFromAgent(agentName string) string {
	switch strings.TrimSpace(agentName) {
	case "claude_cloud", "codex_cloud":
		return agentName
	default:
		return ""
	}
}

// --- Cloud event processing ---

func (s *Server) handleImportCloudEvents(w http.ResponseWriter, r *http.Request, body webConversationImportRequest) {
	// If cloudData is present, unwrap events from it.
	if len(body.CloudData) > 0 && len(body.Events) == 0 {
		var envelope struct {
			Data []json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body.CloudData, &envelope); err == nil && len(envelope.Data) > 0 {
			body.Events = envelope.Data
		} else {
			// Try as direct array.
			var arr []json.RawMessage
			if err := json.Unmarshal(body.CloudData, &arr); err == nil && len(arr) > 0 {
				body.Events = arr
			}
		}
		if len(body.Events) == 0 {
			writeError(w, http.StatusBadRequest, "no events found in cloudData")
			return
		}
	}

	result, err := cloudimport.ProcessClaudeCloudEvents(body.Events)
	if err != nil {
		log.Printf("[import-cloud] processing error: %v", err)
		writeError(w, http.StatusBadRequest, "processing error: "+err.Error())
		return
	}

	messages := result.Messages
	if len(messages) == 0 {
		writeError(w, http.StatusBadRequest, "no conversational messages found in events")
		return
	}

	title := result.Title
	repoURL := result.RepoURL
	cwd := result.Cwd

	projectID, projectMatchMethod, resolveErr := resolveProjectForImport(r.Context(), s.DB, repoURL, cwd)
	if resolveErr != nil {
		log.Printf("error resolving project for cloud import: %v", resolveErr)
		writeError(w, http.StatusInternalServerError, "failed to ensure project")
		return
	}
	log.Printf("[import-cloud] project match: id=%q method=%s repoURL=%q cwd=%q", projectID, projectMatchMethod, repoURL, cwd)

	log.Printf("[import-cloud] title=%q", title)

	// Upsert conversation (update title on re-import, insert on first import).
	conversationID, alreadyExisted, upsertErr := s.upsertCloudConversation(r.Context(), body.URL, projectID, title, body.Agent, messages)
	if upsertErr != nil {
		log.Printf("error upserting conversation: %v", upsertErr)
		writeError(w, http.StatusInternalServerError, "failed to upsert conversation")
		return
	}
	log.Printf("[import-cloud] upsert: conversationId=%q alreadyExisted=%v", conversationID, alreadyExisted)

	for i := range messages {
		messages[i].ProjectID = projectID
		messages[i].ConversationID = conversationID
	}

	// Extract diffs from RawJSON of agent messages.
	beforeDiffs := len(messages)
	messages = agent.AppendDiffDBMessagesWithOptions(messages, agent.DiffAppendOptions{
		Deduplicate:     true,
		UseAllJSONDiffs: true,
	})
	log.Printf("[import-cloud] diff extraction: %d messages before, %d after (%d diffs added)", beforeDiffs, len(messages), len(messages)-beforeDiffs)

	if err := db.InsertMessages(r.Context(), s.DB, messages); err != nil {
		log.Printf("error inserting messages: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to insert messages")
		return
	}
	log.Printf("[import-cloud] done: conversationId=%q projectId=%q messageCount=%d alreadyExisted=%v", conversationID, projectID, len(messages), alreadyExisted)

	// Trigger async recomputation of commit coverage so the commit list view
	// reflects attribution from the newly imported conversation.
	if projectMatchMethod != "web-imports-fallback" {
		go s.recomputeCoverageAfterImport(projectID, messages[0].Timestamp)
	}

	status := http.StatusCreated
	if alreadyExisted {
		status = http.StatusOK
	}

	writeSuccess(w, status, map[string]any{
		"imported":       true,
		"conversationId": conversationID,
		"alreadyExisted": alreadyExisted,
		"messageCount":   len(messages),
	})
}

func (s *Server) handleImportCodexCloudTask(w http.ResponseWriter, r *http.Request, body webConversationImportRequest) {
	log.Printf("[import-codex] cloudData size: %d bytes", len(body.CloudData))
	result, err := cloudimport.ProcessCodexTask(body.CloudData)
	if err != nil {
		log.Printf("[import-codex] processing error: %v", err)
		writeError(w, http.StatusBadRequest, "processing error: "+err.Error())
		return
	}

	messages := result.Messages
	if len(messages) == 0 {
		writeError(w, http.StatusBadRequest, "no messages found in codex task")
		return
	}

	title := result.Title

	projectID, projectMatchMethod, resolveErr := resolveProjectForImport(r.Context(), s.DB, result.RepoURL, "")
	if resolveErr != nil {
		log.Printf("error resolving project for codex import: %v", resolveErr)
		writeError(w, http.StatusInternalServerError, "failed to ensure project")
		return
	}
	log.Printf("[import-codex] project match: id=%q method=%s repoURL=%q", projectID, projectMatchMethod, result.RepoURL)

	// Upsert conversation.
	conversationID, alreadyExisted, upsertErr := s.upsertCloudConversation(r.Context(), body.URL, projectID, title, body.Agent, messages)
	if upsertErr != nil {
		log.Printf("error upserting conversation: %v", upsertErr)
		writeError(w, http.StatusInternalServerError, "failed to upsert conversation")
		return
	}

	for i := range messages {
		messages[i].ProjectID = projectID
		messages[i].ConversationID = conversationID
	}

	if err := db.InsertMessages(r.Context(), s.DB, messages); err != nil {
		log.Printf("error inserting messages: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to insert messages")
		return
	}
	log.Printf("[import-codex] done: conversationId=%q projectId=%q messageCount=%d alreadyExisted=%v", conversationID, projectID, len(messages), alreadyExisted)

	if projectMatchMethod != "web-imports-fallback" {
		go s.recomputeCoverageAfterImport(projectID, messages[0].Timestamp)
	}

	status := http.StatusCreated
	if alreadyExisted {
		status = http.StatusOK
	}

	writeSuccess(w, status, map[string]any{
		"imported":       true,
		"conversationId": conversationID,
		"alreadyExisted": alreadyExisted,
		"messageCount":   len(messages),
	})
}

func (s *Server) upsertCloudConversation(ctx context.Context, url, projectID, title, agentName string, messages []db.Message) (string, bool, error) {
	var existingID string
	err := s.DB.QueryRowContext(ctx, "SELECT id FROM conversations WHERE url = ?", url).Scan(&existingID)
	if err == nil {
		if title != "" {
			s.DB.ExecContext(ctx, "UPDATE conversations SET title = ? WHERE id = ? AND (title = '' OR title IS NULL)", title, existingID)
		}
		return existingID, true, nil
	}
	if err != sql.ErrNoRows {
		return "", false, fmt.Errorf("check existing conversation: %w", err)
	}

	conversationID := db.NewID()

	var startedAt, endedAt int64
	if len(messages) > 0 {
		startedAt = messages[0].Timestamp
		endedAt = messages[len(messages)-1].Timestamp
	}
	now := time.Now().UnixMilli()
	if startedAt == 0 {
		startedAt = now
	}
	if endedAt == 0 {
		endedAt = now
	}

	if agentName == "" {
		agentName = "claude_code"
	}

	_, err = s.DB.ExecContext(ctx,
		"INSERT INTO conversations (id, project_id, agent, title, started_at, ended_at, url, family_root_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		conversationID, projectID, agentName, title, startedAt, endedAt, url, conversationID,
	)
	if err != nil {
		return "", false, fmt.Errorf("insert conversation: %w", err)
	}

	return conversationID, false, nil
}

// resolveProjectForImport finds the best project match for an import by trying
// repo URL, then cwd, then falling back to the web-imports project.
func resolveProjectForImport(ctx context.Context, database *sql.DB, repoURL, cwd string) (projectID, matchMethod string, err error) {
	if repoURL != "" {
		repoURL = normalizeRemoteToRepoKey(repoURL)
		pid, findErr := findProjectByRepoURL(ctx, database, repoURL)
		if findErr != nil {
			log.Printf("error matching project by repo URL: %v", findErr)
		} else if pid != "" {
			return pid, "repoURL", nil
		}
	}
	if cwd != "" {
		pid, findErr := findProjectByCwd(ctx, database, cwd)
		if findErr != nil {
			log.Printf("error matching project by cwd: %v", findErr)
		} else if pid != "" {
			return pid, "cwd", nil
		}
	}
	pid, ensureErr := ensureWebImportsProject(ctx, database)
	if ensureErr != nil {
		return "", "", fmt.Errorf("ensure web imports project: %w", ensureErr)
	}
	return pid, "web-imports-fallback", nil
}

// findProjectByCwd matches a working directory to a project by exact path,
// old_paths, or basename.
func findProjectByCwd(ctx context.Context, database *sql.DB, cwd string) (string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "", nil
	}

	// Try exact path match first.
	var id string
	err := database.QueryRowContext(ctx, "SELECT id FROM projects WHERE path = ?", cwd).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("query project by cwd: %w", err)
	}

	// Try old_paths match.
	rows, err := database.QueryContext(ctx, "SELECT id, path, old_paths FROM projects WHERE old_paths <> '' OR path <> ''")
	if err != nil {
		return "", fmt.Errorf("query projects for cwd match: %w", err)
	}
	defer rows.Close()

	cwdBase := filepath.Base(cwd)
	var basenameMatch string

	for rows.Next() {
		var projID, projPath, oldPaths string
		if err := rows.Scan(&projID, &projPath, &oldPaths); err != nil {
			return "", fmt.Errorf("scan project for cwd match: %w", err)
		}

		// Check old_paths (newline-separated).
		for _, oldPath := range strings.Split(oldPaths, "\n") {
			oldPath = strings.TrimSpace(oldPath)
			if oldPath == "" {
				continue
			}
			if cwd == oldPath || strings.HasPrefix(cwd, oldPath+"/") {
				return projID, nil
			}
			if basenameMatch == "" && cwdBase != "" && filepath.Base(oldPath) == cwdBase {
				basenameMatch = projID
			}
		}

		// Collect basename match as fallback.
		if basenameMatch == "" && cwdBase != "" && filepath.Base(projPath) == cwdBase {
			basenameMatch = projID
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate projects for cwd match: %w", err)
	}

	return basenameMatch, nil
}

// normalizeRemoteToRepoKey extracts "host/owner/repo" from a git remote URL.
// Handles SSH (git@github.com:owner/repo.git), HTTPS, and plain host/owner/repo forms.
func normalizeRemoteToRepoKey(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}

	// SSH format: git@github.com:owner/repo.git
	if strings.Contains(remote, "@") && strings.Contains(remote, ":") {
		parts := strings.SplitN(remote, "@", 2)
		if len(parts) == 2 {
			hostAndPath := parts[1]
			hostAndPath = strings.TrimSuffix(hostAndPath, ".git")
			// Replace first ":" with "/" to normalize git@host:owner/repo -> host/owner/repo
			hostAndPath = strings.Replace(hostAndPath, ":", "/", 1)
			segments := strings.Split(hostAndPath, "/")
			if len(segments) >= 3 {
				return strings.ToLower(segments[0] + "/" + segments[1] + "/" + segments[2])
			}
		}
	}

	// HTTPS, HTTP, or git:// format.
	remote = strings.TrimPrefix(remote, "git://")
	remote = strings.TrimPrefix(remote, "https://")
	remote = strings.TrimPrefix(remote, "http://")
	remote = strings.TrimSuffix(remote, ".git")
	remote = strings.TrimSuffix(remote, "/")

	segments := strings.Split(remote, "/")
	if len(segments) >= 3 {
		return strings.ToLower(segments[0] + "/" + segments[1] + "/" + segments[2])
	}
	return ""
}

// findProjectByRepoURL finds a project whose git remote matches the given
// normalized repo URL (e.g., "github.com/owner/repo").
func findProjectByRepoURL(ctx context.Context, database *sql.DB, repoURL string) (string, error) {
	needle := strings.ToLower(strings.TrimSpace(repoURL))
	if needle == "" {
		return "", nil
	}

	rows, err := database.QueryContext(ctx, "SELECT id, remote FROM projects WHERE remote <> ''")
	if err != nil {
		return "", fmt.Errorf("query projects for remote match: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, remote string
		if err := rows.Scan(&id, &remote); err != nil {
			return "", fmt.Errorf("scan project remote: %w", err)
		}
		normalized := normalizeRemoteToRepoKey(remote)
		if normalized != "" && normalized == needle {
			return id, nil
		}
	}
	return "", rows.Err()
}

func (s *Server) recomputeCoverageAfterImport(projectID string, startedAtMs int64) {
	ctx := context.Background()

	groups, err := listAllProjectGroups(ctx, s.DB)
	if err != nil {
		log.Printf("[import-cloud] recompute: failed to list project groups: %v", err)
		return
	}
	group, ok := findProjectGroupByProjectID(groups, projectID)
	if !ok {
		log.Printf("[import-cloud] recompute: project group not found for %s", projectID)
		return
	}
	repoProject, err := resolveRepoProject(ctx, group)
	if err != nil {
		log.Printf("[import-cloud] recompute: repo not found for %s: %v", projectID, err)
		return
	}

	branch := strings.TrimSpace(repoProject.DefaultBranch)
	if branch == "" {
		branch = "main"
	}

	identity, _ := resolveGitIdentity(ctx, repoProject.Path)
	n, err := recomputeCommitCoverageForProjectWithChangedPatterns(ctx, s.DB, repoProject, group, branch, "", nil, &identity, s.loadExtraLocalUserEmails(), nil, startedAtMs)
	if err != nil {
		log.Printf("[import-cloud] recompute: error for project %s: %v", projectID, err)
		return
	}
	log.Printf("[import-cloud] recompute: updated %d commits for project %s", n, projectID)
}

func ensureWebImportsProject(ctx context.Context, database *sql.DB) (string, error) {
	const webImportsPath = "__web-imports__"
	var id string
	err := database.QueryRowContext(ctx, "SELECT id FROM projects WHERE path = ?", webImportsPath).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("query web imports project: %w", err)
	}

	id = db.NewID()
	_, err = database.ExecContext(ctx,
		"INSERT OR IGNORE INTO projects (id, path, label) VALUES (?, ?, ?)",
		id, webImportsPath, "Web Imports",
	)
	if err != nil {
		return "", fmt.Errorf("insert web imports project: %w", err)
	}

	err = database.QueryRowContext(ctx, "SELECT id FROM projects WHERE path = ?", webImportsPath).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("re-query web imports project: %w", err)
	}
	return id, nil
}
