package claude

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func writeHistoryFile(t *testing.T, path string, entries []historyEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create history file: %v", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("encode entry: %v", err)
		}
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

// --- Watcher tests ---

func TestName(t *testing.T) {
	a := newAgent(nil, "", "")
	if a.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", a.Name(), "claude")
	}
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

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

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

	writeEntries(t, histPath, []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()

	a.scanSince(ctx, time.Time{})
	if n := countRows(t, database, "turns"); n != 1 {
		t.Fatalf("after scan: turns = %d, want 1", n)
	}

	writeEntries(t, histPath, []historyEntry{
		{Display: "response", Timestamp: 2000, SessionID: "sess-1", Project: "/proj/a", Type: "assistant"},
		{Display: "new msg", Timestamp: 3000, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
	})

	a.poll(ctx)
	if n := countRows(t, database, "turns"); n != 3 {
		t.Errorf("after poll: turns = %d, want 3", n)
	}

	a.poll(ctx)
	if n := countRows(t, database, "turns"); n != 3 {
		t.Errorf("after second poll: turns = %d, want 3", n)
	}
}

func TestWatcherFileRotation(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	writeEntries(t, histPath, []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
		{Display: "world", Timestamp: 2000, SessionID: "sess-1", Project: "/proj/a", Type: "assistant"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	if n := countRows(t, database, "turns"); n != 2 {
		t.Fatalf("after scan: turns = %d, want 2", n)
	}

	os.WriteFile(histPath, nil, 0644)
	writeEntries(t, histPath, []historyEntry{
		{Display: "new file", Timestamp: 5000, SessionID: "sess-2", Project: "/proj/b", Type: "user"},
	})

	a.poll(ctx)

	if n := countRows(t, database, "conversations"); n != 2 {
		t.Errorf("conversations = %d, want 2", n)
	}
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

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()

	a.scanSince(ctx, time.Time{})
	a.offset = 0
	a.scanSince(ctx, time.Time{})

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

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

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

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	if n := countRows(t, database, "turns"); n != 0 {
		t.Errorf("turns = %d, want 0 (should skip session without project)", n)
	}
}

func TestWatcherMissingFile(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "nonexistent.jsonl")

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()

	a.scanSince(ctx, time.Time{})
	a.poll(ctx)

	if n := countRows(t, database, "turns"); n != 0 {
		t.Errorf("turns = %d, want 0", n)
	}
}

func TestWatcherScanSinceFiltersOldEntries(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	now := time.Now()
	oldTs := now.Add(-30 * 24 * time.Hour).UnixMilli()
	recentTs := now.Add(-3 * 24 * time.Hour).UnixMilli()

	writeEntries(t, histPath, []historyEntry{
		{Display: "old entry", Timestamp: oldTs, SessionID: "sess-old", Project: "/proj/a", Type: "user"},
		{Display: "recent entry", Timestamp: recentTs, SessionID: "sess-new", Project: "/proj/b", Type: "user"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()

	a.scanSince(ctx, now.Add(-7*24*time.Hour))

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
	oldTs := now.Add(-60 * 24 * time.Hour).UnixMilli()
	midTs := now.Add(-20 * 24 * time.Hour).UnixMilli()
	recentTs := now.Add(-1 * 24 * time.Hour).UnixMilli()

	writeEntries(t, histPath, []historyEntry{
		{Display: "old", Timestamp: oldTs, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
		{Display: "mid", Timestamp: midTs, SessionID: "sess-2", Project: "/proj/a", Type: "user"},
		{Display: "recent", Timestamp: recentTs, SessionID: "sess-3", Project: "/proj/a", Type: "user"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()

	count := a.ScanSince(ctx, now.Add(-30*24*time.Hour))
	if count != 2 {
		t.Errorf("ScanSince returned %d, want 2", count)
	}
	if n := countRows(t, database, "turns"); n != 2 {
		t.Errorf("turns = %d, want 2", n)
	}

	count = a.ScanSince(ctx, now.Add(-90*24*time.Hour))
	if count != 3 {
		t.Errorf("ScanSince returned %d, want 3", count)
	}
	if n := countRows(t, database, "turns"); n != 3 {
		t.Errorf("turns = %d, want 3", n)
	}
}

// --- Session/history tests ---

func TestSearchHistoryMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()
	entries := []historyEntry{
		{Display: "some other command", Timestamp: now.Add(-10 * time.Second).UnixMilli(), SessionID: "sess-old", Type: "user"},
		{Display: "/zrate 4 good work", Timestamp: now.Add(-2 * time.Second).UnixMilli(), SessionID: "sess-123", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	sid, ok := searchHistory(path, "/zrate 4 good work", 64*1024, 30*time.Second)
	if !ok {
		t.Fatal("expected match, got none")
	}
	if sid != "sess-123" {
		t.Errorf("sessionID = %q, want %q", sid, "sess-123")
	}
}

func TestSearchHistoryNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()
	entries := []historyEntry{
		{Display: "/zrate 3 different", Timestamp: now.Add(-2 * time.Second).UnixMilli(), SessionID: "sess-1", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	_, ok := searchHistory(path, "/zrate 5 good work", 64*1024, 30*time.Second)
	if ok {
		t.Error("expected no match")
	}
}

func TestSearchHistoryTooOld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	entries := []historyEntry{
		{Display: "/zrate 4 good work", Timestamp: time.Now().Add(-5 * time.Minute).UnixMilli(), SessionID: "sess-old", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	_, ok := searchHistory(path, "/zrate 4 good work", 64*1024, 30*time.Second)
	if ok {
		t.Error("expected no match for entry older than maxAge")
	}
}

func TestSearchHistoryMissingFile(t *testing.T) {
	_, ok := searchHistory("/nonexistent/path/history.jsonl", "/zrate 3", 64*1024, 30*time.Second)
	if ok {
		t.Error("expected no match for missing file")
	}
}

func TestSearchHistoryEmptySessionID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()
	entries := []historyEntry{
		{Display: "/zrate 4 test", Timestamp: now.Add(-2 * time.Second).UnixMilli(), SessionID: "", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	_, ok := searchHistory(path, "/zrate 4 test", 64*1024, 30*time.Second)
	if ok {
		t.Error("expected no match when sessionID is empty")
	}
}

func TestCollectSessionEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	entries := []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-1", Project: "/my/project", Type: "user"},
		{Display: "hi there", Timestamp: 2000, SessionID: "sess-1", Project: "/my/project", Type: "assistant"},
		{Display: "unrelated", Timestamp: 3000, SessionID: "sess-other", Project: "/other", Type: "user"},
		{Display: "thanks", Timestamp: 4000, SessionID: "sess-1", Project: "/my/project", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	result := collectSessionEntries(dir, path, "sess-1")

	if len(result) != 3 {
		t.Fatalf("got %d entries, want 3", len(result))
	}

	if result[0].Timestamp != 1000 {
		t.Errorf("first entry timestamp = %d, want 1000", result[0].Timestamp)
	}
	if result[2].Timestamp != 4000 {
		t.Errorf("last entry timestamp = %d, want 4000", result[2].Timestamp)
	}

	if result[0].Role != "user" {
		t.Errorf("first entry role = %q, want %q", result[0].Role, "user")
	}
	if result[1].Role != "agent" {
		t.Errorf("second entry role = %q, want %q", result[1].Role, "agent")
	}

	if result[0].Project != "/my/project" {
		t.Errorf("project = %q, want %q", result[0].Project, "/my/project")
	}
}

func TestCollectSessionEntriesNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	entries := []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-other", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	result := collectSessionEntries(dir, path, "sess-nonexistent")
	if len(result) != 0 {
		t.Errorf("got %d entries, want 0", len(result))
	}
}

func TestCollectSessionEntriesMissingFile(t *testing.T) {
	result := collectSessionEntries("/tmp", "/nonexistent/history.jsonl", "sess-1")
	if result != nil {
		t.Errorf("expected nil for missing file, got %v", result)
	}
}

func TestResolvePastedContents(t *testing.T) {
	dir := t.TempDir()

	cacheDir := filepath.Join(dir, ".claude", "paste-cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	hash := "abc123"
	if err := os.WriteFile(filepath.Join(cacheDir, hash+".txt"), []byte("pasted content here"), 0644); err != nil {
		t.Fatalf("write paste cache: %v", err)
	}

	display := "Before [Pasted text #1] after"
	pasted := map[string]pastedContent{
		"1": {ID: 1, Type: "text", ContentHash: hash},
	}

	result := resolvePastedContents(dir, display, pasted)
	expected := "Before pasted content here after"
	if result != expected {
		t.Errorf("result = %q, want %q", result, expected)
	}
}

func TestResolvePastedContentsNonTextType(t *testing.T) {
	dir := t.TempDir()

	display := "Before [Pasted text #1] after"
	pasted := map[string]pastedContent{
		"1": {ID: 1, Type: "image", ContentHash: "abc123"},
	}

	result := resolvePastedContents(dir, display, pasted)
	if result != display {
		t.Errorf("result = %q, want %q (unchanged)", result, display)
	}
}

func TestResolvePastedContentsMissingCache(t *testing.T) {
	dir := t.TempDir()

	display := "Before [Pasted text #1] after"
	pasted := map[string]pastedContent{
		"1": {ID: 1, Type: "text", ContentHash: "nonexistent"},
	}

	result := resolvePastedContents(dir, display, pasted)
	if result != display {
		t.Errorf("result = %q, want %q (unchanged)", result, display)
	}
}

func TestSearchHistoryTailBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	enc := json.NewEncoder(f)

	for i := 0; i < 50; i++ {
		enc.Encode(historyEntry{
			Display:   fmt.Sprintf("filler entry %d with extra padding text to take up space", i),
			Timestamp: now.Add(-1 * time.Minute).UnixMilli(),
			SessionID: "sess-filler",
			Type:      "user",
		})
	}

	enc.Encode(historyEntry{
		Display:   "/zrate 5 found it",
		Timestamp: now.Add(-1 * time.Second).UnixMilli(),
		SessionID: "sess-target",
		Type:      "user",
	})
	f.Close()

	sid, ok := searchHistory(path, "/zrate 5 found it", 1024, 30*time.Second)
	if !ok {
		t.Fatal("expected match in tail of file")
	}
	if sid != "sess-target" {
		t.Errorf("sessionID = %q, want %q", sid, "sess-target")
	}
}

// --- Conversation file (first prompt) tests ---

func TestReadFirstPromptStringContent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock conversation JSONL file.
	projectPath := "/proj/a"
	sessionID := "sess-plan"
	dirName := "-proj-a"
	convDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(convDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	convPath := filepath.Join(convDir, sessionID+".jsonl")
	lines := []string{
		`{"type":"file-history-snapshot","messageId":"abc"}`,
		`{"type":"user","timestamp":"2026-02-11T09:50:03.662Z","message":{"content":[{"type":"text","text":"[Request interrupted by user for tool use]"}]}}`,
		`{"type":"user","timestamp":"2026-02-11T09:50:03.659Z","message":{"content":"Implement the following plan:\n\n# Fix the bug"}}`,
		`{"type":"assistant","timestamp":"2026-02-11T09:50:07.250Z","message":{"content":"I will fix the bug."}}`,
	}
	if err := os.WriteFile(convPath, []byte(fmt.Sprintf("%s\n", joinLines(lines))), 0644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	text, ts := readFirstPrompt(tmpDir, projectPath, sessionID)
	if text == "" {
		t.Fatal("expected first prompt, got empty")
	}
	if !containsSubstring(text, "Implement the following plan") {
		t.Errorf("text = %q, want to contain %q", text, "Implement the following plan")
	}
	if ts == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestReadFirstPromptArrayContent(t *testing.T) {
	tmpDir := t.TempDir()

	projectPath := "/proj/b"
	sessionID := "sess-normal"
	dirName := "-proj-b"
	convDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(convDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	convPath := filepath.Join(convDir, sessionID+".jsonl")
	lines := []string{
		`{"type":"user","timestamp":"2026-02-11T10:00:00.000Z","message":{"content":[{"type":"text","text":"<local-command-caveat>skip this</local-command-caveat>"}]}}`,
		`{"type":"user","timestamp":"2026-02-11T10:00:01.000Z","message":{"content":[{"type":"text","text":"Fix the login bug"}]}}`,
	}
	if err := os.WriteFile(convPath, []byte(fmt.Sprintf("%s\n", joinLines(lines))), 0644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	text, ts := readFirstPrompt(tmpDir, projectPath, sessionID)
	if text != "Fix the login bug" {
		t.Errorf("text = %q, want %q", text, "Fix the login bug")
	}
	if ts == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestReadFirstPromptMissingFile(t *testing.T) {
	text, ts := readFirstPrompt(t.TempDir(), "/proj/missing", "sess-none")
	if text != "" || ts != 0 {
		t.Errorf("expected empty result for missing file, got %q %d", text, ts)
	}
}

func TestReadFirstPromptSkipsSystemMessages(t *testing.T) {
	tmpDir := t.TempDir()

	projectPath := "/proj/c"
	sessionID := "sess-sys"
	dirName := "-proj-c"
	convDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(convDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	convPath := filepath.Join(convDir, sessionID+".jsonl")
	lines := []string{
		`{"type":"user","timestamp":"2026-01-01T00:00:00.000Z","message":{"content":[{"type":"text","text":"<command-name>/clear</command-name>"}]}}`,
		`{"type":"user","timestamp":"2026-01-01T00:00:01.000Z","message":{"content":[{"type":"text","text":"<local-command-stdout></local-command-stdout>"}]}}`,
		`{"type":"user","timestamp":"2026-01-01T00:00:02.000Z","message":{"content":[{"type":"text","text":"[Request interrupted by user for tool use]"}]}}`,
		`{"type":"user","timestamp":"2026-01-01T00:00:03.000Z","message":{"content":[{"type":"text","text":"The real first prompt"}]}}`,
	}
	if err := os.WriteFile(convPath, []byte(fmt.Sprintf("%s\n", joinLines(lines))), 0644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	text, _ := readFirstPrompt(tmpDir, projectPath, sessionID)
	if text != "The real first prompt" {
		t.Errorf("text = %q, want %q", text, "The real first prompt")
	}
}

func TestExtractUserTextString(t *testing.T) {
	raw := json.RawMessage(`"hello world"`)
	if text := extractUserText(raw); text != "hello world" {
		t.Errorf("text = %q, want %q", text, "hello world")
	}
}

func TestExtractUserTextArray(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"from array"}]`)
	if text := extractUserText(raw); text != "from array" {
		t.Errorf("text = %q, want %q", text, "from array")
	}
}

func TestExtractUserTextToolResult(t *testing.T) {
	raw := json.RawMessage(`[{"type":"tool_result","tool_use_id":"abc"}]`)
	if text := extractUserText(raw); text != "" {
		t.Errorf("text = %q, want empty", text)
	}
}

func TestExtractUserTextEmpty(t *testing.T) {
	if text := extractUserText(nil); text != "" {
		t.Errorf("text = %q, want empty", text)
	}
}

func TestIsSystemMessage(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"<local-command-caveat>...", true},
		{"<command-name>/clear</command-name>", true},
		{"<system-reminder>...", true},
		{"[Request interrupted by user for tool use]", true},
		{"[]", true},
		{"Fix the bug", false},
		{"Implement the following plan:", false},
		{"<div>html content</div>", false},
	}
	for _, tt := range tests {
		if got := isSystemMessage(tt.text); got != tt.want {
			t.Errorf("isSystemMessage(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestProcessEntriesAddsFirstPromptFromConversationFile(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	// Create a conversation file with a plan-mode first prompt.
	projectPath := "/proj/plan"
	sessionID := "sess-plan-test"
	dirName := "-proj-plan"
	convDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(convDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Use realistic timestamps: plan prompt at T, /zrate 10 minutes later.
	planTime := time.Date(2026, 2, 11, 9, 50, 0, 0, time.UTC)
	rateTime := planTime.Add(10 * time.Minute)

	convPath := filepath.Join(convDir, sessionID+".jsonl")
	convLines := []string{
		fmt.Sprintf(`{"type":"user","timestamp":"%s","message":{"content":"Implement the plan: fix the bug"}}`, planTime.Format(time.RFC3339Nano)),
		fmt.Sprintf(`{"type":"assistant","timestamp":"%s","message":{"content":"OK"}}`, planTime.Add(time.Second).Format(time.RFC3339Nano)),
	}
	if err := os.WriteFile(convPath, []byte(fmt.Sprintf("%s\n", joinLines(convLines))), 0644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	// History only has a later prompt (the initial plan prompt is missing from history).
	writeEntries(t, histPath, []historyEntry{
		{Display: "/zrate 5 great", Timestamp: rateTime.UnixMilli(), SessionID: sessionID, Project: projectPath, Type: "user"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	// Should have 2 turns: the first prompt from the conversation file + the /zrate from history.
	if n := countRows(t, database, "turns"); n != 2 {
		t.Errorf("turns = %d, want 2", n)
	}

	// Verify the first prompt was inserted with correct content.
	var content string
	err := database.QueryRow(
		"SELECT content FROM turns WHERE conversation_id = ? ORDER BY timestamp LIMIT 1",
		sessionID,
	).Scan(&content)
	if err != nil {
		t.Fatalf("query first turn: %v", err)
	}
	if !containsSubstring(content, "Implement the plan") {
		t.Errorf("first turn content = %q, want to contain %q", content, "Implement the plan")
	}
}

func TestProcessEntriesDoesNotDuplicateFirstPrompt(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	// Create a conversation file where the first prompt matches a history entry.
	projectPath := "/proj/dup"
	sessionID := "sess-dup-test"
	dirName := "-proj-dup"
	convDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(convDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	convPath := filepath.Join(convDir, sessionID+".jsonl")
	convLines := []string{
		`{"type":"user","timestamp":"2026-02-11T10:00:00.000Z","message":{"content":[{"type":"text","text":"Fix the login bug"}]}}`,
	}
	if err := os.WriteFile(convPath, []byte(fmt.Sprintf("%s\n", joinLines(convLines))), 0644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	// History has the same first prompt.
	writeEntries(t, histPath, []historyEntry{
		{Display: "Fix the login bug", Timestamp: 1000, SessionID: sessionID, Project: projectPath, Type: "user"},
		{Display: "follow up", Timestamp: 2000, SessionID: sessionID, Project: projectPath, Type: "user"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	// Should have exactly 2 turns (no duplicate).
	if n := countRows(t, database, "turns"); n != 2 {
		t.Errorf("turns = %d, want 2", n)
	}
}

func joinLines(lines []string) string {
	return fmt.Sprintf("%s", strings.Join(lines, "\n"))
}

func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}

// --- readSessionTitle tests ---

func TestReadSessionTitleFound(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := "/proj/a"
	dirName := "-proj-a"
	indexDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	indexContent := `{"version":1,"entries":[{"sessionId":"sess-1","summary":"Fix the login bug"},{"sessionId":"sess-2","summary":"Add dark mode"}]}`
	if err := os.WriteFile(filepath.Join(indexDir, "sessions-index.json"), []byte(indexContent), 0644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	title := readSessionTitle(tmpDir, projectPath, "sess-1")
	if title != "Fix the login bug" {
		t.Errorf("title = %q, want %q", title, "Fix the login bug")
	}

	title = readSessionTitle(tmpDir, projectPath, "sess-2")
	if title != "Add dark mode" {
		t.Errorf("title = %q, want %q", title, "Add dark mode")
	}
}

func TestReadSessionTitleFallbackToFirstPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := "/proj/a"
	dirName := "-proj-a"
	projDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// sessions-index.json exists but does NOT contain this session.
	indexContent := `{"version":1,"entries":[{"sessionId":"sess-other","summary":"Other session"}]}`
	if err := os.WriteFile(filepath.Join(projDir, "sessions-index.json"), []byte(indexContent), 0644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	// Create a conversation .jsonl file for the session.
	convLines := []string{
		`{"type":"user","timestamp":"2026-02-11T10:00:00.000Z","message":{"content":[{"type":"text","text":"Fix the login bug"}]}}`,
	}
	if err := os.WriteFile(filepath.Join(projDir, "sess-fallback.jsonl"), []byte(strings.Join(convLines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	title := readSessionTitle(tmpDir, projectPath, "sess-fallback")
	if title != "Fix the login bug" {
		t.Errorf("title = %q, want %q", title, "Fix the login bug")
	}
}

func TestReadSessionTitleFallbackTruncatesLongPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := "/proj/a"
	dirName := "-proj-a"
	projDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	longText := strings.Repeat("x", 200)
	convLines := []string{
		fmt.Sprintf(`{"type":"user","timestamp":"2026-02-11T10:00:00.000Z","message":{"content":[{"type":"text","text":"%s"}]}}`, longText),
	}
	if err := os.WriteFile(filepath.Join(projDir, "sess-long.jsonl"), []byte(strings.Join(convLines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	title := readSessionTitle(tmpDir, projectPath, "sess-long")
	if len(title) > maxTitleLen+3 {
		t.Errorf("title length = %d, want <= %d", len(title), maxTitleLen+3)
	}
	if !strings.HasSuffix(title, "...") {
		t.Errorf("expected truncated title to end with '...', got %q", title[len(title)-10:])
	}
}

func TestReadSessionTitleFallbackUsesMarkdownHeading(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := "/proj/a"
	dirName := "-proj-a"
	projDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	convLines := []string{
		`{"type":"user","timestamp":"2026-02-11T10:00:00.000Z","message":{"content":"Implement the following plan:\n\n# Add conversation title from Claude's sessions-index.json\n\n## Context\nMore details here"}}`,
	}
	if err := os.WriteFile(filepath.Join(projDir, "sess-heading.jsonl"), []byte(strings.Join(convLines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	title := readSessionTitle(tmpDir, projectPath, "sess-heading")
	if title != "Add conversation title from Claude's sessions-index.json" {
		t.Errorf("title = %q, want %q", title, "Add conversation title from Claude's sessions-index.json")
	}
}

func TestReadSessionTitleFallbackFirstLineWhenNoHeading(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := "/proj/a"
	dirName := "-proj-a"
	projDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	convLines := []string{
		`{"type":"user","timestamp":"2026-02-11T10:00:00.000Z","message":{"content":"First line\nSecond line\nThird line"}}`,
	}
	if err := os.WriteFile(filepath.Join(projDir, "sess-multi.jsonl"), []byte(strings.Join(convLines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	title := readSessionTitle(tmpDir, projectPath, "sess-multi")
	if title != "First line" {
		t.Errorf("title = %q, want %q", title, "First line")
	}
}

func TestTitleFromPrompt(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"heading after preamble", "Implement the following plan:\n\n# Fix the bug\n\nDetails...", "Fix the bug"},
		{"heading on first line", "# Quick fix", "Quick fix"},
		{"no heading", "Just do this thing\nmore details", "Just do this thing"},
		{"ignores h2", "Do this:\n\n## Not a title\nstuff", "Do this:"},
		{"long title truncated", "# " + strings.Repeat("a", 200), strings.Repeat("a", 100) + "..."},
		{"empty heading falls back", "# \nreal content", "#"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := titleFromPrompt(tt.input)
			if got != tt.want {
				t.Errorf("titleFromPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadSessionTitleMissingEverything(t *testing.T) {
	title := readSessionTitle(t.TempDir(), "/proj/missing", "sess-1")
	if title != "" {
		t.Errorf("title = %q, want empty", title)
	}
}

func TestProcessEntriesSetsTitle(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	projectPath := "/proj/titled"
	sessionID := "sess-titled"
	dirName := "-proj-titled"

	// Create sessions-index.json with a summary for this session.
	indexDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	indexContent := fmt.Sprintf(`{"version":1,"entries":[{"sessionId":"%s","summary":"Refactor auth module"}]}`, sessionID)
	if err := os.WriteFile(filepath.Join(indexDir, "sessions-index.json"), []byte(indexContent), 0644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	writeEntries(t, histPath, []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: sessionID, Project: projectPath, Type: "user"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	// Verify the conversation title was set.
	var title string
	err := database.QueryRow("SELECT title FROM conversations WHERE id = ?", sessionID).Scan(&title)
	if err != nil {
		t.Fatalf("query title: %v", err)
	}
	if title != "Refactor auth module" {
		t.Errorf("title = %q, want %q", title, "Refactor auth module")
	}
}

func TestBackfillTitles(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	projectPath := "/proj/backfill"
	dirName := "-proj-backfill"

	// First, create a conversation via processEntries WITHOUT a sessions-index.json.
	writeEntries(t, histPath, []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-bf", Project: projectPath, Type: "user"},
	})
	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	// Verify title is empty.
	var title string
	err := database.QueryRow("SELECT title FROM conversations WHERE id = 'sess-bf'").Scan(&title)
	if err != nil {
		t.Fatalf("query title: %v", err)
	}
	if title != "" {
		t.Fatalf("expected empty title before backfill, got %q", title)
	}

	// Now create the sessions-index.json.
	indexDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	indexContent := `{"version":1,"entries":[{"sessionId":"sess-bf","summary":"Backfilled title"}]}`
	if err := os.WriteFile(filepath.Join(indexDir, "sessions-index.json"), []byte(indexContent), 0644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	// Run backfill.
	a.backfillTitles(ctx)

	// Verify title was set.
	err = database.QueryRow("SELECT title FROM conversations WHERE id = 'sess-bf'").Scan(&title)
	if err != nil {
		t.Fatalf("query title: %v", err)
	}
	if title != "Backfilled title" {
		t.Errorf("title = %q, want %q", title, "Backfilled title")
	}
}

// --- parseZrateDisplay tests ---

func TestParseZrateDisplay(t *testing.T) {
	tests := []struct {
		display    string
		wantRating int
		wantNote   string
	}{
		{"/zrate 4 great work", 4, "great work"},
		{"/zrate 0", 0, ""},
		{"/zrate 5", 5, ""},
		{"/zrate 3 ", 3, ""},
		{"/zrate abc", -1, ""},
		{"/zrate 6", -1, ""},
		{"/zrate -1", -1, ""},
	}
	for _, tt := range tests {
		rating, note := parseZrateDisplay(tt.display)
		if rating != tt.wantRating || note != tt.wantNote {
			t.Errorf("parseZrateDisplay(%q) = (%d, %q), want (%d, %q)",
				tt.display, rating, note, tt.wantRating, tt.wantNote)
		}
	}
}

func TestWatcherReconcileOrphanedRating(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	now := time.Now()

	// Insert an orphaned rating (no matching conversation).
	orphanedConvID := "orphaned-conv-id"
	_, err := db.InsertRating(context.Background(), database, orphanedConvID, 4, "nice", "")
	if err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	// Write history entries including a /zrate entry for the real session.
	realSessionID := "real-sess-id"
	writeEntries(t, histPath, []historyEntry{
		{Display: "hello", Timestamp: now.UnixMilli() - 5000, SessionID: realSessionID, Project: "/proj/a", Type: "user"},
		{Display: "/zrate 4 nice", Timestamp: now.UnixMilli(), SessionID: realSessionID, Project: "/proj/a", Type: "user"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	// Verify the orphaned rating was reconciled to the real session.
	var convID string
	err = database.QueryRow("SELECT conversation_id FROM ratings WHERE conversation_id = ?", realSessionID).Scan(&convID)
	if err != nil {
		t.Fatalf("query reconciled rating: %v", err)
	}
	if convID != realSessionID {
		t.Errorf("conversation_id = %q, want %q", convID, realSessionID)
	}
}

// --- Watcher stores agent name correctly ---

func TestWatcherStoresAgentName(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	writeEntries(t, histPath, []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	var agent string
	err := database.QueryRow("SELECT agent FROM conversations WHERE id = 'sess-1'").Scan(&agent)
	if err != nil {
		t.Fatalf("query agent: %v", err)
	}
	if agent != "claude" {
		t.Errorf("agent = %q, want %q", agent, "claude")
	}
}
