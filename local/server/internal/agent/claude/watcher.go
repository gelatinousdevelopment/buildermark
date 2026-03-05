package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"sort"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

const claudeWatcherSourceKindHistoryFile = "history_file"
const claudeWatcherSourceKindProjectFile = "project_file"

var unresolvedPasteRe = regexp.MustCompile(`\[Pasted text #\d+.*\]`)

// Run performs an initial scan (last 1 week) then polls for new data until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	log.Printf("claude watcher: starting, monitoring %s", a.path)

	scanWindow := agent.DefaultScanWindow
	if latestMs, err := db.LatestWatcherScanTimestamp(ctx, a.DB, a.Name()); err == nil {
		scanWindow = agent.StartupScanWindow(latestMs)
	}
	log.Printf("claude watcher: startup scan window %s", scanWindow)

	trackedFilter := a.trackedProjectFilter(ctx)
	start := time.Now()
	a.scanSinceFiltered(ctx, time.Now().Add(-scanWindow), trackedFilter)
	projectScanCount := a.scanProjectFilesSince(ctx, time.Now().Add(-scanWindow), true, trackedFilter, nil)
	log.Printf("claude watcher: startup scan duration %s", time.Since(start))
	if projectScanCount > 0 {
		log.Printf("claude watcher: startup project scan processed %d entries", projectScanCount)
	}
	a.backfillTitles(ctx)
	a.backfillParentConversations(ctx)
	a.BackfillGitIDs(ctx)
	a.BackfillLabels(ctx)
	a.backfillGitWorktreePaths(ctx)

	ticker := time.NewTicker(a.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("claude watcher: stopped")
			return
		case <-ticker.C:
			trackedFilter = a.trackedProjectFilter(ctx)
			a.pollFiltered(ctx, trackedFilter)
			a.pollProjectFiles(ctx, trackedFilter)
			a.BackfillGitIDs(ctx)
		}
	}
}

// DiscoverProjectPathsSince returns project paths inferred from Claude project
// conversation files modified since the given cutoff.
func (a *Agent) DiscoverProjectPathsSince(_ context.Context, since time.Time) []string {
	seen := make(map[string]struct{})
	cutoffMs := since.UnixMilli()

	if f, err := os.Open(a.path); err == nil {
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var entry historyEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			projectPath := strings.TrimSpace(entry.Project)
			if projectPath == "" || entry.Timestamp < cutoffMs {
				continue
			}
			seen[filepath.Clean(projectPath)] = struct{}{}
		}
		_ = f.Close()
	}

	paths := listProjectConversationFiles(a.Home)
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || info.ModTime().Before(since) {
			continue
		}
		projectPath := strings.TrimSpace(projectPathFromConversationFile(p))
		if projectPath == "" {
			continue
		}
		seen[filepath.Clean(projectPath)] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}

// ScanSince reads the entire file and imports entries with timestamps after
// the given cutoff. This is used by the API to trigger a historical scan.
func (a *Agent) ScanSince(ctx context.Context, since time.Time, progress agent.ScanProgressFunc) int {
	filter := a.trackedProjectFilter(ctx)
	n := a.doScan(ctx, since, false, filter)
	n += a.scanProjectFilesSince(ctx, since, false, filter, progress)
	a.backfillParentConversations(ctx)
	a.persistHistoryOffset(ctx, a.offset)
	log.Printf("claude watcher: manual scan processed %d entries (since %s)", n, since.Format(time.RFC3339))
	return n
}

// ScanPathsSince scans only entries for matching project paths.
func (a *Agent) ScanPathsSince(ctx context.Context, since time.Time, paths []string, progress agent.ScanProgressFunc) int {
	filter := agent.NewPathFilter(paths)
	n := a.doScan(ctx, since, false, filter)
	n += a.scanProjectFilesSince(ctx, since, false, filter, progress)
	a.backfillParentConversations(ctx)
	a.persistHistoryOffset(ctx, a.offset)
	log.Printf("claude watcher: manual path scan processed %d entries (since %s, paths=%d)", n, since.Format(time.RFC3339), len(paths))
	return n
}

