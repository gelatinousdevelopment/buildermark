package cursor

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

const workspaceMapCacheTTL = 5 * time.Minute

type scanState struct {
	Conversations map[string]int64 `json:"conversations"` // composerID -> lastUpdatedAt
}

const cursorWatcherSourceKindScanMarker = "scan_marker"

func (a *Agent) Run(ctx context.Context) {
	if !a.globalDBExists() {
		log.Printf("cursor watcher: global DB not found at %s, skipping", a.globalDBPath)
		return
	}
	log.Printf("cursor watcher: starting, monitoring %s", a.globalDBPath)

	scanWindow := a.startupScanWindow(ctx)
	log.Printf("cursor watcher: startup scan window %s", scanWindow)

	scanCutoff := time.Now().Add(-scanWindow)
	trackedFilter := agent.TrackedProjectFilter(ctx, a.DB, nil)
	a.scanSince(ctx, scanCutoff, trackedFilter)
	a.persistStartupScanMarker(ctx)
	a.BackfillGitIDs(ctx)
	a.BackfillLabels(ctx)

	// Load previous scan state for incremental polling.
	var state scanState
	if raw, err := db.GetWatcherScanState(ctx, a.DB, a.Name(), "cursor_state", "conversations"); err == nil && raw != nil && raw.StateJSON != "" {
		_ = json.Unmarshal([]byte(raw.StateJSON), &state)
	}
	if state.Conversations == nil {
		state.Conversations = make(map[string]int64)
	}

	ticker := time.NewTicker(a.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("cursor watcher: stopped")
			return
		case <-ticker.C:
			pollStart := time.Now()
			trackedFilter = agent.TrackedProjectFilter(ctx, a.DB, nil)
			filterDone := time.Now()
			count := a.poll(ctx, &state, trackedFilter)
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
			log.Printf("cursor watcher: poll took %s (filter=%s composers=%s[%d] tracked=%d) next=%s",
				f(total),
				f(filterDone.Sub(pollStart)),
				f(pollDone.Sub(filterDone)), count,
				len(state.Conversations),
				f(newInterval))
			ticker.Reset(newInterval)
		}
	}
}

