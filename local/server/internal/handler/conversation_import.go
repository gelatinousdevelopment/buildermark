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
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent/claude"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
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

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

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
	if len(body.Events) == 0 && len(body.Messages) == 0 {
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

	// Cloud event path: process raw events into messages.
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

	// Try to match to an existing project by repo URL, fall back to web-imports.
	var projectID string
	body.RepoURL = strings.TrimSpace(body.RepoURL)
	if body.RepoURL != "" {
		projectID, err = findProjectByRepoURL(r.Context(), s.DB, body.RepoURL)
		if err != nil {
			log.Printf("error matching project by repo URL: %v", err)
		}
	}
	if projectID == "" {
		projectID, err = ensureWebImportsProject(r.Context(), s.DB)
		if err != nil {
			log.Printf("error ensuring web imports project: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to ensure project")
			return
		}
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
		"INSERT INTO conversations (id, project_id, agent, title, started_at, ended_at, url) VALUES (?, ?, ?, ?, ?, ?, ?)",
		conversationID, projectID, body.Agent, body.Title, startedAt, endedAt, body.URL,
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

// --- Cloud event processing (consolidated from the old dedicated endpoint) ---

type cloudEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	CreatedAt string `json:"created_at"`
	Model     string `json:"model"`
	Cwd       string `json:"cwd"`
	Result    string `json:"result"`
	Message   *struct {
		Role       string          `json:"role"`
		Model      string          `json:"model"`
		Content    json.RawMessage `json:"content"`
		StopReason string          `json:"stop_reason"`
	} `json:"message,omitempty"`
}

func (s *Server) handleImportCloudEvents(w http.ResponseWriter, r *http.Request, body webConversationImportRequest) {
	// Parse events.
	events := make([]cloudEvent, 0, len(body.Events))
	parseErrors := 0
	for _, raw := range body.Events {
		var ev cloudEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			parseErrors++
			continue
		}
		events = append(events, ev)
	}

	log.Printf("[import-cloud] parsed %d/%d events (%d parse errors)", len(events), len(body.Events), parseErrors)

	if len(events) == 0 {
		writeError(w, http.StatusBadRequest, "no valid events found")
		return
	}

	// Log event type breakdown.
	typeCounts := map[string]int{}
	for _, ev := range events {
		key := ev.Type
		if ev.Subtype != "" {
			key += ":" + ev.Subtype
		}
		typeCounts[key]++
	}
	log.Printf("[import-cloud] event types: %v", typeCounts)

	// Extract metadata from init event.
	var model, cwd, repoURL string
	for _, ev := range events {
		if ev.Type == "system" && ev.Subtype == "init" {
			if ev.Model != "" {
				model = ev.Model
			}
			if ev.Cwd != "" {
				cwd = ev.Cwd
			}
			break
		}
	}
	log.Printf("[import-cloud] init metadata: model=%q cwd=%q", model, cwd)

	// Look for repo clone info in env_manager_log entries.
	for _, rawEv := range body.Events {
		var logEv struct {
			Type    string `json:"type"`
			Subtype string `json:"subtype"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(rawEv, &logEv); err != nil {
			continue
		}
		if logEv.Type != "system" || logEv.Subtype != "env_manager_log" {
			continue
		}
		if strings.Contains(logEv.Content, "Cloning repository") {
			parts := strings.SplitN(logEv.Content, "Cloning repository ", 2)
			if len(parts) == 2 {
				candidate := strings.TrimSpace(strings.Fields(parts[1])[0])
				if normalized := normalizeRemoteToRepoKey(candidate); normalized != "" {
					repoURL = normalized
				}
			}
		}
	}

	// Convert events to messages.
	messages := make([]db.Message, 0, len(events))
	now := time.Now().UnixMilli()

	skippedSystem, skippedNoMsg, skippedEmpty, skippedSysMsg := 0, 0, 0, 0
	roleCounts := map[string]int{}

	for i, ev := range events {
		if ev.Type == "system" {
			skippedSystem++
			continue
		}

		// "result" events are the final answer displayed to the user.
		// They have no message field — the content is in ev.Result.
		if ev.Type == "result" && strings.TrimSpace(ev.Result) != "" {
			ts := int64(0)
			if parsed, err := time.Parse(time.RFC3339Nano, ev.CreatedAt); err == nil {
				ts = parsed.UnixMilli()
			}
			if ts == 0 {
				ts = now
			}
			roleCounts["agent"]++
			messages = append(messages, db.Message{
				Timestamp:   ts,
				Role:        "agent",
				MessageType: db.MessageTypeFinalAnswer,
				Content:     strings.TrimSpace(ev.Result),
				Model:       model,
				RawJSON:     string(body.Events[i]),
			})
			continue
		}

		if ev.Message == nil {
			skippedNoMsg++
			continue
		}

		var entry claude.ConversationEntry
		var rawJSON string

		switch body.Agent {
		case "claude_cloud":
			entry, rawJSON = claudeCloudToEntry(ev, body.Events[i])
		default:
			// Default: treat as Claude-like events.
			entry, rawJSON = claudeCloudToEntry(ev, body.Events[i])
		}

		content := claude.ContentFromConversationEntry(entry)

		// If content is empty but the event has tool_use blocks, generate a
		// summary so the message isn't skipped. The RawJSON still carries the
		// full payload for diff extraction.
		if content == "" {
			summary := toolUseSummary(ev.Message.Content)
			if summary != "" {
				content = summary
			}
		}

		if content == "" {
			skippedEmpty++
			continue
		}
		if claude.IsSystemMessage(content) {
			skippedSysMsg++
			continue
		}

		role := "agent"
		if entry.Type == "user" {
			role = "user"
			if strings.TrimSpace(entry.SourceToolAssistantUUID) != "" || claude.IsAssistantAuthoredConversationEntry(entry) || claude.IsSkillExpansion(content) {
				role = "agent"
			}
		}

		stopReason := strings.TrimSpace(ev.Message.StopReason)
		var messageType string
		role, messageType, content = claude.ClassifyClaudeMessage(role, content, rawJSON, stopReason)
		roleCounts[role]++

		ts := int64(0)
		if parsed, err := time.Parse(time.RFC3339Nano, ev.CreatedAt); err == nil {
			ts = parsed.UnixMilli()
		}
		if ts == 0 {
			ts = now
		}

		msgModel := strings.TrimSpace(ev.Message.Model)
		if msgModel == "" {
			msgModel = model
		}

		messages = append(messages, db.Message{
			Timestamp:   ts,
			Role:        role,
			MessageType: messageType,
			Content:     content,
			Model:       msgModel,
			RawJSON:     rawJSON,
		})
	}

	log.Printf("[import-cloud] message conversion: %d messages from %d events (skipped: system=%d noMsg=%d empty=%d sysMsg=%d) roles: %v",
		len(messages), len(events), skippedSystem, skippedNoMsg, skippedEmpty, skippedSysMsg, roleCounts)

	if len(messages) == 0 {
		writeError(w, http.StatusBadRequest, "no conversational messages found in events")
		return
	}

	// Extract title from summary events or first user prompt.
	// Skip body.Title (from browser extension) as it can be from a different conversation.
	title := extractCloudTitle(events, messages)

	// Find project: try repo URL, then cwd (with old_paths / basename fallback), then web-imports.
	var projectID string
	var err error
	var projectMatchMethod string
	if repoURL != "" {
		projectID, err = findProjectByRepoURL(r.Context(), s.DB, repoURL)
		if err != nil {
			log.Printf("error matching project by repo URL: %v", err)
		}
		if projectID != "" {
			projectMatchMethod = "repoURL"
		}
	}
	if projectID == "" && cwd != "" {
		projectID, err = findProjectByCwd(r.Context(), s.DB, cwd)
		if err != nil {
			log.Printf("error matching project by cwd: %v", err)
		}
		if projectID != "" {
			projectMatchMethod = "cwd"
		}
	}
	if projectID == "" {
		projectID, err = ensureWebImportsProject(r.Context(), s.DB)
		if err != nil {
			log.Printf("error ensuring web imports project: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to ensure project")
			return
		}
		projectMatchMethod = "web-imports-fallback"
	}
	log.Printf("[import-cloud] project match: id=%q method=%s repoURL=%q cwd=%q", projectID, projectMatchMethod, repoURL, cwd)

	log.Printf("[import-cloud] title=%q", title)

	// Upsert conversation (update title on re-import, insert on first import).
	conversationID, alreadyExisted, err := s.upsertCloudConversation(r.Context(), body.URL, projectID, title, body.Agent, messages)
	if err != nil {
		log.Printf("error upserting conversation: %v", err)
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
		go s.recomputeCoverageAfterImport(projectID)
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
		"INSERT INTO conversations (id, project_id, agent, title, started_at, ended_at, url) VALUES (?, ?, ?, ?, ?, ?, ?)",
		conversationID, projectID, agentName, title, startedAt, endedAt, url,
	)
	if err != nil {
		return "", false, fmt.Errorf("insert conversation: %w", err)
	}

	return conversationID, false, nil
}

// toolUseSummary generates a short placeholder for assistant messages that
// contain only tool_use blocks (Edit, Write, Bash, etc.) whose inputs don't
// have any of the preferred text keys, so ContentFromConversationEntry returns "".
func toolUseSummary(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	var names []string
	for _, b := range blocks {
		typ, _ := b["type"].(string)
		if typ != "tool_use" {
			continue
		}
		name, _ := b["name"].(string)
		if name == "" {
			name = "tool"
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}

	// Deduplicate while preserving order.
	seen := make(map[string]struct{}, len(names))
	unique := make([]string, 0, len(names))
	for _, n := range names {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		unique = append(unique, n)
	}

	return "[" + strings.Join(unique, ", ") + "]"
}

func extractCloudTitle(events []cloudEvent, messages []db.Message) string {
	for _, ev := range events {
		if ev.Type == "summary" || (ev.Type == "system" && ev.Subtype == "summary") {
			if ev.Message != nil {
				text := claude.ExtractUserText(ev.Message.Content)
				if text != "" {
					return text
				}
			}
		}
	}

	for _, m := range messages {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			return agent.TitleFromPrompt(m.Content)
		}
	}

	return ""
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

	// HTTPS or plain format.
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

// claudeCloudToEntry translates a cloud event into a local ConversationEntry
// so the existing Claude classification pipeline works correctly.
func claudeCloudToEntry(ev cloudEvent, raw json.RawMessage) (claude.ConversationEntry, string) {
	entry := claude.ConversationEntry{}
	entry.Type = ev.Type
	if entry.Type == "" {
		entry.Type = ev.Message.Role
	}
	entry.Timestamp = ev.CreatedAt
	entry.Message.Role = ev.Message.Role
	entry.Message.Model = ev.Message.Model
	entry.Message.Content = ev.Message.Content
	entry.Message.StopReason = ev.Message.StopReason

	// Cloud events with type "user" whose content is a JSON array are
	// agent-authored (subagent prompts, tool results, etc). Real user messages
	// have string content. Set SourceToolAssistantUUID so the existing pipeline
	// classifies them as role "agent" instead of "user".
	if entry.Type == "user" && isContentArray(ev.Message.Content) {
		entry.SourceToolAssistantUUID = "cloud"
	}

	return entry, string(raw)
}

// codexCloudToEntry is a stub for future Codex cloud event translation.
func codexCloudToEntry(ev cloudEvent, raw json.RawMessage) (role string, content string, model string, rawJSON string) {
	role = "agent"
	if ev.Message != nil && ev.Message.Role == "user" {
		role = "user"
	}
	content = claude.ExtractUserText(ev.Message.Content)
	if ev.Message != nil {
		model = ev.Message.Model
	}
	rawJSON = string(raw)
	return
}

// isContentArray returns true if content is a JSON array (starts with '[').
// In Claude cloud events, real user messages have string content while
// agent-authored messages (tool results, subagent prompts, etc.) have array content.
func isContentArray(content json.RawMessage) bool {
	return len(content) > 0 && content[0] == '['
}

func (s *Server) recomputeCoverageAfterImport(projectID string) {
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
	n, err := recomputeCommitCoverageForProject(ctx, s.DB, repoProject, group, branch, &identity, s.loadExtraLocalUserEmails())
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
