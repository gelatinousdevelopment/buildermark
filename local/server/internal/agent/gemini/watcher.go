package gemini

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

type processedFile struct {
	modTime time.Time
}

func (a *Agent) Run(ctx context.Context) {
	log.Printf("gemini watcher: starting, monitoring %s", a.tmpDir)

	scanWindow := agent.DefaultScanWindow
	if latestMs, err := db.LatestWatcherScanTimestamp(ctx, a.db, a.Name()); err == nil {
		scanWindow = agent.StartupScanWindow(latestMs)
	}
	log.Printf("gemini watcher: startup scan window %s", scanWindow)

	trackedFilter := a.trackedProjectFilter(ctx)
	a.scanSince(ctx, time.Now().Add(-scanWindow), trackedFilter)
	// Write a scan marker so future restarts can compute a narrow window.
	_ = db.UpsertWatcherScanState(ctx, a.db, db.WatcherScanState{
		Agent:      a.Name(),
		SourceKind: "scan_marker",
		SourceKey:  "startup",
	})
	a.backfillGitIDs(ctx)
	a.backfillLabels(ctx)

	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	seen := make(map[string]processedFile)

	for {
		select {
		case <-ctx.Done():
			log.Println("gemini watcher: stopped")
			return
		case <-ticker.C:
			trackedFilter = a.trackedProjectFilter(ctx)
			a.poll(ctx, seen, trackedFilter)
			a.backfillGitIDs(ctx)
		}
	}
}

// DiscoverProjectPathsSince returns local project paths resolved from Gemini
// session files modified since the given cutoff.
func (a *Agent) DiscoverProjectPathsSince(_ context.Context, since time.Time) []string {
	files := a.listSessionFiles(since)
	seen := make(map[string]struct{})
	for _, path := range files {
		conv, err := readConversation(path)
		if err != nil {
			continue
		}
		projectPath := strings.TrimSpace(a.resolveProjectPath(conv))
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

func (a *Agent) ScanSince(ctx context.Context, since time.Time, progress agent.ScanProgressFunc) int {
	n := a.doScan(ctx, since, nil, progress, false)
	log.Printf("gemini watcher: manual scan processed %d files (since %s)", n, since.Format(time.RFC3339))
	return n
}

// ScanPathsSince scans only session files that resolve to matching project paths.
func (a *Agent) ScanPathsSince(ctx context.Context, since time.Time, paths []string, progress agent.ScanProgressFunc) int {
	n := a.doScan(ctx, since, newPathFilter(paths), progress, false)
	log.Printf("gemini watcher: manual path scan processed %d files (since %s, paths=%d)", n, since.Format(time.RFC3339), len(paths))
	return n
}

func (a *Agent) scanSince(ctx context.Context, since time.Time, filter pathFilter) {
	n := a.doScan(ctx, since, filter, nil, true)
	if n > 0 {
		log.Printf("gemini watcher: initial scan processed %d files", n)
	}
}

func (a *Agent) doScan(ctx context.Context, since time.Time, filter pathFilter, progress agent.ScanProgressFunc, useModTime bool) int {
	listSince := time.Time{}
	if useModTime {
		listSince = since
	}
	files := a.listSessionFiles(listSince)
	processed := 0
	for _, path := range files {
		if progress != nil {
			progress(path)
		}
		if filter != nil {
			conv, err := readConversation(path)
			if err != nil {
				continue
			}
			projectPath := a.resolveProjectPath(conv)
			if !filter.match(projectPath) {
				continue
			}
		}
		if a.processSessionFileSince(ctx, path, since) {
			processed++
		}
	}
	return processed
}

func (a *Agent) poll(ctx context.Context, seen map[string]processedFile, filter pathFilter) {
	filepath.Walk(a.tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".json") || !strings.Contains(path, string(filepath.Separator)+"chats"+string(filepath.Separator)) {
			return nil
		}

		modTime := info.ModTime()
		if prev, ok := seen[path]; ok && !modTime.After(prev.modTime) {
			return nil
		}

		if filter != nil {
			conv, err := readConversation(path)
			if err != nil || !filter.match(a.resolveProjectPath(conv)) {
				seen[path] = processedFile{modTime: modTime}
				return nil
			}
		}

		a.processSessionFile(ctx, path)
		seen[path] = processedFile{modTime: modTime}
		return nil
	})
}