// DiscoverProjectPathsSince returns project paths from Cursor workspaces
// that have composers modified since the given cutoff.
func (a *Agent) DiscoverProjectPathsSince(_ context.Context, since time.Time) []string {
	if !a.globalDBExists() {
		return nil
	}
	wsMap := a.buildWorkspaceMap()
	globalDB, err := openCursorDB(a.globalDBPath)
	if err != nil {
		return nil
	}
	defer globalDB.Close()

	sinceMs := since.UnixMilli()
	seen := make(map[string]struct{})
	for composerID, projectPath := range wsMap {
		cd, err := readComposerData(globalDB, composerID)
		if err != nil || cd == nil {
			continue
		}
		if cd.LastUpdatedAt > 0 && cd.LastUpdatedAt < sinceMs {
			continue
		}
		if projectPath != "" {
			seen[filepath.Clean(projectPath)] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}

func (a *Agent) ScanSince(ctx context.Context, since time.Time, progress agent.ScanProgressFunc) int {
	filter := agent.TrackedProjectFilter(ctx, a.DB, nil)
	n := a.doScan(ctx, since, filter, progress)
	a.persistStartupScanMarker(ctx)
	log.Printf("cursor watcher: manual scan processed %d composers (since %s)", n, since.Format(time.RFC3339))
	return n
}

func (a *Agent) ScanPathsSince(ctx context.Context, since time.Time, paths []string, progress agent.ScanProgressFunc) int {
	n := a.doScan(ctx, since, agent.NewPathFilter(paths), progress)
	a.persistStartupScanMarker(ctx)
	log.Printf("cursor watcher: manual path scan processed %d composers (since %s, paths=%d)", n, since.Format(time.RFC3339), len(paths))
	return n
}

func (a *Agent) startupScanWindow(ctx context.Context) time.Duration {
	scanWindow := agent.DefaultScanWindow
	latestMs, err := db.LatestWatcherScanTimestampForScopes(ctx, a.DB, a.Name(),
		db.WatcherScanScope{SourceKind: cursorWatcherSourceKindScanMarker, SourceKey: a.startupScanMarkerKey()},
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
		SourceKind: cursorWatcherSourceKindScanMarker,
		SourceKey:  a.startupScanMarkerKey(),
	})
}

func (a *Agent) scanSince(ctx context.Context, since time.Time, filter agent.PathFilter) {
	n := a.doScan(ctx, since, filter, nil)
	if n > 0 {
		log.Printf("cursor watcher: initial scan processed %d composers", n)
	}
}

func (a *Agent) doScan(ctx context.Context, since time.Time, filter agent.PathFilter, progress agent.ScanProgressFunc) int {
	if !a.globalDBExists() {
		return 0
	}
	globalDB, err := openCursorDB(a.globalDBPath)
	if err != nil {
		log.Printf("cursor watcher: open global DB: %v", err)
		return 0
	}
	defer globalDB.Close()

	wsMap := a.buildWorkspaceMap()
	composerIDs := listComposerIDs(globalDB)
	sinceMs := since.UnixMilli()
	processed := 0

	for _, id := range composerIDs {
		if progress != nil {
			progress(id)
		}
		cd, err := readComposerData(globalDB, id)
		if err != nil || cd == nil {
			continue
		}
		if sinceMs > 0 && cd.LastUpdatedAt > 0 && cd.LastUpdatedAt < sinceMs {
			continue
		}
		projectPath := a.resolveProjectPath(cd, wsMap)
		if filter != nil && !filter.Match(projectPath) {
			continue
		}
		if a.processComposer(ctx, globalDB, cd, projectPath) {
			processed++
		}
	}
	return processed
}

func (a *Agent) poll(ctx context.Context, state *scanState, filter agent.PathFilter) int {
	if !a.globalDBExists() {
		return 0
	}
	globalDB, err := openCursorDB(a.globalDBPath)
	if err != nil {
		return 0
	}
	defer globalDB.Close()

	wsMap := a.getWorkspaceMapCached()
	composerIDs := listComposerIDs(globalDB)
	processed := 0

	for _, id := range composerIDs {
		cd, err := readComposerData(globalDB, id)
		if err != nil || cd == nil {
			continue
		}
		if prev, ok := state.Conversations[id]; ok && cd.LastUpdatedAt <= prev {
			continue
		}

		projectPath := a.resolveProjectPath(cd, wsMap)
		if filter != nil && !filter.Match(projectPath) {
			state.Conversations[id] = cd.LastUpdatedAt
			continue
		}

		if a.processComposer(ctx, globalDB, cd, projectPath) {
			processed++
		}
		state.Conversations[id] = cd.LastUpdatedAt
	}

	// Persist scan state.
	if processed > 0 {
		if raw, err := json.Marshal(state); err == nil {
			_ = db.UpsertWatcherScanState(ctx, a.DB, db.WatcherScanState{
				Agent:      a.Name(),
				SourceKind: "cursor_state",
				SourceKey:  "conversations",
				StateJSON:  string(raw),
			})
		}
	}
	return processed
}

func (a *Agent) processComposer(ctx context.Context, globalDB *sql.DB, cd *composerData, projectPath string) bool {
	if cd.ComposerID == "" {
		return false
	}

	if projectPath == "" {
		projectPath = "unknown"
	}
	if projectPath != "unknown" {
		if root, ok := agent.FindGitRoot(projectPath); ok {
			projectPath = root
		}
	}

	projectID, err := db.EnsureProject(ctx, a.DB, projectPath)
	if err != nil {
		log.Printf("cursor watcher: ensure project %q: %v", projectPath, err)
		return false
	}

	// Extract messages from both formats.
	messages := a.extractMessages(globalDB, cd, projectID)
	if len(messages) == 0 {
		return false
	}

	// Skip conversations with no user messages.
	hasUserMessage := false
	for _, m := range messages {
		if m.Role == "user" {
			hasUserMessage = true
			break
		}
	}
	if !hasUserMessage {
		return false
	}

	if err := db.EnsureConversation(ctx, a.DB, cd.ComposerID, projectID, a.Name()); err != nil {
		log.Printf("cursor watcher: ensure conversation %s: %v", cd.ComposerID, err)
		return false
	}
	if err := db.UpdateConversationProject(ctx, a.DB, cd.ComposerID, projectID); err != nil {
		log.Printf("cursor watcher: update project for %s: %v", cd.ComposerID, err)
	}

	// Use composer name or first user message as title.
	title := strings.TrimSpace(cd.Name)
	if title == "" {
		for _, m := range messages {
			if m.Role == "user" {
				title = agent.TitleFromPrompt(m.Content)
				break
			}
		}
	}
	if title != "" {
		if err := db.UpdateConversationTitle(ctx, a.DB, cd.ComposerID, title); err != nil {
			log.Printf("cursor watcher: update title for %s: %v", cd.ComposerID, err)
		}
	}

	messages = agent.AppendDiffDBMessagesWithOptions(messages, agent.DiffAppendOptions{
		UseAllJSONDiffs: true,
		Deduplicate:     true,
	})
	if err := db.InsertMessages(ctx, a.DB, messages); err != nil {
		log.Printf("cursor watcher: insert messages for %s: %v", cd.ComposerID, err)
	}

	return true
}

func (a *Agent) extractMessages(globalDB *sql.DB, cd *composerData, projectID string) []db.Message {
	// Prefer new format (inline conversation array).
	if len(cd.Conversation) > 0 {
		return a.extractNewFormatMessages(cd, projectID)
	}
	// Fall back to old format (headers + individual bubble lookups).
	if len(cd.FullConversationHeadersOnly) > 0 {
		return a.extractOldFormatMessages(globalDB, cd, projectID)
	}
	return nil
}

func (a *Agent) extractNewFormatMessages(cd *composerData, projectID string) []db.Message {
	// Build timings slice and interpolate timestamps across all bubbles.
	timings := make([]*bubbleTimingInfo, len(cd.Conversation))
	for i, bubble := range cd.Conversation {
		timings[i] = bubble.TimingInfo
	}
	timestamps := interpolateTimestamps(timings, cd.CreatedAt, cd.LastUpdatedAt)

	messages := make([]db.Message, 0, len(cd.Conversation))
	for i, bubble := range cd.Conversation {
		content := extractBubbleText(bubble.Text, bubble.ToolFormerData)
		if content == "" {
			continue
		}
		role := bubbleRole(bubble.Type)
		ts := timestamps[i]
		if ts <= 0 {
			continue
		}

		rawJSON, _ := json.Marshal(bubble)
		messages = append(messages, db.Message{
			Timestamp:      ts,
			ProjectID:      projectID,
			ConversationID: cd.ComposerID,
			Role:           role,
			Content:        content,
			RawJSON:        buildEnrichedRawJSON(string(rawJSON)),
		})
	}
	return messages
}

func (a *Agent) extractOldFormatMessages(globalDB *sql.DB, cd *composerData, projectID string) []db.Message {
	n := len(cd.FullConversationHeadersOnly)

	// Pass 1: read all bubble data and collect timings.
	type bubbleEntry struct {
		bd     *bubbleData
		rawStr string
	}
	entries := make([]*bubbleEntry, n)
	timings := make([]*bubbleTimingInfo, n)
	for i, header := range cd.FullConversationHeadersOnly {
		bd, rawStr, err := readBubbleData(globalDB, cd.ComposerID, header.BubbleID)
		if err != nil || bd == nil {
			continue
		}
		entries[i] = &bubbleEntry{bd: bd, rawStr: rawStr}
		timings[i] = bd.TimingInfo
	}

	// Interpolate timestamps across all bubbles.
	timestamps := interpolateTimestamps(timings, cd.CreatedAt, cd.LastUpdatedAt)

	// Pass 2: build messages using interpolated timestamps.
	messages := make([]db.Message, 0, n)
	for i, entry := range entries {
		if entry == nil {
			continue
		}
		content := extractBubbleText(entry.bd.Text, entry.bd.ToolFormerData)
		if content == "" {
			continue
		}
		role := bubbleRole(entry.bd.Type)
		ts := timestamps[i]
		if ts <= 0 {
			continue
		}

		messages = append(messages, db.Message{
			Timestamp:      ts,
			ProjectID:      projectID,
			ConversationID: cd.ComposerID,
			Role:           role,
			Content:        content,
			RawJSON:        buildEnrichedRawJSON(entry.rawStr),
		})
	}
	return messages
}

// resolveProjectPath determines the project path for a composer.
func (a *Agent) resolveProjectPath(cd *composerData, wsMap map[string]string) string {
	// 1. Check workspace map.
	if p, ok := wsMap[cd.ComposerID]; ok && p != "" {
		return p
	}

	// 2. Infer from file context.
	if cd.Context != nil && len(cd.Context.FileSelections) > 0 {
		if p := inferProjectFromFileSelections(cd.Context.FileSelections); p != "" {
			if root, ok := agent.FindGitRoot(p); ok {
				return root
			}
			return p
		}
	}

	return ""
}

// inferProjectFromFileSelections finds the common path prefix from file selections.
func inferProjectFromFileSelections(selections []fileSelection) string {
	if len(selections) == 0 {
		return ""
	}
	paths := make([]string, 0, len(selections))
	for _, s := range selections {
		p := strings.TrimSpace(s.URI.FsPath)
		if p != "" {
			paths = append(paths, filepath.Dir(p))
		}
	}
	if len(paths) == 0 {
		return ""
	}
	common := paths[0]
	for _, p := range paths[1:] {
		common = commonPathPrefix(common, p)
		if common == "" {
			break
		}
	}
	return common
}

// commonPathPrefix returns the longest common directory prefix of two paths.
func commonPathPrefix(a, b string) string {
	cleanA := filepath.Clean(a)
	cleanB := filepath.Clean(b)
	partsA := strings.Split(cleanA, string(filepath.Separator))
	partsB := strings.Split(cleanB, string(filepath.Separator))
	n := len(partsA)
	if len(partsB) < n {
		n = len(partsB)
	}
	common := make([]string, 0, n)
	for i := 0; i < n; i++ {
		if partsA[i] != partsB[i] {
			break
		}
		common = append(common, partsA[i])
	}
	if len(common) == 0 {
		return ""
	}
	// On absolute paths, Split produces "" as first element. If the only
	// common part is that empty root element, the paths share no real prefix.
	if len(common) == 1 && common[0] == "" {
		return ""
	}
	return strings.Join(common, string(filepath.Separator))
}

// --- Workspace map ---

func (a *Agent) getWorkspaceMapCached() map[string]string {
	if a.cachedWorkspaceMap != nil && time.Since(a.cachedWorkspaceMapTime) < workspaceMapCacheTTL {
		return a.cachedWorkspaceMap
	}
	a.cachedWorkspaceMap = a.buildWorkspaceMap()
	a.cachedWorkspaceMapTime = time.Now()
	return a.cachedWorkspaceMap
}

// buildWorkspaceMap iterates workspaceStorage directories and maps composerIDs
// to project paths.
func (a *Agent) buildWorkspaceMap() map[string]string {
	result := make(map[string]string)
	entries, err := os.ReadDir(a.workspaceDir)
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		wsDir := filepath.Join(a.workspaceDir, entry.Name())

		// Read workspace.json to get project path.
		projectPath := readWorkspacePath(wsDir)
		if projectPath == "" {
			continue
		}

		// Read composer list from workspace DB.
		composerIDs := readWorkspaceComposerIDs(wsDir)
		for _, id := range composerIDs {
			result[id] = projectPath
		}
	}
	return result
}