// scanSince reads the entire file and processes only entries newer than the cutoff,
// then updates the file offset so subsequent polls start from the end.
func (a *Agent) scanSince(ctx context.Context, since time.Time) {
	a.scanSinceFiltered(ctx, since, nil)
}

func (a *Agent) scanSinceFiltered(ctx context.Context, since time.Time, filter agent.PathFilter) {
	n := a.doScan(ctx, since, true, filter)
	if n > 0 {
		log.Printf("claude watcher: initial scan processed %d entries", n)
	}
}

// doScan reads the entire file and processes entries newer than the cutoff.
// If updateOffset is true, it advances the file offset so subsequent polls
// start from the end of the file.
func (a *Agent) doScan(ctx context.Context, since time.Time, updateOffset bool, filter agent.PathFilter) int {
	startOffset := int64(0)
	if updateOffset {
		startOffset = a.restoreHistoryOffset(ctx)
	}

	entries, newOffset := a.readFrom(startOffset)
	cutoffMs := since.UnixMilli()
	var filtered []historyEntry
	for _, e := range entries {
		if e.Timestamp >= cutoffMs {
			if filter != nil && !filter.Match(e.Project) {
				continue
			}
			filtered = append(filtered, e)
		}
	}
	if len(filtered) > 0 {
		a.processEntries(ctx, filtered)
	}
	if updateOffset {
		a.offset = newOffset
		a.persistHistoryOffset(ctx, newOffset)
	}
	return len(filtered)
}


// poll reads new data appended since the last read. If the file shrank
// (rotation), it resets and rescans from the beginning.
func (a *Agent) poll(ctx context.Context) {
	a.pollFiltered(ctx, nil)
}

func (a *Agent) pollFiltered(ctx context.Context, filter agent.PathFilter) {
	info, err := os.Stat(a.path)
	if err != nil {
		return
	}

	size := info.Size()
	if size < a.offset {
		log.Println("claude watcher: file shrunk, rescanning")
		a.offset = 0
	}
	if size == a.offset {
		return
	}

	entries, newOffset := a.readFrom(a.offset)
	if len(entries) > 0 {
		filtered := entries
		if filter != nil {
			filtered = filtered[:0]
			for _, e := range entries {
				if filter.Match(e.Project) {
					filtered = append(filtered, e)
				}
			}
		}
		if len(filtered) > 0 {
			a.processEntries(ctx, filtered)
		}
	}
	a.offset = newOffset
	a.persistHistoryOffset(ctx, newOffset)
}

func (a *Agent) pollProjectFiles(ctx context.Context, filter agent.PathFilter) {
	n := a.scanProjectFilesSince(ctx, time.Time{}, true, filter, nil)
	if n > 0 {
		log.Printf("claude watcher: project poll processed %d entries", n)
	}
}

func (a *Agent) trackedProjectFilter(ctx context.Context) agent.PathFilter {
	return agent.TrackedProjectFilter(ctx, a.DB, func(p db.Project) []string {
		// Include worktree paths (populated at startup by backfillGitWorktreePaths)
		// so conversations in external worktrees pass the filter during polling.
		var extra []string
		for _, wt := range strings.Split(p.GitWorktreePaths, "\n") {
			wt = strings.TrimSpace(wt)
			if wt != "" {
				extra = append(extra, wt)
			}
		}
		return extra
	})
}

func (a *Agent) scanProjectFilesSince(ctx context.Context, since time.Time, updateOffset bool, filter agent.PathFilter, progress agent.ScanProgressFunc) int {
	paths := listProjectConversationFiles(a.Home)
	if len(paths) == 0 {
		return 0
	}

	cutoffMs := since.UnixMilli()
	processed := 0
	for _, path := range paths {
		if progress != nil {
			progress(path)
		}
		startOffset := int64(0)
		if updateOffset {
			startOffset = a.restoreProjectFileOffset(ctx, path)
		}

		entries, newOffset := a.readProjectFileFrom(path, startOffset)
		if len(entries) > 0 {
			filtered := make([]historyEntry, 0, len(entries))
			for _, e := range entries {
				if e.Timestamp < cutoffMs {
					continue
				}
				if filter != nil && !filter.Match(e.Project) {
					continue
				}
				filtered = append(filtered, e)
			}
			if len(filtered) > 0 {
				a.processEntries(ctx, filtered)
				processed += len(filtered)
			}
		}

		if updateOffset {
			a.persistProjectFileOffset(ctx, path, newOffset)
		}
	}
	return processed
}

