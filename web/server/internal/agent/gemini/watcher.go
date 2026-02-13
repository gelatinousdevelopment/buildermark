package gemini

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/davidcann/zrate/web/server/internal/agent"
	"github.com/davidcann/zrate/web/server/internal/db"
)

type processedFile struct {
	modTime time.Time
}

func (a *Agent) Run(ctx context.Context) {
	log.Printf("gemini watcher: starting, monitoring %s", a.tmpDir)

	a.scanSince(ctx, time.Now().Add(-agent.DefaultScanWindow))
	a.backfillGitIDs(ctx)

	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	seen := make(map[string]processedFile)

	for {
		select {
		case <-ctx.Done():
			log.Println("gemini watcher: stopped")
			return
		case <-ticker.C:
			a.poll(ctx, seen)
			a.backfillGitIDs(ctx)
		}
	}
}

func (a *Agent) ScanSince(ctx context.Context, since time.Time) int {
	n := a.doScan(ctx, since)
	log.Printf("gemini watcher: manual scan processed %d files (since %s)", n, since.Format(time.RFC3339))
	return n
}

func (a *Agent) scanSince(ctx context.Context, since time.Time) {
	n := a.doScan(ctx, since)
	if n > 0 {
		log.Printf("gemini watcher: initial scan processed %d files", n)
	}
}

func (a *Agent) doScan(ctx context.Context, since time.Time) int {
	files := a.listSessionFiles(since)
	for _, path := range files {
		a.processSessionFile(ctx, path)
	}
	return len(files)
}

func (a *Agent) poll(ctx context.Context, seen map[string]processedFile) {
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
	conv, err := readConversation(path)
	if err != nil {
		return
	}
	if conv.SessionID == "" {
		return
	}
	sessionModel := strings.TrimSpace(conv.Model)

	projectPath := a.resolveProjectPath(conv)
	if projectPath == "" {
		projectPath = "unknown"
	}

	projectID, err := db.EnsureProject(ctx, a.db, projectPath)
	if err != nil {
		log.Printf("gemini watcher: ensure project %q: %v", projectPath, err)
		return
	}

	if err := db.EnsureConversation(ctx, a.db, conv.SessionID, projectID, a.Name()); err != nil {
		log.Printf("gemini watcher: ensure conversation %s: %v", conv.SessionID, err)
		return
	}
	if err := db.UpdateConversationProject(ctx, a.db, conv.SessionID, projectID); err != nil {
		log.Printf("gemini watcher: update project for %s: %v", conv.SessionID, err)
	}

	if title := readSessionTitle(path); title != "" {
		if err := db.UpdateConversationTitle(ctx, a.db, conv.SessionID, title); err != nil {
			log.Printf("gemini watcher: update title for %s: %v", conv.SessionID, err)
		}
	}

	messages := make([]db.Message, 0, len(conv.Messages))
	var zrateEntries []struct {
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
			if rating, note := parseZrateDisplay(content); rating >= 0 {
				zrateEntries = append(zrateEntries, struct {
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
			if rating, note := parseZrateDisplay(content); rating >= 0 {
				zrateEntries = append(zrateEntries, struct {
					rating    int
					note      string
					timestamp int64
				}{rating, note, ts})
			}
		}
	}

	if len(messages) > 0 {
		messages = appendDiffDBMessages(messages)
		if err := db.InsertMessages(ctx, a.db, messages); err != nil {
			log.Printf("gemini watcher: insert messages for session %s: %v", conv.SessionID, err)
		}
	}

	for _, z := range zrateEntries {
		if err := db.ReconcileOrphanedRating(ctx, a.db, z.rating, z.note, z.timestamp, conv.SessionID); err != nil {
			log.Printf("gemini watcher: reconcile rating for session %s: %v", conv.SessionID, err)
		}
	}
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