// readWorkspacePath reads workspace.json and returns the project path.
func readWorkspacePath(wsDir string) string {
	data, err := os.ReadFile(filepath.Join(wsDir, "workspace.json"))
	if err != nil {
		return ""
	}
	var ws struct {
		Folder    string `json:"folder"`
		Workspace string `json:"workspace"`
	}
	if err := json.Unmarshal(data, &ws); err != nil {
		return ""
	}
	if ws.Folder != "" {
		return workspaceFolderToPath(ws.Folder)
	}
	if ws.Workspace != "" {
		return workspaceFolderToPath(ws.Workspace)
	}
	return ""
}

// readWorkspaceComposerIDs reads the per-workspace state.vscdb to get composer IDs.
func readWorkspaceComposerIDs(wsDir string) []string {
	dbPath := filepath.Join(wsDir, "state.vscdb")
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	wsDB, err := openCursorDB(dbPath)
	if err != nil {
		return nil
	}
	defer wsDB.Close()

	var raw string
	err = wsDB.QueryRow("SELECT value FROM ItemTable WHERE key = 'composer.composerData'").Scan(&raw)
	if err != nil {
		return nil
	}

	var wcd workspaceComposerData
	if err := json.Unmarshal([]byte(raw), &wcd); err != nil {
		return nil
	}

	ids := make([]string, 0, len(wcd.AllComposers))
	for _, c := range wcd.AllComposers {
		if c.ComposerID != "" {
			ids = append(ids, c.ComposerID)
		}
	}
	return ids
}