func (a *Agent) readProjectFileFrom(path string, offset int64) ([]historyEntry, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, offset
	}

	size := info.Size()
	if size < offset {
		offset = 0
	}
	if size == offset {
		return nil, offset
	}

	buf := make([]byte, size-offset)
	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, offset
	}
	buf = buf[:n]

	entries := make([]historyEntry, 0, 64)
	sessionHint := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	projectHint := projectPathFromConversationFile(path)
	for _, line := range strings.Split(string(buf), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		entry, ok := parseProjectConversationLine(line)
		if !ok {
			entry, ok = parseSummaryConversationLine(line, sessionHint, projectHint)
		}
		if !ok {
			continue
		}
		if sessionHint == "" && strings.TrimSpace(entry.SessionID) != "" {
			sessionHint = strings.TrimSpace(entry.SessionID)
		}
		if projectHint == "" && strings.TrimSpace(entry.Project) != "" {
			projectHint = strings.TrimSpace(entry.Project)
		}
		entries = append(entries, entry)
	}

	return entries, offset + int64(n)
}

func parseSummaryConversationLine(line, sessionHint, projectHint string) (historyEntry, bool) {
	var entry conversationEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return historyEntry{}, false
	}
	if strings.TrimSpace(entry.Type) != "summary" {
		return historyEntry{}, false
	}
	summary := strings.TrimSpace(entry.Summary)
	if summary == "" {
		return historyEntry{}, false
	}

	sessionID := strings.TrimSpace(entry.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(sessionHint)
	}
	project := strings.TrimSpace(entry.Cwd)
	if project == "" {
		project = strings.TrimSpace(projectHint)
	}
	if sessionID == "" || project == "" {
		return historyEntry{}, false
	}

	ts := int64(0)
	if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(entry.Timestamp)); err == nil {
		ts = parsed.UnixMilli()
	}

	return historyEntry{
		Display:   summary,
		Timestamp: ts,
		SessionID: sessionID,
		Project:   project,
		Type:      "summary",
		Summary:   summary,
		RawJSON:   line,
	}, true
}

func projectPathFromConversationFile(path string) string {
	dirName := strings.TrimSpace(filepath.Base(filepath.Dir(path)))
	if dirName == "" || dirName == "." {
		return ""
	}
	if idx := strings.Index(dirName, worktreeDirMarker); idx >= 0 {
		dirName = dirName[:idx]
	}
	return strings.ReplaceAll(dirName, "-", "/")
}

func (a *Agent) restoreProjectFileOffset(ctx context.Context, path string) int64 {
	st, err := db.GetWatcherScanState(ctx, a.DB, a.Name(), claudeWatcherSourceKindProjectFile, path)
	if err != nil || st == nil {
		return 0
	}

	info, err := os.Stat(path)
	if err != nil {
		return 0
	}

	size := info.Size()
	mtimeMs := info.ModTime().UnixMilli()
	if st.FileSize == size && st.FileMtimeMs == mtimeMs {
		return size
	}
	if st.FileOffset > 0 && st.FileOffset <= size {
		return st.FileOffset
	}
	return 0
}

func (a *Agent) persistProjectFileOffset(ctx context.Context, path string, offset int64) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	_ = db.UpsertWatcherScanState(ctx, a.DB, db.WatcherScanState{
		Agent:       a.Name(),
		SourceKind:  claudeWatcherSourceKindProjectFile,
		SourceKey:   path,
		FileSize:    info.Size(),
		FileMtimeMs: info.ModTime().UnixMilli(),
		FileOffset:  offset,
	})
}

