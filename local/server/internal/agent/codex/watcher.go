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

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
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

// processedFile tracks the last-seen modification time and cached workingDir for a session file.
type processedFile struct {
	modTime    time.Time
	workingDir string
}

const codexWatcherSourceKindSessionFile = "session_file"
const codexWatcherSourceKindScanMarker = "scan_marker"

type sessionFileInfo struct {
	path    string
	size    int64
	modTime time.Time
}

// Run performs an initial scan (last 1 week) then polls for new/modified files until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	log.Printf("codex watcher: starting, monitoring %s", a.sessionsDir)

	scanWindow := a.startupScanWindow(ctx)
	log.Printf("codex watcher: startup scan window %s", scanWindow)

	scanCutoff := time.Now().Add(-scanWindow)
	trackedFilter := agent.TrackedProjectFilter(ctx, a.DB, nil)
	start := time.Now()
	a.scanSinceFiltered(ctx, scanCutoff, trackedFilter)
	a.persistStartupScanMarker(ctx)
	log.Printf("codex watcher: startup scan duration %s", agent.FmtDuration(time.Since(start)))
	a.BackfillGitIDs(ctx)
	a.BackfillLabels(ctx)

	ticker := time.NewTicker(a.Interval)
	defer ticker.Stop()

	seen := make(map[string]processedFile)

	for {
		select {
		case <-ctx.Done():
			log.Println("codex watcher: stopped")
			return
		case <-ticker.C:
			pollStart := time.Now()
			trackedFilter = agent.TrackedProjectFilter(ctx, a.DB, nil)
			filterDone := time.Now()
			count := a.pollFiltered(ctx, seen, trackedFilter)
			pollDone := time.Now()
			a.BackfillGitIDsThrottled(ctx)
			total := time.Since(pollStart)
			var newInterval time.Duration
			if count > 0 {
				newInterval = a.MarkActive()
			} else {
				newInterval = a.MarkIdle()
			}
			a.RecordPoll()
			f := agent.FmtDuration
			log.Printf("codex watcher: poll took %s (filter=%s sessions=%s[%d] files_cached=%d seen=%d) next=%s",
				f(total),
				f(filterDone.Sub(pollStart)),
				f(pollDone.Sub(filterDone)), count,
				len(a.cachedSessionFiles), len(seen),
				f(newInterval))
			ticker.Reset(newInterval)
		}
	}
}