func (a *Agent) listSessionFiles(since time.Time) []string {
	var files []string
	filepath.Walk(a.tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".json") || !strings.Contains(path, string(filepath.Separator)+"chats"+string(filepath.Separator)) {
			return nil
		}
		if info.ModTime().Before(since) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files
}

func (a *Agent) processSessionFile(ctx context.Context, path string) {
	_ = a.processSessionFileSince(ctx, path, time.Time{})
}

func (a *Agent) processSessionFileSince(ctx context.Context, path string, since time.Time) bool {
	conv, err := readConversation(path)
	if err != nil {
		return false
	}
	if conv.SessionID == "" {
		return false
	}
	sessionModel := strings.TrimSpace(conv.Model)

	projectPath := a.resolveProjectPath(conv)
	if projectPath == "" {
		projectPath = "unknown"
	}
	if projectPath != "unknown" {
		if root, ok := agent.FindGitRoot(projectPath); ok {
			projectPath = root
		}
	}

	projectID, err := db.EnsureProject(ctx, a.db, projectPath)
	if err != nil {
		log.Printf("gemini watcher: ensure project %q: %v", projectPath, err)
		return false
	}
	cutoffMs := int64(0)
	if !since.IsZero() {
		cutoffMs = since.UnixMilli()
	}

	messages := make([]db.Message, 0, len(conv.Messages))
	var ratingEntries []struct {
		rating    int
		note      string
		timestamp int64
	}

	for _, m := range conv.Messages {
		content := extractMessageText(m)
		if strings.TrimSpace(content) == "" {
			continue
		}

		role := "agent"
		if m.Type == "user" {
			role = "user"
		}
		msgModel := ""
		if role == "agent" {
			msgModel = firstNonEmpty(strings.TrimSpace(m.Model), strings.TrimSpace(m.ModelName), sessionModel)
		}

		rawJSON, _ := json.Marshal(m)

		ts := parseGeminiTimestamp(m.Timestamp)
		if ts <= 0 {
			continue
		}
		if cutoffMs > 0 && ts < cutoffMs {
			continue
		}
		messages = append(messages, db.Message{
			Timestamp:      ts,
			ProjectID:      projectID,
			ConversationID: conv.SessionID,
			Role:           role,
			Model:          msgModel,
			Content:        content,
			RawJSON:        string(rawJSON),
		})

		if role == "user" {
			if rating, note := parseRatingDisplay(content); rating >= 0 {
				ratingEntries = append(ratingEntries, struct {
					rating    int
					note      string
					timestamp int64
				}{rating, note, ts})
			}
		}
	}

	logsPath := filepath.Join(filepath.Dir(filepath.Dir(path)), "logs.json")
	for _, entry := range readGeminiLogEntries(logsPath, conv.SessionID) {
		content := strings.TrimSpace(entry.Message)
		if content == "" {
			content = "[" + strings.TrimSpace(entry.Type) + "]"
		}
		if content == "[]" {
			content = "[log]"
		}

		role := "agent"
		if entry.Type == "user" {
			role = "user"
		}
		msgModel := ""
		if role == "agent" {
			msgModel = firstNonEmpty(strings.TrimSpace(entry.Model), strings.TrimSpace(entry.ModelName), sessionModel)
		}

		rawJSON, _ := json.Marshal(entry)
		ts := parseGeminiTimestamp(entry.Timestamp)
		if ts <= 0 {
			continue
		}
		if cutoffMs > 0 && ts < cutoffMs {
			continue
		}
		messages = append(messages, db.Message{
			Timestamp:      ts,
			ProjectID:      projectID,
			ConversationID: conv.SessionID,
			Role:           role,
			Model:          msgModel,
			Content:        content,
			RawJSON:        string(rawJSON),
		})

		if role == "user" {
			if rating, note := parseRatingDisplay(content); rating >= 0 {
				ratingEntries = append(ratingEntries, struct {
					rating    int
					note      string
					timestamp int64
				}{rating, note, ts})
			}
		}
	}
	if len(messages) == 0 && len(ratingEntries) == 0 {
		return false
	}

	if err := db.EnsureConversation(ctx, a.db, conv.SessionID, projectID, a.Name()); err != nil {
		log.Printf("gemini watcher: ensure conversation %s: %v", conv.SessionID, err)
		return false
	}
	if err := db.UpdateConversationProject(ctx, a.db, conv.SessionID, projectID); err != nil {
		log.Printf("gemini watcher: update project for %s: %v", conv.SessionID, err)
	}

	if title := readSessionTitle(path); title != "" {
		if err := db.UpdateConversationTitle(ctx, a.db, conv.SessionID, title); err != nil {
			log.Printf("gemini watcher: update title for %s: %v", conv.SessionID, err)
		}
	}

	if len(messages) > 0 {
		messages = appendDiffDBMessages(messages)
		if err := db.InsertMessages(ctx, a.db, messages); err != nil {
			log.Printf("gemini watcher: insert messages for session %s: %v", conv.SessionID, err)
		}
	}

	for _, z := range ratingEntries {
		if err := db.ReconcileOrphanedRating(ctx, a.db, z.rating, z.note, z.timestamp, conv.SessionID); err != nil {
			log.Printf("gemini watcher: reconcile rating for session %s: %v", conv.SessionID, err)
		}
	}
	return len(messages) > 0
}

