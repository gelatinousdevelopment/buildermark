package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/davidcann/zrate/web/server/internal/agent"
	"github.com/davidcann/zrate/web/server/internal/db"
)

// rolloutContentBlock represents a content block within a rollout item.
type rolloutContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// rolloutItem represents an item within a rollout event.
type rolloutItem struct {
	Type    string                `json:"type"`
	Role    string                `json:"role"`
	Content []rolloutContentBlock `json:"content"`
}

// rolloutEvent represents a single line in a Codex rollout JSONL file.
type rolloutEvent struct {
	Type     string      `json:"type"`
	ThreadID string      `json:"thread_id"`
	Role     string      `json:"role"`
	Content  string      `json:"content"`
	Item     rolloutItem `json:"item"`
	// Metadata fields
	WorkingDir string `json:"working_dir"`
	Timestamp  int64  `json:"timestamp"`
}

// processedFile tracks the last-seen modification time for a session file.
type processedFile struct {
	modTime time.Time
}

const codexWatcherSourceKindSessionFile = "session_file"

type sessionFileInfo struct {
	path    string
	size    int64
	modTime time.Time
}

// Run performs an initial scan (last 1 week) then polls for new/modified files until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	log.Printf("codex watcher: starting, monitoring %s", a.sessionsDir)

	start := time.Now()
	a.scanSince(ctx, time.Now().Add(-agent.DefaultScanWindow))
	log.Printf("codex watcher: startup scan duration %s", time.Since(start))
	a.backfillGitIDs(ctx)
	a.backfillLabels(ctx)

	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	seen := make(map[string]processedFile)

	for {
		select {
		case <-ctx.Done():
			log.Println("codex watcher: stopped")
			return
		case <-ticker.C:
			a.poll(ctx, seen)
			a.backfillGitIDs(ctx)
		}
	}
}

// ScanSince walks the sessions directory and imports entries from files modified after since.
func (a *Agent) ScanSince(ctx context.Context, since time.Time) int {
	n := a.doScan(ctx, since, nil, false)
	log.Printf("codex watcher: manual scan processed %d files (since %s)", n, since.Format(time.RFC3339))
	return n
}

// ScanPathsSince scans only session files associated with matching working directories.
func (a *Agent) ScanPathsSince(ctx context.Context, since time.Time, paths []string) int {
	n := a.doScan(ctx, since, newPathFilter(paths), false)
	log.Printf("codex watcher: manual path scan processed %d files (since %s, paths=%d)", n, since.Format(time.RFC3339), len(paths))
	return n
}

// scanSince is the internal initial scan.
func (a *Agent) scanSince(ctx context.Context, since time.Time) {
	n := a.doScan(ctx, since, nil, true)
	if n > 0 {
		log.Printf("codex watcher: initial scan processed %d files", n)
	}
}

// doScan walks the sessions directory and processes files modified after since.
func (a *Agent) doScan(ctx context.Context, since time.Time, filter pathFilter, useCheckpoint bool) int {
	files := a.listSessionFiles(since)
	processed := 0
	projectCache := make(map[string]string)
	for _, fi := range files {
		if filter != nil {
			workingDir := readWorkingDir(fi.path)
			if !filter.match(workingDir) {
				continue
			}
		}

		if useCheckpoint {
			st, err := db.GetWatcherScanState(ctx, a.db, a.Name(), codexWatcherSourceKindSessionFile, fi.path)
			if err == nil && st != nil && st.FileSize == fi.size && st.FileMtimeMs == fi.modTime.UnixMilli() {
				continue
			}
		}

		a.processSessionFile(ctx, fi.path, projectCache)
		if useCheckpoint {
			_ = db.UpsertWatcherScanState(ctx, a.db, db.WatcherScanState{
				Agent:       a.Name(),
				SourceKind:  codexWatcherSourceKindSessionFile,
				SourceKey:   fi.path,
				FileSize:    fi.size,
				FileMtimeMs: fi.modTime.UnixMilli(),
				FileOffset:  fi.size,
			})
		}
		processed++
	}
	return processed
}

// poll checks for new or modified session files since the last poll.
func (a *Agent) poll(ctx context.Context, seen map[string]processedFile) {
	filepath.Walk(a.sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}

		modTime := info.ModTime()
		if prev, ok := seen[path]; ok && !modTime.After(prev.modTime) {
			return nil
		}

		a.processSessionFile(ctx, path, nil)
		seen[path] = processedFile{modTime: modTime}
		_ = db.UpsertWatcherScanState(ctx, a.db, db.WatcherScanState{
			Agent:       a.Name(),
			SourceKind:  codexWatcherSourceKindSessionFile,
			SourceKey:   path,
			FileSize:    info.Size(),
			FileMtimeMs: modTime.UnixMilli(),
			FileOffset:  info.Size(),
		})
		return nil
	})
}