// readFrom reads the file starting at the given byte offset and returns parsed
// entries plus the new byte offset (end of file).
func (a *Agent) readFrom(offset int64) ([]historyEntry, int64) {
	f, err := os.Open(a.path)
	if err != nil {
		return nil, offset
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, offset
	}

	size := info.Size()
	if size <= offset {
		return nil, offset
	}

	buf := make([]byte, size-offset)
	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, offset
	}
	buf = buf[:n]

	var entries []historyEntry
	lines := strings.Split(string(buf), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry historyEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.SessionID == "" {
			continue
		}
		entry.RawJSON = line
		entries = append(entries, entry)
	}

	return entries, offset + int64(n)
}

func (a *Agent) restoreHistoryOffset(ctx context.Context) int64 {
	st, err := db.GetWatcherScanState(ctx, a.DB, a.Name(), claudeWatcherSourceKindHistoryFile, a.path)
	if err != nil || st == nil {
		return 0
	}

	info, err := os.Stat(a.path)
	if err != nil {
		return 0
	}

	size := info.Size()
	mtimeMs := info.ModTime().UnixMilli()
	if st.FileSize == size && st.FileMtimeMs == mtimeMs {
		return size
	}
	if st.FileOffset > 0 && st.FileOffset <= size {
		return st.FileOffset
	}
	return 0
}

func (a *Agent) persistHistoryOffset(ctx context.Context, offset int64) {
	info, err := os.Stat(a.path)
	if err != nil {
		return
	}

	_ = db.UpsertWatcherScanState(ctx, a.DB, db.WatcherScanState{
		Agent:       a.Name(),
		SourceKind:  claudeWatcherSourceKindHistoryFile,
		SourceKey:   a.path,
		FileSize:    info.Size(),
		FileMtimeMs: info.ModTime().UnixMilli(),
		FileOffset:  offset,
	})
}

// backfillTitles finds all conversations with empty titles and attempts to
// populate them from Claude's sessions-index.json files.
func (a *Agent) backfillTitles(ctx context.Context) {
	untitled, err := db.ListUntitledConversations(ctx, a.DB, a.Name())
	if err != nil {
		log.Printf("claude watcher: list untitled conversations: %v", err)
		return
	}

	updated := 0
	for _, u := range untitled {
		if title := readSessionTitle(a.Home, u.ProjectPath, u.ID); title != "" {
			if err := db.UpdateConversationTitle(ctx, a.DB, u.ID, title); err != nil {
				log.Printf("claude watcher: backfill title for %s: %v", u.ID, err)
				continue
			}
			updated++
		}
	}
	if updated > 0 {
		log.Printf("claude watcher: backfilled %d conversation titles", updated)
	}
}

// backfillParentConversations finds conversations with no parent set and
// attempts to detect a parent session ID from their conversation log files.
func (a *Agent) backfillParentConversations(ctx context.Context) {
	parentless, err := db.ListParentlessConversations(ctx, a.DB, a.Name())
	if err != nil {
		log.Printf("claude watcher: list parentless conversations: %v", err)
		return
	}

	updated := 0
	for _, u := range parentless {
		entries := readConversationLogEntries(a.Home, u.ProjectPath, u.ID)
		parentSessionID := extractParentSessionID(entries)
		if parentSessionID != "" && parentSessionID != u.ID {
			if err := db.UpdateConversationParent(ctx, a.DB, u.ID, parentSessionID); err != nil {
				log.Printf("claude watcher: backfill parent for %s: %v", u.ID, err)
				continue
			}
			updated++
		}
	}
	if updated > 0 {
		log.Printf("claude watcher: backfilled %d conversation parents", updated)
	}
}