// DiscoverProjectPathsSince returns working directories found in Codex session
// files modified since the given cutoff.
func (a *Agent) DiscoverProjectPathsSince(_ context.Context, since time.Time) []string {
	files := a.listSessionFiles(since)
	seen := make(map[string]struct{})
	for _, fi := range files {
		workingDir := strings.TrimSpace(readWorkingDir(fi.path))
		if workingDir == "" {
			continue
		}
		seen[filepath.Clean(workingDir)] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}

// ScanSince walks the sessions directory and imports entries from files modified after since.
func (a *Agent) ScanSince(ctx context.Context, since time.Time, progress agent.ScanProgressFunc) int {
	filter := agent.TrackedProjectFilter(ctx, a.DB, nil)
	n := a.doScan(ctx, since, filter, false, progress)
	a.persistStartupScanMarker(ctx)
	log.Printf("codex watcher: manual scan processed %d files (since %s)", n, since.Format(time.RFC3339))
	return n
}

// ScanPathsSince scans only session files associated with matching working directories.
func (a *Agent) ScanPathsSince(ctx context.Context, since time.Time, paths []string, progress agent.ScanProgressFunc) int {
	n := a.doScan(ctx, since, agent.NewPathFilter(paths), false, progress)
	a.persistStartupScanMarker(ctx)
	log.Printf("codex watcher: manual path scan processed %d files (since %s, paths=%d)", n, since.Format(time.RFC3339), len(paths))
	return n
}

func (a *Agent) startupScanWindow(ctx context.Context) time.Duration {
	scanWindow := agent.DefaultScanWindow
	latestMs, err := db.LatestWatcherScanTimestampForScopes(ctx, a.DB, a.Name(),
		db.WatcherScanScope{SourceKind: codexWatcherSourceKindSessionFile, SourceKey: a.sessionsDir, MatchPrefix: true},
		db.WatcherScanScope{SourceKind: codexWatcherSourceKindScanMarker, SourceKey: a.startupScanMarkerKey()},
	)
	if err == nil {
		scanWindow = agent.StartupScanWindow(latestMs)
	}
	return scanWindow
}

func (a *Agent) startupScanMarkerKey() string {
	return filepath.Clean(a.Home)
}

func (a *Agent) persistStartupScanMarker(ctx context.Context) {
	_ = db.UpsertWatcherScanState(ctx, a.DB, db.WatcherScanState{
		Agent:      a.Name(),
		SourceKind: codexWatcherSourceKindScanMarker,
		SourceKey:  a.startupScanMarkerKey(),
	})
}

// scanSince is the internal initial scan.
func (a *Agent) scanSince(ctx context.Context, since time.Time) {
	a.scanSinceFiltered(ctx, since, nil)
}

func (a *Agent) scanSinceFiltered(ctx context.Context, since time.Time, filter agent.PathFilter) {
	n := a.doScan(ctx, since, filter, true, nil)
	if n > 0 {
		log.Printf("codex watcher: initial scan processed %d files", n)
	}
}

// doScan walks the sessions directory and processes files modified after since.
func (a *Agent) doScan(ctx context.Context, since time.Time, filter agent.PathFilter, useCheckpoint bool, progress agent.ScanProgressFunc) int {
	listSince := since
	if !useCheckpoint {
		// Manual scans prioritize correctness: inspect all session files and
		// apply timeframe filtering by event timestamp inside each file.
		listSince = time.Time{}
	}
	files := a.listSessionFiles(listSince)
	processed := 0
	projectCache := make(map[string]string)
	for _, fi := range files {
		if progress != nil {
			progress(fi.path)
		}
		if filter != nil {
			workingDir := readWorkingDir(fi.path)
			if !filter.Match(workingDir) {
				continue
			}
		}

		if useCheckpoint {
			st, err := db.GetWatcherScanState(ctx, a.DB, a.Name(), codexWatcherSourceKindSessionFile, fi.path)
			if err == nil && st != nil && st.FileSize == fi.size && st.FileMtimeMs == fi.modTime.UnixMilli() {
				continue
			}
		}

		if !a.processSessionFileSince(ctx, fi.path, projectCache, since) {
			continue
		}
		if useCheckpoint {
			_ = db.UpsertWatcherScanState(ctx, a.DB, db.WatcherScanState{
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
	a.pollFiltered(ctx, seen, nil)
}

func (a *Agent) pollFiltered(ctx context.Context, seen map[string]processedFile, filter agent.PathFilter) int {
	processed := 0
	files := a.listSessionFilesCached()
	for _, fi := range files {
		modTime := fi.modTime
		if prev, ok := seen[fi.path]; ok && !modTime.After(prev.modTime) {
			continue
		}

		if filter != nil {
			// Reuse cached workingDir from seen map if available for unchanged files.
			wd := ""
			if prev, ok := seen[fi.path]; ok && prev.workingDir != "" {
				wd = prev.workingDir
			} else {
				wd = readWorkingDir(fi.path)
			}
			if !filter.Match(wd) {
				seen[fi.path] = processedFile{modTime: modTime, workingDir: wd}
				continue
			}
			seen[fi.path] = processedFile{modTime: modTime, workingDir: wd}
		} else {
			seen[fi.path] = processedFile{modTime: modTime}
		}

		a.processSessionFile(ctx, fi.path, nil)
		processed++
		_ = db.UpsertWatcherScanState(ctx, a.DB, db.WatcherScanState{
			Agent:       a.Name(),
			SourceKind:  codexWatcherSourceKindSessionFile,
			SourceKey:   fi.path,
			FileSize:    fi.size,
			FileMtimeMs: modTime.UnixMilli(),
			FileOffset:  fi.size,
		})
	}
	return processed
}

const dirCacheTTL = 5 * time.Minute

// listSessionFilesCached returns a cached listing of all session files,
// refreshing the cache every dirCacheTTL.
func (a *Agent) listSessionFilesCached() []sessionFileInfo {
	if a.cachedSessionFiles != nil && time.Since(a.cachedSessionFilesTime) < dirCacheTTL {
		return a.cachedSessionFiles
	}
	a.cachedSessionFiles = a.listSessionFiles(time.Time{})
	a.cachedSessionFilesTime = time.Now()
	return a.cachedSessionFiles
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

// processSessionFile parses a single rollout JSONL file and imports its data.
func (a *Agent) processSessionFile(ctx context.Context, path string, projectCache map[string]string) {
	_ = a.processSessionFileSince(ctx, path, projectCache, time.Time{})
}

func (a *Agent) processSessionFileSince(ctx context.Context, path string, projectCache map[string]string, since time.Time) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	cutoffMs := int64(0)
	if !since.IsZero() {
		cutoffMs = since.UnixMilli()
	}

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
	requestUserInputQuestions := make(map[string][]codexQuestionSpec)
	var ratingEntries []struct {
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
			if m := agent.FirstNonEmpty(strings.TrimSpace(meta.Model), strings.TrimSpace(meta.ModelSlug)); m != "" {
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
			if m := agent.FirstNonEmpty(strings.TrimSpace(turnCtx.Model), strings.TrimSpace(turnCtx.ModelSlug)); m != "" {
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
			if m := agent.FirstNonEmpty(strings.TrimSpace(item.Model), strings.TrimSpace(item.ModelSlug)); m != "" {
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
			messageType := "log"
			if item.Type == "message" {
				role = "user"
				if item.Role == "assistant" {
					role = "agent"
				} else if item.Role != "user" {
					role = "agent"
				}
				if role == "user" {
					messageType = "prompt"
				}
			}
			if item.Type == "function_call" && strings.TrimSpace(item.Name) == "request_user_input" {
				role = "agent"
				messageType = "question"
				questions := parseRequestUserInputQuestions(item.Arguments)
				if len(questions) > 0 {
					content = formatCodexQuestionsMarkdown(questions)
					if callID := strings.TrimSpace(item.CallID); callID != "" {
						requestUserInputQuestions[callID] = questions
					}
				}
			}
			if item.Type == "function_call_output" {
				callID := strings.TrimSpace(item.CallID)
				answers := parseRequestUserInputAnswers(item.Output)
				questions := requestUserInputQuestions[callID]
				if len(questions) > 0 {
					role = "user"
					messageType = "answer"
					content = formatCodexAnswersMarkdown(questions, answers)
				}
			}
			if strings.TrimSpace(content) == "" {
				content = summarizeCodexEvent(event.Type, event.Payload, item.Type)
			}
			if messageType == "log" && isSkippableCodexContent(content) {
				continue
			}
			rawJSON := enrichCodexRawJSON(line, workingDir)

			isResponseItemUser := item.Type == "message" && item.Role == "user"

			messages = append(messages, db.Message{
				Timestamp:   ts,
				Role:        role,
				MessageType: messageType,
				Model:       currentModel,
				Content:     content,
				RawJSON:     rawJSON,
			})
			if isResponseItemUser {
				responseItemUserIdx = append(responseItemUserIdx, len(messages)-1)
				if firstResponseItemUser == "" {
					if text := agent.NormalizeTitleCandidate(content); text != "" {
						firstResponseItemUser = text
					}
				}
			}

			if role == "user" {
				if rating, note := parseRatingDisplay(content); rating >= 0 {
					ratingEntries = append(ratingEntries, struct {
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
			messageType := "log"
			if msg.Type == "user_message" {
				role = "user"
				hasEventMsgUser = true
				if firstEventMsgUser == "" {
					if text := agent.NormalizeTitleCandidate(msg.Message); text != "" {
						firstEventMsgUser = text
					}
				}
			}
			if strings.TrimSpace(msg.Phase) == "final_answer" {
				messageType = db.MessageTypeFinalAnswer
			}
			content := strings.TrimSpace(msg.Message)
			if content == "" {
				content = summarizeCodexEvent(event.Type, event.Payload, msg.Type)
			}
			if messageType == "log" && isSkippableCodexContent(content) {
				continue
			}
			messages = append(messages, db.Message{
				Timestamp:   ts,
				Role:        role,
				MessageType: messageType,
				Model:       currentModel,
				Content:     content,
				RawJSON:     line,
			})
			if role == "user" {
				if rating, note := parseRatingDisplay(msg.Message); rating >= 0 {
					ratingEntries = append(ratingEntries, struct {
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
			// Check for $bb command.
			if strings.HasPrefix(content, "$bb ") {
				if rating, note := parseRatingDisplay(content); rating >= 0 {
					ratingEntries = append(ratingEntries, struct {
						rating    int
						note      string
						timestamp int64
					}{rating, note, ts})
				}
			}
			if firstLegacyUser == "" {
				if text := agent.NormalizeTitleCandidate(content); text != "" {
					firstLegacyUser = text
				}
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

			// Check user messages for $bb.
			if role == "user" && strings.HasPrefix(content, "$bb ") {
				if rating, note := parseRatingDisplay(content); rating >= 0 {
					ratingEntries = append(ratingEntries, struct {
						rating    int
						note      string
						timestamp int64
					}{rating, note, ts})
				}
			}
			if role == "user" && firstLegacyUser == "" {
				if text := agent.NormalizeTitleCandidate(content); text != "" {
					firstLegacyUser = text
				}
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
	messages = agent.AppendDiffDBMessagesWithOptions(messages, agent.DiffAppendOptions{
		Deduplicate:     true,
		UseAllJSONDiffs: true,
	})
	normalizeMessageTimestamps(messages)
	if cutoffMs > 0 {
		filteredMessages := make([]db.Message, 0, len(messages))
		for _, m := range messages {
			if m.Timestamp < cutoffMs {
				continue
			}
			filteredMessages = append(filteredMessages, m)
		}
		messages = filteredMessages

		filteredRatings := make([]struct {
			rating    int
			note      string
			timestamp int64
		}, 0, len(ratingEntries))
		for _, z := range ratingEntries {
			if z.timestamp < cutoffMs {
				continue
			}
			filteredRatings = append(filteredRatings, z)
		}
		ratingEntries = filteredRatings
	}

	// Derive thread ID from filename if not found in events.
	if threadID == "" {
		threadID = threadIDFromFilename(filepath.Base(path))
	}

	if threadID == "" || workingDir == "" {
		return false
	}
	// Skip conversations that have no user messages and no ratings — these
	// would appear empty in the UI.
	hasUserMessage := false
	for _, m := range messages {
		if m.Role == "user" {
			hasUserMessage = true
			break
		}
	}
	if !hasUserMessage && len(ratingEntries) == 0 {
		return false
	}

	if root, ok := agent.FindGitRoot(workingDir); ok {
		workingDir = root
	}

	projectID := ""
	if projectCache != nil {
		projectID = projectCache[workingDir]
	}
	if projectID == "" {
		var err error
		projectID, err = db.EnsureProject(ctx, a.DB, workingDir)
		if err != nil {
			log.Printf("codex watcher: ensure project %q: %v", workingDir, err)
			return false
		}
		if projectCache != nil {
			projectCache[workingDir] = projectID
		}
	}

	if err := db.EnsureConversation(ctx, a.DB, threadID, projectID, a.Name()); err != nil {
		log.Printf("codex watcher: ensure conversation %s: %v", threadID, err)
		return false
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
		title = agent.TitleFromPrompt(titlePrompt)
	}
	if title != "" {
		if err := db.UpdateConversationTitle(ctx, a.DB, threadID, title); err != nil {
			log.Printf("codex watcher: update title for %s: %v", threadID, err)
		}
	}

	// Fill in project and conversation IDs on messages.
	for i := range messages {
		messages[i].ProjectID = projectID
		messages[i].ConversationID = threadID
	}

	if len(messages) > 0 {
		if err := db.InsertMessages(ctx, a.DB, messages); err != nil {
			log.Printf("codex watcher: insert messages for session %s: %v", threadID, err)
		}
	}

	// Reconcile orphaned ratings.
	for _, z := range ratingEntries {
		if err := db.ReconcileOrphanedRating(ctx, a.DB, z.rating, z.note, z.timestamp, threadID); err != nil {
			log.Printf("codex watcher: reconcile rating for session %s: %v", threadID, err)
		}
	}
	return len(messages) > 0
}

// isSkippableCodexContent returns true for synthesized noise messages that
// provide no value in the conversation timeline.
func isSkippableCodexContent(content string) bool {
	return strings.HasPrefix(content, "[response_item:reasoning]") ||
		strings.HasPrefix(content, "[event_msg:item_completed]") ||
		strings.HasPrefix(content, "[event_msg:token_count]")
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

// parseRatingDisplay parses "$bb 4 optional note" into (4, "optional note").
// Returns (-1, "") if the format is invalid.
func parseRatingDisplay(display string) (int, string) {
	display = strings.TrimSpace(display)

	// Codex can render the command as markdown link: [$bb](...) 4 note
	if strings.HasPrefix(display, "[$bb](") {
		if i := strings.Index(display, ")"); i >= 0 {
			display = "$bb" + display[i+1:]
		}
	}

	if !strings.HasPrefix(display, "$bb ") {
		if i := strings.Index(display, "$bb "); i >= 0 {
			display = display[i:]
		}
	}
	if !strings.HasPrefix(display, "$bb ") {
		return -1, ""
	}

	rest := strings.TrimSpace(strings.TrimPrefix(display, "$bb"))
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

func normalizeSummaryTitle(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	title := agent.TitleFromPrompt(summary)
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

	if model := agent.FirstNonEmpty(
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
	return agent.FirstNonEmpty(
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

func enrichCodexRawJSON(rawLine string, cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return rawLine
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(rawLine), &obj); err != nil {
		return rawLine
	}

	if existing, ok := obj["cwd"].(string); ok && strings.TrimSpace(existing) != "" {
		return rawLine
	}
	obj["cwd"] = cwd

	b, err := json.Marshal(obj)
	if err != nil {
		return rawLine
	}
	return string(b)
}