// listSessionFiles returns paths to all .jsonl files in the sessions directory
// that were modified after the given time.
func (a *Agent) listSessionFiles(since time.Time) []sessionFileInfo {
	var files []sessionFileInfo
	filepath.Walk(a.sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}
		if info.ModTime().Before(since) {
			return nil
		}
		files = append(files, sessionFileInfo{
			path:    path,
			size:    info.Size(),
			modTime: info.ModTime(),
		})
		return nil
	})
	return files
}

func readWorkingDir(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event codexSessionLine
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		switch event.Type {
		case "session_meta":
			var meta codexSessionMetaPayload
			if err := json.Unmarshal(event.Payload, &meta); err == nil && strings.TrimSpace(meta.Cwd) != "" {
				return strings.TrimSpace(meta.Cwd)
			}
		case "turn_context":
			var turnCtx codexTurnContextPayload
			if err := json.Unmarshal(event.Payload, &turnCtx); err == nil && strings.TrimSpace(turnCtx.Cwd) != "" {
				return strings.TrimSpace(turnCtx.Cwd)
			}
		}
		if wd := strings.TrimSpace(event.WorkingDir); wd != "" {
			return wd
		}
	}
	return ""
}

type pathFilter map[string]struct{}

func newPathFilter(paths []string) pathFilter {
	out := make(pathFilter)
	for _, p := range paths {
		p = strings.TrimSpace(filepath.Clean(p))
		if p == "" {
			continue
		}
		out[p] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (f pathFilter) match(projectPath string) bool {
	if len(f) == 0 {
		return true
	}
	projectPath = strings.TrimSpace(filepath.Clean(projectPath))
	if projectPath == "" {
		return false
	}
	for p := range f {
		if projectPath == p {
			return true
		}
		if strings.HasPrefix(projectPath, p+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// processSessionFile parses a single rollout JSONL file and imports its data.
func (a *Agent) processSessionFile(ctx context.Context, path string, projectCache map[string]string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var threadID string
	var workingDir string
	var currentModel string
	var messages []db.Message
	var responseItemUserIdx []int
	hasEventMsgUser := false
	var firstEventMsgUser string
	var firstResponseItemUser string
	var firstLegacyUser string
	var latestReasoningSummary string
	var zrateEntries []struct {
		rating    int
		note      string
		timestamp int64
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event codexSessionLine
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if m := extractCodexModelFromRawLine(line); m != "" {
			currentModel = m
		}

		ts := parseCodexTimestamp(event.Timestamp)
		if ts <= 0 {
			continue
		}

		switch event.Type {
		case "session_meta":
			var meta codexSessionMetaPayload
			if err := json.Unmarshal(event.Payload, &meta); err != nil {
				continue
			}
			if meta.ID != "" && threadID == "" {
				threadID = meta.ID
			}
			if meta.Cwd != "" && workingDir == "" {
				workingDir = meta.Cwd
			}
			if m := firstNonEmpty(strings.TrimSpace(meta.Model), strings.TrimSpace(meta.ModelSlug)); m != "" {
				currentModel = m
			}
			messages = append(messages, db.Message{
				Timestamp: ts,
				Role:      "agent",
				Model:     currentModel,
				Content:   summarizeCodexEvent(event.Type, event.Payload, ""),
				RawJSON:   line,
			})

		case "turn_context":
			var turnCtx codexTurnContextPayload
			if err := json.Unmarshal(event.Payload, &turnCtx); err != nil {
				continue
			}
			if turnCtx.Cwd != "" && workingDir == "" {
				workingDir = turnCtx.Cwd
			}
			if m := firstNonEmpty(strings.TrimSpace(turnCtx.Model), strings.TrimSpace(turnCtx.ModelSlug)); m != "" {
				currentModel = m
			}
			messages = append(messages, db.Message{
				Timestamp: ts,
				Role:      "agent",
				Model:     currentModel,
				Content:   summarizeCodexEvent(event.Type, event.Payload, ""),
				RawJSON:   line,
			})

		case "response_item":
			var item codexResponseItemPayload
			if err := json.Unmarshal(event.Payload, &item); err != nil {
				continue
			}
			if m := firstNonEmpty(strings.TrimSpace(item.Model), strings.TrimSpace(item.ModelSlug)); m != "" {
				currentModel = m
			}
			role := "agent"
			content := extractResponseItemText(item.Content)
			summaryText := extractResponseItemSummaryText(item.Summary)
			if strings.TrimSpace(content) == "" {
				content = summaryText
			}
			if item.Type == "reasoning" && strings.TrimSpace(summaryText) != "" {
				latestReasoningSummary = strings.TrimSpace(summaryText)
			}
			if item.Type == "message" {
				role = "user"
				if item.Role == "assistant" {
					role = "agent"
				} else if item.Role != "user" {
					role = "agent"
				}
			}
			if strings.TrimSpace(content) == "" {
				content = summarizeCodexEvent(event.Type, event.Payload, item.Type)
			}

			isResponseItemUser := item.Type == "message" && item.Role == "user"

			messages = append(messages, db.Message{
				Timestamp: ts,
				Role:      role,
				Model:     currentModel,
				Content:   content,
				RawJSON:   line,
			})
			if isResponseItemUser {
				responseItemUserIdx = append(responseItemUserIdx, len(messages)-1)
				if firstResponseItemUser == "" && strings.TrimSpace(content) != "" {
					firstResponseItemUser = strings.TrimSpace(content)
				}
			}

			if role == "user" {
				if rating, note := parseZrateDisplay(content); rating >= 0 {
					zrateEntries = append(zrateEntries, struct {
						rating    int
						note      string
						timestamp int64
					}{rating, note, ts})
				}
			}

		case "event_msg":
			var msg codexEventMsgPayload
			if err := json.Unmarshal(event.Payload, &msg); err != nil {
				continue
			}
			role := "agent"
			if msg.Type == "user_message" {
				role = "user"
				hasEventMsgUser = true
				if firstEventMsgUser == "" && strings.TrimSpace(msg.Message) != "" {
					firstEventMsgUser = strings.TrimSpace(msg.Message)
				}
			}
			content := strings.TrimSpace(msg.Message)
			if content == "" {
				content = summarizeCodexEvent(event.Type, event.Payload, msg.Type)
			}
			messages = append(messages, db.Message{
				Timestamp: ts,
				Role:      role,
				Model:     currentModel,
				Content:   content,
				RawJSON:   line,
			})
			if role == "user" {
				if rating, note := parseZrateDisplay(msg.Message); rating >= 0 {
					zrateEntries = append(zrateEntries, struct {
						rating    int
						note      string
						timestamp int64
					}{rating, note, ts})
				}
			}

		case "input":
			// Legacy schema.
			if event.ThreadID != "" && threadID == "" {
				threadID = event.ThreadID
			}
			if event.WorkingDir != "" && workingDir == "" {
				workingDir = event.WorkingDir
			}
			// Direct user input event.
			content := strings.TrimSpace(event.Content)
			if content == "" {
				content = summarizeCodexEvent(event.Type, nil, "")
			}
			messages = append(messages, db.Message{
				Timestamp: ts,
				Role:      "user",
				Model:     currentModel,
				Content:   content,
				RawJSON:   line,
			})
			// Check for $zrate command.
			if strings.HasPrefix(content, "$zrate ") {
				if rating, note := parseZrateDisplay(content); rating >= 0 {
					zrateEntries = append(zrateEntries, struct {
						rating    int
						note      string
						timestamp int64
					}{rating, note, ts})
				}
			}
			if firstLegacyUser == "" && strings.TrimSpace(content) != "" {
				firstLegacyUser = strings.TrimSpace(content)
			}

		case "item.completed":
			// Legacy schema.
			if event.ThreadID != "" && threadID == "" {
				threadID = event.ThreadID
			}
			if event.WorkingDir != "" && workingDir == "" {
				workingDir = event.WorkingDir
			}
			item := event.Item
			role := "user"
			if item.Role == "assistant" || item.Type == "agent_message" {
				role = "agent"
			}

			var text strings.Builder
			for _, c := range item.Content {
				if c.Type == "text" || c.Type == "output_text" || c.Type == "input_text" {
					if text.Len() > 0 {
						text.WriteString("\n")
					}
					text.WriteString(c.Text)
				}
			}

			content := strings.TrimSpace(text.String())
			if content == "" {
				content = summarizeCodexEvent(event.Type, nil, item.Type)
			}

			messages = append(messages, db.Message{
				Timestamp: ts,
				Role:      role,
				Model:     currentModel,
				Content:   content,
				RawJSON:   line,
			})

			// Check user messages for $zrate.
			if role == "user" && strings.HasPrefix(content, "$zrate ") {
				if rating, note := parseZrateDisplay(content); rating >= 0 {
					zrateEntries = append(zrateEntries, struct {
						rating    int
						note      string
						timestamp int64
					}{rating, note, ts})
				}
			}
			if role == "user" && firstLegacyUser == "" && strings.TrimSpace(content) != "" {
				firstLegacyUser = strings.TrimSpace(content)
			}

		default:
			messages = append(messages, db.Message{
				Timestamp: ts,
				Role:      "agent",
				Model:     currentModel,
				Content:   summarizeCodexEvent(event.Type, event.Payload, ""),
				RawJSON:   line,
			})
		}
	}
	if hasEventMsgUser {
		for _, i := range responseItemUserIdx {
			// When explicit user_message events exist, treat response_item user
			// records as non-user log/system context to avoid mislabeling.
			messages[i].Role = "agent"
		}
	}
	messages = appendDiffDBMessages(messages)
	normalizeMessageTimestamps(messages)

	// Derive thread ID from filename if not found in events.
	if threadID == "" {
		threadID = threadIDFromFilename(filepath.Base(path))
	}

	if threadID == "" || workingDir == "" {
		return
	}

	projectID := ""
	if projectCache != nil {
		projectID = projectCache[workingDir]
	}
	if projectID == "" {
		var err error
		projectID, err = db.EnsureProject(ctx, a.db, workingDir)
		if err != nil {
			log.Printf("codex watcher: ensure project %q: %v", workingDir, err)
			return
		}
		if projectCache != nil {
			projectCache[workingDir] = projectID
		}
	}

	if err := db.EnsureConversation(ctx, a.db, threadID, projectID, a.Name()); err != nil {
		log.Printf("codex watcher: ensure conversation %s: %v", threadID, err)
		return
	}

	title := normalizeSummaryTitle(latestReasoningSummary)
	if title == "" {
		titlePrompt := firstEventMsgUser
		if titlePrompt == "" {
			titlePrompt = firstResponseItemUser
		}
		if titlePrompt == "" {
			titlePrompt = firstLegacyUser
		}
		title = titleFromPrompt(titlePrompt)
	}
	if title != "" {
		if err := db.UpdateConversationTitle(ctx, a.db, threadID, title); err != nil {
			log.Printf("codex watcher: update title for %s: %v", threadID, err)
		}
	}

	// Fill in project and conversation IDs on messages.
	for i := range messages {
		messages[i].ProjectID = projectID
		messages[i].ConversationID = threadID
	}

	if len(messages) > 0 {
		if err := db.InsertMessages(ctx, a.db, messages); err != nil {
			log.Printf("codex watcher: insert messages for session %s: %v", threadID, err)
		}
	}

	// Reconcile orphaned ratings.
	for _, z := range zrateEntries {
		if err := db.ReconcileOrphanedRating(ctx, a.db, z.rating, z.note, z.timestamp, threadID); err != nil {
			log.Printf("codex watcher: reconcile rating for session %s: %v", threadID, err)
		}
	}
}

func summarizeCodexEvent(eventType string, payload json.RawMessage, subtype string) string {
	label := strings.TrimSpace(eventType)
	if label == "" {
		label = "event"
	}
	if strings.TrimSpace(subtype) != "" {
		label += ":" + strings.TrimSpace(subtype)
	}

	extract := func(v any) string {
		s, ok := v.(string)
		if !ok {
			return ""
		}
		return strings.TrimSpace(s)
	}

	if len(payload) > 0 && string(payload) != "null" {
		var obj map[string]any
		if err := json.Unmarshal(payload, &obj); err == nil {
			for _, key := range []string{"message", "type", "role", "cwd", "id"} {
				if value := extract(obj[key]); value != "" {
					return fmt.Sprintf("[%s] %s", label, value)
				}
			}
		}
	}

	return fmt.Sprintf("[%s]", label)
}

// normalizeMessageTimestamps makes per-batch timestamps unique while preserving
// event order. This avoids dropping same-millisecond Codex events due the DB's
// UNIQUE(conversation_id, timestamp) constraint.
func normalizeMessageTimestamps(messages []db.Message) {
	used := make(map[int64]struct{}, len(messages))
	for i := range messages {
		ts := messages[i].Timestamp
		for {
			if _, exists := used[ts]; !exists {
				break
			}
			ts++
		}
		messages[i].Timestamp = ts
		used[ts] = struct{}{}
	}
}

// threadIDFromFilename extracts a thread ID from a rollout filename like
// "rollout-1234567890-abc123.jsonl". Returns the "abc123" part, or the
// full base name (without extension) if the format doesn't match.
func threadIDFromFilename(name string) string {
	name = strings.TrimSuffix(name, ".jsonl")
	parts := strings.SplitN(name, "-", 3)
	if len(parts) >= 3 && parts[0] == "rollout" {
		return parts[2]
	}
	return name
}

// backfillLabels updates project labels from the last path component to the
// git repository root directory name for projects whose label was auto-generated.
func (a *Agent) backfillLabels(ctx context.Context) {
	projects, err := db.ListAllProjects(ctx, a.db)
	if err != nil {
		log.Printf("codex watcher: list projects for label backfill: %v", err)
		return
	}

	updated := 0
	for _, p := range projects {
		repoName := db.RepoLabel(p.Path)
		if repoName != p.Label && p.Label == filepath.Base(p.Path) {
			if err := db.SetProjectLabel(ctx, a.db, p.ID, repoName); err != nil {
				log.Printf("codex watcher: update label for %s: %v", p.ID, err)
				continue
			}
			updated++
		}
	}
	if updated > 0 {
		log.Printf("codex watcher: backfilled %d project labels", updated)
	}
}

// backfillGitIDs finds all projects without a git_id and attempts to
// resolve it from the git root commit.
func (a *Agent) backfillGitIDs(ctx context.Context) {
	projects, err := db.ListProjectsWithoutGitID(ctx, a.db)
	if err != nil {
		log.Printf("codex watcher: list projects without git_id: %v", err)
		return
	}

	updated := 0
	for _, p := range projects {
		if gitID := resolveGitID(p.Path); gitID != "" {
			if err := db.UpdateProjectGitID(ctx, a.db, p.ID, gitID); err != nil {
				log.Printf("codex watcher: update git_id for %s: %v", p.ID, err)
				continue
			}
			updated++
		}
	}
	if updated > 0 {
		log.Printf("codex watcher: backfilled %d project git_ids", updated)
	}
}

// parseZrateDisplay parses "$zrate 4 optional note" into (4, "optional note").
// Returns (-1, "") if the format is invalid.
func parseZrateDisplay(display string) (int, string) {
	display = strings.TrimSpace(display)

	// Codex can render the command as markdown link: [$zrate](...) 4 note
	if strings.HasPrefix(display, "[$zrate](") {
		if i := strings.Index(display, ")"); i >= 0 {
			display = "$zrate" + display[i+1:]
		}
	}

	if !strings.HasPrefix(display, "$zrate ") {
		if i := strings.Index(display, "$zrate "); i >= 0 {
			display = display[i:]
		}
	}
	if !strings.HasPrefix(display, "$zrate ") {
		return -1, ""
	}

	rest := strings.TrimSpace(strings.TrimPrefix(display, "$zrate"))
	if rest == "" {
		return -1, ""
	}
	parts := strings.SplitN(rest, " ", 2)
	rating, err := strconv.Atoi(parts[0])
	if err != nil || rating < 0 || rating > 5 {
		return -1, ""
	}
	note := ""
	if len(parts) > 1 {
		note = parts[1]
	}
	return rating, note
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func normalizeSummaryTitle(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	title := titleFromPrompt(summary)
	if title == "" {
		return ""
	}
	title = strings.TrimSpace(title)
	if strings.HasPrefix(title, "**") && strings.HasSuffix(title, "**") && len(title) > 4 {
		title = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(title, "**"), "**"))
	}
	if strings.HasPrefix(title, "__") && strings.HasSuffix(title, "__") && len(title) > 4 {
		title = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(title, "__"), "__"))
	}
	return strings.TrimSpace(title)
}

func extractCodexModelFromRawLine(line string) string {
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		return ""
	}

	if model := firstNonEmpty(
		stringValue(obj["model"]),
		stringValue(obj["model_slug"]),
		stringValue(obj["modelName"]),
		stringValue(obj["model_name"]),
	); model != "" {
		return model
	}

	payload, _ := obj["payload"].(map[string]any)
	if payload == nil {
		return ""
	}
	return firstNonEmpty(
		stringValue(payload["model"]),
		stringValue(payload["model_slug"]),
		stringValue(payload["modelName"]),
		stringValue(payload["model_name"]),
	)
}

func stringValue(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