// backfillGitWorktreePaths refreshes git_worktree_paths for all tracked projects.
// It discovers worktrees from two sources:
// 1. `git worktree list` (active worktrees)
// 2. Claude's ~/.claude/projects/ directory names (historical worktrees that may have been cleaned up)
func (a *Agent) backfillGitWorktreePaths(ctx context.Context) {
	projects, err := db.ListProjects(ctx, a.DB, false)
	if err != nil {
		log.Printf("claude watcher: list projects for worktree backfill: %v", err)
		return
	}

	// Build a set of worktree paths from Claude's project directories.
	claudeWorktrees := discoverClaudeWorktreePaths(a.Home)

	updated := 0
	for _, p := range projects {
		seen := make(map[string]struct{})
		cleanPath := filepath.Clean(p.Path)

		// Source 1: active git worktrees.
		for _, wt := range agent.ListGitWorktrees(p.Path) {
			wt = filepath.Clean(wt)
			if wt != cleanPath {
				seen[wt] = struct{}{}
			}
		}

		// Source 2: worktree paths from Claude's project directory names.
		if paths, ok := claudeWorktrees[cleanPath]; ok {
			for _, wt := range paths {
				seen[wt] = struct{}{}
			}
		}

		var sorted []string
		for wt := range seen {
			sorted = append(sorted, wt)
		}
		sort.Strings(sorted)
		newVal := strings.Join(sorted, "\n")
		if newVal != p.GitWorktreePaths {
			if err := db.UpdateProjectGitWorktreePaths(ctx, a.DB, p.ID, newVal); err != nil {
				log.Printf("claude watcher: update git_worktree_paths for %s: %v", p.ID, err)
				continue
			}
			updated++
		}
	}
	if updated > 0 {
		log.Printf("claude watcher: backfilled %d project git_worktree_paths", updated)
	}
}

// discoverClaudeWorktreePaths scans ~/.claude/projects/ for directories with
// the --claude-worktrees- marker and returns a map from parent project path
// to worktree paths.
func discoverClaudeWorktreePaths(home string) map[string][]string {
	projectsDir := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil
	}

	result := make(map[string][]string)
	for _, d := range entries {
		if !d.IsDir() {
			continue
		}
		name := d.Name()
		idx := strings.Index(name, worktreeDirMarker)
		if idx < 0 {
			continue
		}
		// Decode the parent project path from the directory prefix.
		parentDirName := name[:idx]
		parentPath := strings.ReplaceAll(parentDirName, "-", "/")

		// Decode the worktree suffix to reconstruct the full worktree path.
		// The full directory name is the encoded path including /.claude/worktrees/<name>,
		// so we can reconstruct the worktree path from it.
		wtSuffix := name[idx:]
		// --claude-worktrees-<name> → /.claude/worktrees/<name>
		wtSuffix = strings.Replace(wtSuffix, "--claude-worktrees-", "/.claude/worktrees/", 1)
		wtPath := parentPath + wtSuffix

		result[parentPath] = append(result[parentPath], wtPath)
	}
	return result
}


