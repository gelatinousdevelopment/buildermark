package claude

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/davidcann/zrate/web/server/internal/agent"
	"github.com/davidcann/zrate/web/server/internal/db"
)

// Run performs an initial scan (last 1 week) then polls for new data until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	log.Printf("claude watcher: starting, monitoring %s", a.path)

	a.scanSince(ctx, time.Now().Add(-agent.DefaultScanWindow))

	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("claude watcher: stopped")
			return
		case <-ticker.C:
			a.poll(ctx)
		}
	}
}

// ScanSince reads the entire file and imports entries with timestamps after
// the given cutoff. This is used by the API to trigger a historical scan.
func (a *Agent) ScanSince(ctx context.Context, since time.Time) int {
	entries, _ := a.readFrom(0)
	cutoffMs := since.UnixMilli()
	var filtered []historyEntry
	for _, e := range entries {
		if e.Timestamp >= cutoffMs {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) > 0 {
		a.processEntries(ctx, filtered)
	}
	log.Printf("claude watcher: manual scan processed %d entries (since %s)", len(filtered), since.Format(time.RFC3339))
	return len(filtered)
}

// scanSince reads the entire file and processes only entries newer than the cutoff.
func (a *Agent) scanSince(ctx context.Context, since time.Time) {
	entries, newOffset := a.readFrom(0)
	cutoffMs := since.UnixMilli()
	var filtered []historyEntry
	for _, e := range entries {
		if e.Timestamp >= cutoffMs {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) > 0 {
		a.processEntries(ctx, filtered)
		log.Printf("claude watcher: initial scan processed %d entries (of %d total)", len(filtered), len(entries))
	}
	a.offset = newOffset
}

// poll reads new data appended since the last read. If the file shrank
// (rotation), it resets and rescans from the beginning.
func (a *Agent) poll(ctx context.Context) {
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
		a.processEntries(ctx, entries)
	}
	a.offset = newOffset
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
		entries = append(entries, entry)
	}

	return entries, offset + int64(n)
}

// processEntries groups entries by sessionId and upserts projects,
// conversations, and turns for each session.
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

	for _, sid := range order {
		g := sessions[sid]
		if g.project == "" {
			continue
		}

		projectID, err := db.EnsureProject(ctx, a.db, g.project)
		if err != nil {
			log.Printf("claude watcher: ensure project %q: %v", g.project, err)
			continue
		}

		if err := db.EnsureConversation(ctx, a.db, sid, projectID, a.Name()); err != nil {
			log.Printf("claude watcher: ensure conversation %s: %v", sid, err)
			continue
		}

		turns := make([]db.Turn, 0, len(g.entries)+1)

		// Check conversation file for a first prompt that history.jsonl may have missed
		// (e.g. plan-mode auto-submissions).
		if firstText, firstTs := readFirstPrompt(a.home, g.project, sid); firstText != "" {
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
				turns = append(turns, db.Turn{
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
			role := "user"
			if e.Type == "assistant" {
				role = "agent"
			}

			display := e.Display
			if len(e.PastedContents) > 0 {
				display = resolvePastedContents(a.home, display, e.PastedContents)
			}

			rawJSON, _ := json.Marshal(e)

			turns = append(turns, db.Turn{
				Timestamp:      e.Timestamp,
				ProjectID:      projectID,
				ConversationID: sid,
				Role:           role,
				Content:        display,
				RawJSON:        string(rawJSON),
			})
		}

		if err := db.InsertTurns(ctx, a.db, turns); err != nil {
			log.Printf("claude watcher: insert turns for session %s: %v", sid, err)
		}
	}
}
