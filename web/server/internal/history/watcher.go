package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/davidcann/zrate/web/server/internal/db"
)

// DefaultScanWindow is how far back the initial scan looks (1 week).
const DefaultScanWindow = 7 * 24 * time.Hour

// Watcher continuously monitors ~/.claude/history.jsonl and imports all
// projects, conversations, and turns into the database.
type Watcher struct {
	db       *sql.DB
	path     string // full path to history.jsonl
	home     string // user home dir (for paste-cache resolution)
	offset   int64
	interval time.Duration
}

// NewWatcher creates a Watcher that will monitor ~/.claude/history.jsonl.
func NewWatcher(database *sql.DB) (*Watcher, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		db:       database,
		path:     filepath.Join(home, ".claude", "history.jsonl"),
		home:     home,
		interval: 2 * time.Second,
	}, nil
}

// newWatcher is an internal constructor for testing with a custom path.
func newWatcher(database *sql.DB, path, home string) *Watcher {
	return &Watcher{
		db:       database,
		path:     path,
		home:     home,
		interval: 2 * time.Second,
	}
}

// Run performs an initial scan (last 1 week) then polls for new data until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	log.Printf("history watcher: starting, monitoring %s", w.path)

	// Initial scan with default window.
	w.scanSince(ctx, time.Now().Add(-DefaultScanWindow))

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("history watcher: stopped")
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

// ScanSince reads the entire file and imports entries with timestamps after
// the given cutoff. This is used by the API to trigger a historical scan.
func (w *Watcher) ScanSince(ctx context.Context, since time.Time) int {
	entries, _ := w.readFrom(0)
	cutoffMs := since.UnixMilli()
	var filtered []historyEntry
	for _, e := range entries {
		if e.Timestamp >= cutoffMs {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) > 0 {
		w.processEntries(ctx, filtered)
	}
	log.Printf("history watcher: manual scan processed %d entries (since %s)", len(filtered), since.Format(time.RFC3339))
	return len(filtered)
}

// scanSince reads the entire file and processes only entries newer than the cutoff.
func (w *Watcher) scanSince(ctx context.Context, since time.Time) {
	entries, newOffset := w.readFrom(0)
	cutoffMs := since.UnixMilli()
	var filtered []historyEntry
	for _, e := range entries {
		if e.Timestamp >= cutoffMs {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) > 0 {
		w.processEntries(ctx, filtered)
		log.Printf("history watcher: initial scan processed %d entries (of %d total)", len(filtered), len(entries))
	}
	w.offset = newOffset
}

// poll reads new data appended since the last read. If the file shrank
// (rotation), it resets and rescans from the beginning.
func (w *Watcher) poll(ctx context.Context) {
	info, err := os.Stat(w.path)
	if err != nil {
		return
	}

	size := info.Size()
	if size < w.offset {
		// File was rotated/truncated — rescan from beginning.
		log.Println("history watcher: file shrunk, rescanning")
		w.offset = 0
	}
	if size == w.offset {
		return // no new data
	}

	entries, newOffset := w.readFrom(w.offset)
	if len(entries) > 0 {
		w.processEntries(ctx, entries)
	}
	w.offset = newOffset
}

// readFrom reads the file starting at the given byte offset and returns parsed
// entries plus the new byte offset (end of file).
func (w *Watcher) readFrom(offset int64) ([]historyEntry, int64) {
	f, err := os.Open(w.path)
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
func (w *Watcher) processEntries(ctx context.Context, entries []historyEntry) {
	// Group by sessionId.
	type sessionGroup struct {
		project string
		entries []historyEntry
	}
	sessions := make(map[string]*sessionGroup)
	order := make([]string, 0) // preserve insertion order

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
			continue // no project path, can't store
		}

		projectID, err := db.EnsureProject(ctx, w.db, g.project)
		if err != nil {
			log.Printf("history watcher: ensure project %q: %v", g.project, err)
			continue
		}

		if err := db.EnsureConversation(ctx, w.db, sid, projectID, "claude"); err != nil {
			log.Printf("history watcher: ensure conversation %s: %v", sid, err)
			continue
		}

		turns := make([]db.Turn, 0, len(g.entries))
		for _, e := range g.entries {
			role := "user"
			if e.Type == "assistant" {
				role = "agent"
			}

			display := e.Display
			if len(e.PastedContents) > 0 {
				display = resolvePastedContents(w.home, display, e.PastedContents)
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

		if err := db.InsertTurns(ctx, w.db, turns); err != nil {
			log.Printf("history watcher: insert turns for session %s: %v", sid, err)
		}
	}
}