// processEntries groups entries by sessionId and upserts projects,
// conversations, and messages for each session.
func (a *Agent) processEntries(ctx context.Context, entries []historyEntry) {
	type sessionGroup struct {
		project string
		entries []historyEntry
	}
	sessions := make(map[string]*sessionGroup)
	order := make([]string, 0)

	for _, e := range entries {
		g, ok := sessions[e.SessionID]
		if !ok {
			g = &sessionGroup{project: e.Project}
			sessions[e.SessionID] = g
			order = append(order, e.SessionID)
		}
		if g.project == "" && e.Project != "" {
			g.project = e.Project
		}
		g.entries = append(g.entries, e)
	}

	gitRootCache := agent.NewGitRootCache()
	projectIDCache := make(map[string]string)
	sessionTitleCache := make(map[string]string)
	conversationLogCache := make(map[string][]conversationLogEntry)

	for _, sid := range order {
		g := sessions[sid]
		if g.project == "" {
			continue
		}

		normalizedProject := gitRootCache.Resolve(normalizeWorktreePath(g.project))
		projectID := projectIDCache[normalizedProject]
		if projectID == "" {
			var err error
			projectID, err = db.EnsureProject(ctx, a.DB, normalizedProject)
			if err != nil {
				log.Printf("claude watcher: ensure project %q: %v", normalizedProject, err)
				continue
			}
			projectIDCache[normalizedProject] = projectID
		}

		if err := db.EnsureConversation(ctx, a.DB, sid, projectID, a.Name()); err != nil {
			log.Printf("claude watcher: ensure conversation %s: %v", sid, err)
			continue
		}

		cacheKey := g.project + "\n" + sid
		if _, ok := conversationLogCache[cacheKey]; !ok {
			conversationLogCache[cacheKey] = readConversationLogEntries(a.Home, g.project, sid)
		}
		parentSessionID := extractParentSessionID(conversationLogCache[cacheKey])
		if parentSessionID != "" && parentSessionID != sid {
			db.UpdateConversationParent(ctx, a.DB, sid, parentSessionID)
		}

		if _, ok := sessionTitleCache[cacheKey]; !ok {
			// Highest priority: inline summary from type="summary" entries.
			var title string
			for _, e := range g.entries {
				if e.Type == "summary" && strings.TrimSpace(e.Summary) != "" {
					title = strings.TrimSpace(e.Summary)
				}
			}
			if title == "" {
				title = readSummaryFromConversationFile(a.Home, g.project, sid)
			}
			if title == "" {
				title = readSessionSummaryFromIndex(a.Home, g.project, sid)
			}
			if title == "" {
				title = titleFromConversationLogs(conversationLogCache[cacheKey])
			}
			sessionTitleCache[cacheKey] = title
		}

		if title := sessionTitleCache[cacheKey]; title != "" {
			if err := db.UpdateConversationTitle(ctx, a.DB, sid, title); err != nil {
				log.Printf("claude watcher: update title for %s: %v", sid, err)
			}
		}

		messages := make([]db.Message, 0, len(g.entries)+1)

		// Check conversation file for a first prompt that history.jsonl may have missed
		// (e.g. plan-mode auto-submissions).
		if firstText, firstTs := firstPromptFromConversationLogs(conversationLogCache[cacheKey]); firstText != "" {
			alreadyPresent := false
			for _, e := range g.entries {
				if e.Display == firstText {
					alreadyPresent = true
					break
				}
			}
			if !alreadyPresent {
				rawJSON, _ := json.Marshal(map[string]any{
					"display":   firstText,
					"timestamp": firstTs,
					"sessionId": sid,
					"project":   g.project,
					"source":    "conversation_file",
				})
				messages = append(messages, db.Message{
					Timestamp:      firstTs,
					ProjectID:      projectID,
					ConversationID: sid,
					Role:           "user",
					Content:        firstText,
					RawJSON:        string(rawJSON),
				})
			}
		}

		for _, e := range g.entries {
			// Skip entry types that never carry conversational content.
			if e.Type == "progress" {
				continue
			}

			role := "user"
			if isAssistantAuthoredHistoryEntry(e) {
				role = "agent"
			}

			display := e.Display
			if e.Type == "summary" && strings.TrimSpace(e.Summary) != "" {
				display = strings.TrimSpace(e.Summary)
			}
			if len(e.PastedContents) > 0 {
				display = resolvePastedContents(a.Home, display, e.PastedContents)
			}

			// Skip entries with no content and system/meta messages.
			if strings.TrimSpace(display) == "" && e.Type != "summary" {
				continue
			}
			if strings.TrimSpace(display) != "" && isSystemMessage(strings.TrimSpace(display)) {
				continue
			}
			// Skip user messages with unresolved paste placeholders — the conversation
			// file will have the fully resolved version.
			if role == "user" && unresolvedPasteRe.MatchString(display) {
				continue
			}

			if strings.TrimSpace(display) == "" {
				display = "[" + strings.TrimSpace(e.Type) + "]"
			}

			rawJSON := strings.TrimSpace(e.RawJSON)
			if rawJSON == "" {
				b, _ := json.Marshal(e)
				rawJSON = string(b)
			}
			if e.Type == "summary" {
				if summary := strings.TrimSpace(e.Summary); summary != "" {
					display = summary
				} else if summary := extractSummaryFromJSONLine(rawJSON); summary != "" {
					display = summary
				}
			}
			ts := e.Timestamp
			if ts <= 0 {
				ts = nextMessageTimestamp(messages)
			}

			messages = append(messages, db.Message{
				Timestamp:      ts,
				ProjectID:      projectID,
				ConversationID: sid,
				Role:           role,
				Model:          mapRoleModel(role, historyEntryModel(e)),
				Content:        display,
				RawJSON:        rawJSON,
			})
		}

		historyCount := len(messages)
		for _, e := range conversationLogCache[cacheKey] {
			alreadyPresent := false
			for _, m := range messages[:historyCount] {
				if m.Role == e.Role && m.Content == e.Content {
					alreadyPresent = true
					break
				}
			}
			if alreadyPresent {
				continue
			}

			rawJSON, _ := json.Marshal(map[string]any{
				"type":      e.Type,
				"timestamp": e.Timestamp,
				"content":   e.Content,
				"source":    "conversation_file",
			})
			if e.RawJSON != "" {
				rawJSON = []byte(e.RawJSON)
			}

			messages = append(messages, db.Message{
				Timestamp:      normalizeMessageTimestamp(e.Timestamp, messages),
				ProjectID:      projectID,
				ConversationID: sid,
				Role:           e.Role,
				Model:          mapRoleModel(e.Role, extractModelFromJSONLine(string(rawJSON))),
				Content:        e.Content,
				RawJSON:        string(rawJSON),
			})
		}

		messages = appendDiffDBMessages(messages)

		if err := db.InsertMessages(ctx, a.DB, messages); err != nil {
			log.Printf("claude watcher: insert messages for session %s: %v", sid, err)
		}

		// Reconcile orphaned ratings: if any entry in this session is a /bb
		// command, find the corresponding orphaned rating and re-link it.
		for _, e := range g.entries {
			if !isRatingDisplay(e.Display) {
				continue
			}
			rating, note := parseRatingDisplay(e.Display)
			if rating < 0 {
				continue
			}
			if err := db.ReconcileOrphanedRating(ctx, a.DB, rating, note, e.Timestamp, sid); err != nil {
				log.Printf("claude watcher: reconcile rating for session %s: %v", sid, err)
			}
		}
	}
}