func readGeminiLogEntries(path, sessionID string) []geminiLogEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var entries []geminiLogEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}

	result := make([]geminiLogEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.SessionID != sessionID {
			continue
		}
		result = append(result, entry)
	}
	return result
}

type pathFilter map[string]struct{}

func newPathFilter(paths []string) pathFilter {
	out := make(pathFilter)
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		if p == "." {
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
	if f == nil {
		return true
	}
	if len(f) == 0 {
		return false
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

func (a *Agent) trackedProjectFilter(ctx context.Context) pathFilter {
	projects, err := db.ListProjects(ctx, a.db, false)
	if err != nil {
		return make(pathFilter)
	}
	out := make(pathFilter)
	for _, p := range projects {
		path := strings.TrimSpace(p.Path)
		if path != "" {
			path = filepath.Clean(path)
		}
		if path != "" && path != "." {
			out[path] = struct{}{}
		}
		for _, oldPath := range strings.Split(p.OldPaths, "\n") {
			oldPath = strings.TrimSpace(oldPath)
			if oldPath == "" {
				continue
			}
			oldPath = filepath.Clean(oldPath)
			if oldPath == "." {
				continue
			}
			out[oldPath] = struct{}{}
		}
	}
	return out
}

// backfillLabels updates project labels from the last path component to the
// git repository root directory name for projects whose label was auto-generated.
func (a *Agent) backfillLabels(ctx context.Context) {
	projects, err := db.ListAllProjects(ctx, a.db)
	if err != nil {
		log.Printf("gemini watcher: list projects for label backfill: %v", err)
		return
	}

	updated := 0
	for _, p := range projects {
		repoName := db.RepoLabel(p.Path)
		if repoName != p.Label && p.Label == filepath.Base(p.Path) {
			if err := db.SetProjectLabel(ctx, a.db, p.ID, repoName); err != nil {
				log.Printf("gemini watcher: update label for %s: %v", p.ID, err)
				continue
			}
			updated++
		}
	}
	if updated > 0 {
		log.Printf("gemini watcher: backfilled %d project labels", updated)
	}
}

func (a *Agent) backfillGitIDs(ctx context.Context) {
	projects, err := db.ListProjectsWithoutGitID(ctx, a.db)
	if err != nil {
		log.Printf("gemini watcher: list projects without git_id: %v", err)
		return
	}

	updated := 0
	for _, p := range projects {
		if gitID := resolveGitID(p.Path); gitID != "" {
			if err := db.UpdateProjectGitID(ctx, a.db, p.ID, gitID); err != nil {
				log.Printf("gemini watcher: update git_id for %s: %v", p.ID, err)
				continue
			}
			updated++
		}
	}
	if updated > 0 {
		log.Printf("gemini watcher: backfilled %d project git_ids", updated)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}
