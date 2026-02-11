package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidcann/zrate/web/server/internal/db"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.InitDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("init test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func writeEntries(t *testing.T, path string, entries []historyEntry) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("open history file: %v", err)
	}
	defer f.Close()
	for _, e := range entries {
		data, _ := json.Marshal(e)
		f.Write(data)
		f.Write([]byte("\n"))
	}
}

func countRows(t *testing.T, database *sql.DB, table string) int {
	t.Helper()
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
	if err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestWatcherProcessEntries(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	entries := []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
		{Display: "hi there", Timestamp: 2000, SessionID: "sess-1", Project: "/proj/a", Type: "assistant"},
		{Display: "what's up", Timestamp: 3000, SessionID: "sess-2", Project: "/proj/b", Type: "user"},
	}
	writeEntries(t, histPath, entries)

	w := newWatcher(database, histPath, tmpDir)
	ctx := context.Background()
	w.scanSince(ctx, time.Time{})

	if n := countRows(t, database, "projects"); n != 2 {
		t.Errorf("projects: got %d, want 2", n)
	}
	if n := countRows(t, database, "conversations"); n != 2 {
		t.Errorf("conversations: got %d, want 2", n)
	}
	if n := countRows(t, database, "turns"); n != 3 {
		t.Errorf("turns: got %d, want 3", n)
	}

	// Verify role mapping.
	var role string
	err := database.QueryRow("SELECT role FROM turns WHERE conversation_id = 'sess-1' ORDER BY timestamp LIMIT 1").Scan(&role)
	if err != nil {
		t.Fatalf("query role: %v", err)
	}
	if role != "user" {
		t.Errorf("role = %q, want %q", role, "user")
	}
	err = database.QueryRow("SELECT role FROM turns WHERE conversation_id = 'sess-1' ORDER BY timestamp DESC LIMIT 1").Scan(&role)
	if err != nil {
		t.Fatalf("query role: %v", err)
	}
	if role != "agent" {
		t.Errorf("role = %q, want %q", role, "agent")
	}
}

func TestWatcherOffsetTracking(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	// Write initial entries.
	writeEntries(t, histPath, []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
	})

	w := newWatcher(database, histPath, tmpDir)
	ctx := context.Background()

	// Initial scan.
	w.scanSince(ctx, time.Time{})
	if n := countRows(t, database, "turns"); n != 1 {
		t.Fatalf("after scan: turns = %d, want 1", n)
	}

	// Append new entries.
	writeEntries(t, histPath, []historyEntry{
		{Display: "response", Timestamp: 2000, SessionID: "sess-1", Project: "/proj/a", Type: "assistant"},
		{Display: "new msg", Timestamp: 3000, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
	})

	// Poll should pick up only the new entries.
	w.poll(ctx)
	if n := countRows(t, database, "turns"); n != 3 {
		t.Errorf("after poll: turns = %d, want 3", n)
	}

	// Polling again with no new data should be a no-op.
	w.poll(ctx)
	if n := countRows(t, database, "turns"); n != 3 {
		t.Errorf("after second poll: turns = %d, want 3", n)
	}
}

func TestWatcherFileRotation(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	// Write initial entries.
	writeEntries(t, histPath, []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
		{Display: "world", Timestamp: 2000, SessionID: "sess-1", Project: "/proj/a", Type: "assistant"},
	})

	w := newWatcher(database, histPath, tmpDir)
	ctx := context.Background()
	w.scanSince(ctx, time.Time{})

	if n := countRows(t, database, "turns"); n != 2 {
		t.Fatalf("after scan: turns = %d, want 2", n)
	}

	// Simulate file rotation: truncate and write new content.
	os.WriteFile(histPath, nil, 0644)
	writeEntries(t, histPath, []historyEntry{
		{Display: "new file", Timestamp: 5000, SessionID: "sess-2", Project: "/proj/b", Type: "user"},
	})

	// Poll should detect shrink and rescan.
	w.poll(ctx)

	if n := countRows(t, database, "conversations"); n != 2 {
		t.Errorf("conversations = %d, want 2", n)
	}
	// Original 2 turns + 1 new turn.
	if n := countRows(t, database, "turns"); n != 3 {
		t.Errorf("turns = %d, want 3", n)
	}
}