// --- Cursor DB helpers ---

// openCursorDB opens a Cursor state.vscdb in read-only mode.
func openCursorDB(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", "file:"+path+"?mode=ro")
}

// listComposerIDs returns all composer IDs from the global DB.
func listComposerIDs(globalDB *sql.DB) []string {
	rows, err := globalDB.Query("SELECT key FROM cursorDiskKV WHERE key LIKE 'composerData:%'")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			continue
		}
		id := strings.TrimPrefix(key, "composerData:")
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// readComposerData reads a single composerData entry from the global DB.
func readComposerData(globalDB *sql.DB, composerID string) (*composerData, error) {
	var raw string
	err := globalDB.QueryRow("SELECT value FROM cursorDiskKV WHERE key = ?", "composerData:"+composerID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	return parseComposerData([]byte(raw))
}

// readBubbleData reads a single bubble's data from the global DB (old format).
// It returns the parsed struct and the raw JSON string for use as RawJSON.
func readBubbleData(globalDB *sql.DB, composerID, bubbleID string) (*bubbleData, string, error) {
	key := "bubbleId:" + composerID + ":" + bubbleID
	var raw string
	err := globalDB.QueryRow("SELECT value FROM cursorDiskKV WHERE key = ?", key).Scan(&raw)
	if err != nil {
		return nil, "", err
	}
	var bd bubbleData
	if err := json.Unmarshal([]byte(raw), &bd); err != nil {
		return nil, "", err
	}
	return &bd, raw, nil
}