func normalizeMessageTimestamp(ts int64, existing []db.Message) int64 {
	if ts > 0 {
		return ts
	}
	return nextMessageTimestamp(existing)
}

func nextMessageTimestamp(existing []db.Message) int64 {
	maxTS := int64(0)
	for _, m := range existing {
		if m.Timestamp > maxTS {
			maxTS = m.Timestamp
		}
	}
	if maxTS <= 0 {
		return 1
	}
	return maxTS + 1
}

func extractSummaryFromJSONLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		return ""
	}
	s, _ := payload["summary"].(string)
	return strings.TrimSpace(s)
}

func firstPromptFromConversationLogs(entries []conversationLogEntry) (string, int64) {
	for _, e := range entries {
		if e.Role != "user" {
			continue
		}
		text := strings.TrimSpace(e.Content)
		if text == "" || isSystemMessage(text) {
			continue
		}
		return text, e.Timestamp
	}
	return "", 0
}

func titleFromConversationLogs(entries []conversationLogEntry) string {
	// Check all user messages for a plan title.
	for _, e := range entries {
		if e.Role != "user" {
			continue
		}
		text := strings.TrimSpace(e.Content)
		if text == "" || isSystemMessage(text) {
			continue
		}
		if title := agent.TitleFromPlanPrompt(text); title != "" {
			return title
		}
	}
	text, _ := firstPromptFromConversationLogs(entries)
	if text == "" {
		return ""
	}
	return agent.TitleFromPrompt(text)
}

func mapRoleModel(role, model string) string {
	if role != "agent" {
		return ""
	}
	return strings.TrimSpace(model)
}

// parseRatingDisplay parses "/bb 4 optional note" or "/bb:rate 4 optional note"
// into (4, "optional note"). Returns (-1, "") if the format is invalid or has no args.
func parseRatingDisplay(display string) (int, string) {
	rest := display
	switch {
	case strings.HasPrefix(rest, "/bb:rate "):
		rest = strings.TrimPrefix(rest, "/bb:rate ")
	case strings.HasPrefix(rest, "/bbrate "):
		rest = strings.TrimPrefix(rest, "/bbrate ")
	case strings.HasPrefix(rest, "/bb "):
		rest = strings.TrimPrefix(rest, "/bb ")
	default:
		return -1, ""
	}
	rest = strings.TrimSpace(rest)
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
	return rating, strings.TrimSpace(note)
}