func TestWatcherIdempotentReprocessing(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	entries := []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
		{Display: "response", Timestamp: 2000, SessionID: "sess-1", Project: "/proj/a", Type: "assistant"},
	}
	writeEntries(t, histPath, entries)

	w := newWatcher(database, histPath, tmpDir)
	ctx := context.Background()

	// Scan twice — should produce the same result due to INSERT OR IGNORE.
	w.scanSince(ctx, time.Time{})
	w.offset = 0 // reset to force full rescan
	w.scanSince(ctx, time.Time{})

	if n := countRows(t, database, "turns"); n != 2 {
		t.Errorf("turns after double scan = %d, want 2", n)
	}
	if n := countRows(t, database, "projects"); n != 1 {
		t.Errorf("projects after double scan = %d, want 1", n)
	}
	if n := countRows(t, database, "conversations"); n != 1 {
		t.Errorf("conversations after double scan = %d, want 1", n)
	}
}

func TestWatcherSkipsEntriesWithoutSessionID(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	writeEntries(t, histPath, []historyEntry{
		{Display: "no session", Timestamp: 1000, SessionID: "", Project: "/proj/a", Type: "user"},
		{Display: "has session", Timestamp: 2000, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
	})

	w := newWatcher(database, histPath, tmpDir)
	ctx := context.Background()
	w.scanSince(ctx, time.Time{})

	if n := countRows(t, database, "turns"); n != 1 {
		t.Errorf("turns = %d, want 1 (should skip entry without sessionID)", n)
	}
}

func TestWatcherSkipsEntriesWithoutProject(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	writeEntries(t, histPath, []historyEntry{
		{Display: "no project", Timestamp: 1000, SessionID: "sess-1", Project: "", Type: "user"},
	})

	w := newWatcher(database, histPath, tmpDir)
	ctx := context.Background()
	w.scanSince(ctx, time.Time{})

	if n := countRows(t, database, "turns"); n != 0 {
		t.Errorf("turns = %d, want 0 (should skip session without project)", n)
	}
}

func TestWatcherMissingFile(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "nonexistent.jsonl")

	w := newWatcher(database, histPath, tmpDir)
	ctx := context.Background()

	// Should not panic on missing file.
	w.scanSince(ctx, time.Time{})
	w.poll(ctx)

	if n := countRows(t, database, "turns"); n != 0 {
		t.Errorf("turns = %d, want 0", n)
	}
}

func TestWatcherScanSinceFiltersOldEntries(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	now := time.Now()
	oldTs := now.Add(-30 * 24 * time.Hour).UnixMilli() // 30 days ago
	recentTs := now.Add(-3 * 24 * time.Hour).UnixMilli() // 3 days ago

	writeEntries(t, histPath, []historyEntry{
		{Display: "old entry", Timestamp: oldTs, SessionID: "sess-old", Project: "/proj/a", Type: "user"},
		{Display: "recent entry", Timestamp: recentTs, SessionID: "sess-new", Project: "/proj/b", Type: "user"},
	})

	w := newWatcher(database, histPath, tmpDir)
	ctx := context.Background()

	// Scan with 1-week window — should only get the recent entry.
	w.scanSince(ctx, now.Add(-7*24*time.Hour))

	if n := countRows(t, database, "turns"); n != 1 {
		t.Errorf("turns = %d, want 1 (should filter old entries)", n)
	}
	if n := countRows(t, database, "conversations"); n != 1 {
		t.Errorf("conversations = %d, want 1", n)
	}
}

func TestWatcherScanSinceAPI(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	now := time.Now()
	oldTs := now.Add(-60 * 24 * time.Hour).UnixMilli()   // 60 days ago
	midTs := now.Add(-20 * 24 * time.Hour).UnixMilli()    // 20 days ago
	recentTs := now.Add(-1 * 24 * time.Hour).UnixMilli()  // 1 day ago

	writeEntries(t, histPath, []historyEntry{
		{Display: "old", Timestamp: oldTs, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
		{Display: "mid", Timestamp: midTs, SessionID: "sess-2", Project: "/proj/a", Type: "user"},
		{Display: "recent", Timestamp: recentTs, SessionID: "sess-3", Project: "/proj/a", Type: "user"},
	})

	w := newWatcher(database, histPath, tmpDir)
	ctx := context.Background()

	// ScanSince with 30-day window should get mid + recent.
	count := w.ScanSince(ctx, now.Add(-30*24*time.Hour))
	if count != 2 {
		t.Errorf("ScanSince returned %d, want 2", count)
	}
	if n := countRows(t, database, "turns"); n != 2 {
		t.Errorf("turns = %d, want 2", n)
	}

	// ScanSince with 90-day window should also get the old one.
	count = w.ScanSince(ctx, now.Add(-90*24*time.Hour))
	if count != 3 {
		t.Errorf("ScanSince returned %d, want 3", count)
	}
	if n := countRows(t, database, "turns"); n != 3 {
		t.Errorf("turns = %d, want 3", n)
	}
}
