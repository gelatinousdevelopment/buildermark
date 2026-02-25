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

	"github.com/davidcann/zrate/web/server/internal/agent"
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

func TestReadConversationLogEntriesSkipsInvalidTimestamp(t *testing.T) {
	home := t.TempDir()
	projectPath := "/proj/test"
	sessionID := "sess-1"

	convPath := conversationPath(home, projectPath, sessionID)
	if err := os.MkdirAll(filepath.Dir(convPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	lines := []string{
		`{"type":"assistant","timestamp":"not-a-time","message":{"content":"bad"}}`,
		`{"type":"assistant","timestamp":"2026-02-18T10:00:00.000Z","message":{"content":"ok"}}`,
	}
	if err := os.WriteFile(convPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write conversation file: %v", err)
	}

	entries := readConversationLogEntries(home, projectPath, sessionID)
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if got := entries[0].Content; got != "ok" {
		t.Fatalf("content = %q, want %q", got, "ok")
	}
}

func TestParseProjectConversationLineExtractsToolUsePlanText(t *testing.T) {
	line := `{"type":"assistant","timestamp":"2026-02-22T11:46:04.226Z","sessionId":"sess-1","cwd":"/proj/a","message":{"role":"assistant","content":[{"type":"tool_use","input":{"content":"# Escape HTML in user messages\n\nUser messages in conversations often contain raw HTML"}}]}}`
	entry, ok := parseProjectConversationLine(line)
	if !ok {
		t.Fatal("expected parseProjectConversationLine to succeed")
	}
	if entry.SessionID != "sess-1" {
		t.Fatalf("sessionId = %q, want %q", entry.SessionID, "sess-1")
	}
	if entry.Project != "/proj/a" {
		t.Fatalf("project = %q, want %q", entry.Project, "/proj/a")
	}
	if !strings.Contains(entry.Display, "User messages in conversations often contain raw HTML") {
		t.Fatalf("display missing extracted tool text: %q", entry.Display)
	}
}

func TestScanProjectFilesSinceIngestsOrphanSession(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	projectPath := "/proj/a"
	dirName := strings.ReplaceAll(projectPath, "/", "-")
	projDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	sessionID := "sess-orphan"
	line := `{"type":"user","timestamp":"2026-02-22T11:51:25.292Z","sessionId":"sess-orphan","cwd":"/proj/a","message":{"role":"user","content":"Implement the following plan:\n\n# Escape HTML in user messages\n\nUser messages in conversations often contain raw HTML"}}`
	if err := os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write project conversation: %v", err)
	}

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	n := a.scanProjectFilesSince(ctx, time.Time{}, false, nil, nil)
	if n != 1 {
		t.Fatalf("scanProjectFilesSince = %d, want 1", n)
	}

	var convID string
	if err := database.QueryRow("SELECT id FROM conversations WHERE id = ?", sessionID).Scan(&convID); err != nil {
		t.Fatalf("conversation not ingested: %v", err)
	}

	var content string
	if err := database.QueryRow("SELECT content FROM messages WHERE conversation_id = ? LIMIT 1", sessionID).Scan(&content); err != nil {
		t.Fatalf("query ingested message: %v", err)
	}
	if !strings.Contains(content, "User messages in conversations often contain raw HTML") {
		t.Fatalf("ingested content missing expected plan text: %q", content)
	}
}

func TestScanProjectFilesSinceIngestsMinimalSummaryLine(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	projectPath := "/proj/min-summary"
	dirName := strings.ReplaceAll(projectPath, "/", "-")
	projDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	sessionID := "sess-min-summary"
	lines := []string{
		`{"type":"user","timestamp":"2026-02-22T11:51:25.292Z","sessionId":"sess-min-summary","cwd":"/proj/min-summary","message":{"role":"user","content":"Implement plan"}}`,
		`{"leafUuid":"a3f5416e-4c51-4907-ad27-7c554dad4048","summary":"Add reloadNow selector to TerminalClientApp","type":"summary"}`,
	}
	if err := os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write project conversation: %v", err)
	}

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	n := a.scanProjectFilesSince(ctx, time.Time{}, false, nil, nil)
	if n != 2 {
		t.Fatalf("scanProjectFilesSince = %d, want 2", n)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM messages WHERE conversation_id = ? AND content = ?`, sessionID, "Add reloadNow selector to TerminalClientApp").Scan(&count); err != nil {
		t.Fatalf("query summary message: %v", err)
	}
	if count != 1 {
		t.Fatalf("summary messages = %d, want 1", count)
	}
}

func TestScanProjectFilesDerivesDiffFromToolUseEdit(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	projectPath := "/proj/a"
	dirName := strings.ReplaceAll(projectPath, "/", "-")
	projDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	sessionID := "sess-edit-raw"
	lines := []string{
		`{"type":"user","timestamp":"2026-02-22T11:51:25.292Z","sessionId":"sess-edit-raw","cwd":"/proj/a","message":{"role":"user","content":"apply change"}}`,
		`{"type":"assistant","timestamp":"2026-02-22T11:51:35.482Z","sessionId":"sess-edit-raw","cwd":"/proj/a","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/proj/a/web/frontend/src/lib/messageUtils.ts","old_string":"old line","new_string":"new line"}}]}}`,
	}
	if err := os.WriteFile(filepath.Join(projDir, sessionID+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write project conversation: %v", err)
	}

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	if n := a.scanProjectFilesSince(ctx, time.Time{}, false, nil, nil); n != 2 {
		t.Fatalf("scanProjectFilesSince = %d, want 2", n)
	}

	var derivedCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ? AND raw_json = '{\"source\":\"derived_diff\"}'", sessionID).Scan(&derivedCount); err != nil {
		t.Fatalf("count derived diff messages: %v", err)
	}
	if derivedCount == 0 {
		t.Fatal("expected derived diff message for Edit payload")
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
	if n := countRows(t, database, "messages"); n != 3 {
		t.Errorf("messages: got %d, want 3", n)
	}

	// Verify role mapping.
	var role string
	err := database.QueryRow("SELECT role FROM messages WHERE conversation_id = 'sess-1' ORDER BY timestamp LIMIT 1").Scan(&role)
	if err != nil {
		t.Fatalf("query role: %v", err)
	}
	if role != "user" {
		t.Errorf("role = %q, want %q", role, "user")
	}
	err = database.QueryRow("SELECT role FROM messages WHERE conversation_id = 'sess-1' ORDER BY timestamp DESC LIMIT 1").Scan(&role)
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
	if n := countRows(t, database, "messages"); n != 1 {
		t.Fatalf("after scan: messages = %d, want 1", n)
	}

	writeEntries(t, histPath, []historyEntry{
		{Display: "response", Timestamp: 2000, SessionID: "sess-1", Project: "/proj/a", Type: "assistant"},
		{Display: "new msg", Timestamp: 3000, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
	})

	a.poll(ctx)
	if n := countRows(t, database, "messages"); n != 3 {
		t.Errorf("after poll: messages = %d, want 3", n)
	}

	a.poll(ctx)
	if n := countRows(t, database, "messages"); n != 3 {
		t.Errorf("after second poll: messages = %d, want 3", n)
	}
}

func TestAppendDiffEntries(t *testing.T) {
	entries := []agent.Entry{
		{
			Timestamp: 2000,
			SessionID: "sess-1",
			Project:   "/proj/a",
			Role:      "agent",
			Display:   "```diff\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n```",
		},
	}

	out := appendDiffEntries(entries)
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[1].Timestamp != 2001 {
		t.Fatalf("diff timestamp = %d, want 2001", out[1].Timestamp)
	}
	if !strings.Contains(out[1].Display, "+++ b/a.txt") {
		t.Fatalf("diff entry missing expected content: %q", out[1].Display)
	}
}

func TestAppendDiffEntriesFromRawJSON(t *testing.T) {
	entries := []agent.Entry{
		{
			Timestamp: 2000,
			SessionID: "sess-1",
			Project:   "/proj/a",
			Role:      "agent",
			Display:   "[assistant]",
			RawJSON:   `{"type":"assistant","timestamp":"2026-02-11T10:00:02.000Z","message":{"content":[{"type":"text","text":"diff --git a/x.txt b/x.txt\n--- a/x.txt\n+++ b/x.txt\n@@ -1 +1 @@\n-old\n+new\n"}]}}`,
		},
	}

	out := appendDiffEntries(entries)
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[1].Timestamp != 2001 {
		t.Fatalf("diff timestamp = %d, want 2001", out[1].Timestamp)
	}
	if !strings.Contains(out[1].Display, "--- a/x.txt") {
		t.Fatalf("diff entry missing expected content: %q", out[1].Display)
	}
}

func TestAppendDiffDBMessagesDerivesDiffFromSnapshotBackup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "web", "frontend", "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	sessionID := "sess-snapshot"
	backupDir := filepath.Join(home, ".claude", "file-history", sessionID)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}
	backupName := "abc123@v1"
	if err := os.WriteFile(filepath.Join(backupDir, backupName), []byte("line1\nline2\nold\n"), 0o644); err != nil {
		t.Fatalf("write backup file: %v", err)
	}

	rawSnapshot := `{"type":"file-history-snapshot","snapshot":{"trackedFileBackups":{"web/frontend/src/a.txt":{"backupFileName":"abc123@v1"}}}}`
	rawToolResult := fmt.Sprintf(`{
		"sessionId":"%s",
		"cwd":%q,
		"toolUseResult":{
			"type":"text",
			"file":{
				"filePath":%q,
				"content":"line1\nline2\nnew\n",
				"numLines":3,
				"startLine":1,
				"totalLines":3
			}
		}
	}`, sessionID, filepath.Join(repo, "web", "frontend"), filepath.Join(repo, "web", "frontend", "src", "a.txt"))

	messages := []db.Message{
		{Timestamp: 1000, ConversationID: sessionID, Role: "agent", Content: "[file-history-snapshot]", RawJSON: rawSnapshot},
		{Timestamp: 2000, ConversationID: sessionID, Role: "agent", Content: "[tool_result]", RawJSON: rawToolResult},
	}

	out := appendDiffDBMessages(messages)
	if len(out) != 3 {
		t.Fatalf("len(out) = %d, want 3", len(out))
	}
	if out[2].Timestamp != 2001 {
		t.Fatalf("diff timestamp = %d, want 2001", out[2].Timestamp)
	}
	if !strings.Contains(out[2].Content, "--- a/web/frontend/src/a.txt") ||
		!strings.Contains(out[2].Content, "+++ b/web/frontend/src/a.txt") {
		t.Fatalf("missing expected file path in derived diff: %q", out[2].Content)
	}
	if !strings.Contains(out[2].Content, "\n-old\n") || !strings.Contains(out[2].Content, "\n+new\n") {
		t.Fatalf("derived diff does not include before/after changes: %q", out[2].Content)
	}
}

func TestAppendDiffDBMessagesSnapshotBackupMissingSkipsDiff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	sessionID := "sess-snapshot-missing"
	rawSnapshot := `{"type":"file-history-snapshot","snapshot":{"trackedFileBackups":{"web/frontend/src/a.txt":{"backupFileName":"missing@v1"}}}}`
	rawToolResult := fmt.Sprintf(`{
		"sessionId":"%s",
		"cwd":%q,
		"toolUseResult":{
			"type":"text",
			"file":{
				"filePath":%q,
				"content":"line1\nline2\nnew\n",
				"numLines":3,
				"startLine":1,
				"totalLines":3
			}
		}
	}`, sessionID, filepath.Join(repo, "web", "frontend"), filepath.Join(repo, "web", "frontend", "src", "a.txt"))

	messages := []db.Message{
		{Timestamp: 1000, ConversationID: sessionID, Role: "agent", Content: "[file-history-snapshot]", RawJSON: rawSnapshot},
		{Timestamp: 2000, ConversationID: sessionID, Role: "agent", Content: "[tool_result]", RawJSON: rawToolResult},
	}

	out := appendDiffDBMessages(messages)
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
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

	if n := countRows(t, database, "messages"); n != 2 {
		t.Fatalf("after scan: messages = %d, want 2", n)
	}

	os.WriteFile(histPath, nil, 0644)
	writeEntries(t, histPath, []historyEntry{
		{Display: "new file", Timestamp: 5000, SessionID: "sess-2", Project: "/proj/b", Type: "user"},
	})

	a.poll(ctx)

	if n := countRows(t, database, "conversations"); n != 2 {
		t.Errorf("conversations = %d, want 2", n)
	}
	if n := countRows(t, database, "messages"); n != 3 {
		t.Errorf("messages = %d, want 3", n)
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

	if n := countRows(t, database, "messages"); n != 2 {
		t.Errorf("messages after double scan = %d, want 2", n)
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

	if n := countRows(t, database, "messages"); n != 1 {
		t.Errorf("messages = %d, want 1 (should skip entry without sessionID)", n)
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

	if n := countRows(t, database, "messages"); n != 0 {
		t.Errorf("messages = %d, want 0 (should skip session without project)", n)
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

	if n := countRows(t, database, "messages"); n != 0 {
		t.Errorf("messages = %d, want 0", n)
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

	if n := countRows(t, database, "messages"); n != 1 {
		t.Errorf("messages = %d, want 1 (should filter old entries)", n)
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

	count := a.ScanSince(ctx, now.Add(-30*24*time.Hour), nil)
	if count != 2 {
		t.Errorf("ScanSince returned %d, want 2", count)
	}
	if n := countRows(t, database, "messages"); n != 2 {
		t.Errorf("messages = %d, want 2", n)
	}

	count = a.ScanSince(ctx, now.Add(-90*24*time.Hour), nil)
	if count != 3 {
		t.Errorf("ScanSince returned %d, want 3", count)
	}
	if n := countRows(t, database, "messages"); n != 3 {
		t.Errorf("messages = %d, want 3", n)
	}
}

// --- Session/history tests ---

func TestSearchHistoryMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()
	entries := []historyEntry{
		{Display: "some other command", Timestamp: now.Add(-10 * time.Second).UnixMilli(), SessionID: "sess-old", Type: "user"},
		{Display: "/bb 4 good work", Timestamp: now.Add(-2 * time.Second).UnixMilli(), SessionID: "sess-123", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	sid, ok := searchHistory(path, 64*1024, 30*time.Second)
	if !ok {
		t.Fatal("expected match, got none")
	}
	if sid != "sess-123" {
		t.Errorf("sessionID = %q, want %q", sid, "sess-123")
	}
}

func TestSearchHistoryMatchNoArgs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()
	entries := []historyEntry{
		{Display: "some other command", Timestamp: now.Add(-10 * time.Second).UnixMilli(), SessionID: "sess-old", Type: "user"},
		{Display: "/bb ", Timestamp: now.Add(-2 * time.Second).UnixMilli(), SessionID: "sess-456", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	sid, ok := searchHistory(path, 64*1024, 30*time.Second)
	if !ok {
		t.Fatal("expected match for /bb with no args, got none")
	}
	if sid != "sess-456" {
		t.Errorf("sessionID = %q, want %q", sid, "sess-456")
	}
}

func TestSearchHistoryMatchPluginQualified(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()
	entries := []historyEntry{
		{Display: "/bb:rate", Timestamp: now.Add(-2 * time.Second).UnixMilli(), SessionID: "sess-789", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	sid, ok := searchHistory(path, 64*1024, 30*time.Second)
	if !ok {
		t.Fatal("expected match for /bb:rate, got none")
	}
	if sid != "sess-789" {
		t.Errorf("sessionID = %q, want %q", sid, "sess-789")
	}
}

func TestSearchHistoryNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()
	entries := []historyEntry{
		{Display: "some other command", Timestamp: now.Add(-2 * time.Second).UnixMilli(), SessionID: "sess-1", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	_, ok := searchHistory(path, 64*1024, 30*time.Second)
	if ok {
		t.Error("expected no match")
	}
}

func TestSearchHistoryTooOld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	entries := []historyEntry{
		{Display: "/bb 4 good work", Timestamp: time.Now().Add(-5 * time.Minute).UnixMilli(), SessionID: "sess-old", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	_, ok := searchHistory(path, 64*1024, 30*time.Second)
	if ok {
		t.Error("expected no match for entry older than maxAge")
	}
}

func TestSearchHistoryMissingFile(t *testing.T) {
	_, ok := searchHistory("/nonexistent/path/history.jsonl", 64*1024, 30*time.Second)
	if ok {
		t.Error("expected no match for missing file")
	}
}

func TestSearchHistoryEmptySessionID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()
	entries := []historyEntry{
		{Display: "/bb 4 test", Timestamp: now.Add(-2 * time.Second).UnixMilli(), SessionID: "", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	_, ok := searchHistory(path, 64*1024, 30*time.Second)
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
		Display:   "/bb 5 found it",
		Timestamp: now.Add(-1 * time.Second).UnixMilli(),
		SessionID: "sess-target",
		Type:      "user",
	})
	f.Close()

	sid, ok := searchHistory(path, 1024, 30*time.Second)
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

func TestExtractUserTextToolResultContent(t *testing.T) {
	raw := json.RawMessage(`[{"type":"tool_result","tool_use_id":"abc","content":"patch output"}]`)
	if text := extractUserText(raw); text != "patch output" {
		t.Errorf("text = %q, want %q", text, "patch output")
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

	// Use realistic timestamps: plan prompt at T, /bb 10 minutes later.
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
		{Display: "/bb 5 great", Timestamp: rateTime.UnixMilli(), SessionID: sessionID, Project: projectPath, Type: "user"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	// Should have 3 messages: first prompt + assistant reply from conversation file + /bb from history.
	if n := countRows(t, database, "messages"); n != 3 {
		t.Errorf("messages = %d, want 3", n)
	}

	// Verify the first prompt was inserted with correct content.
	var content string
	err := database.QueryRow(
		"SELECT content FROM messages WHERE conversation_id = ? ORDER BY timestamp LIMIT 1",
		sessionID,
	).Scan(&content)
	if err != nil {
		t.Fatalf("query first message: %v", err)
	}
	if !containsSubstring(content, "Implement the plan") {
		t.Errorf("first message content = %q, want to contain %q", content, "Implement the plan")
	}
}

func TestReadFirstPromptSkipsToolResultUserEntries(t *testing.T) {
	tmpDir := t.TempDir()

	projectPath := "/proj/d"
	sessionID := "sess-tool-result"
	dirName := "-proj-d"
	convDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(convDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	convPath := filepath.Join(convDir, sessionID+".jsonl")
	lines := []string{
		`{"type":"user","sourceToolAssistantUUID":"assist-1","timestamp":"2026-01-01T00:00:00.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"abc","content":"not a user prompt"}]}}`,
		`{"type":"user","timestamp":"2026-01-01T00:00:01.000Z","message":{"content":"real prompt"}}`,
	}
	if err := os.WriteFile(convPath, []byte(fmt.Sprintf("%s\n", joinLines(lines))), 0644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	text, _ := readFirstPrompt(tmpDir, projectPath, sessionID)
	if text != "real prompt" {
		t.Errorf("text = %q, want %q", text, "real prompt")
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

	// Should have exactly 2 messages (no duplicate first prompt from conversation file).
	if n := countRows(t, database, "messages"); n != 2 {
		t.Errorf("messages = %d, want 2", n)
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

	longText := strings.Repeat("x", 1200)
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

func TestReadSessionTitleFallbackUsesFullPrompt(t *testing.T) {
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
	want := "Implement the following plan:\n\n# Add conversation title from Claude's sessions-index.json\n\n## Context\nMore details here"
	if title != want {
		t.Errorf("title = %q, want %q", title, want)
	}
}

func TestReadSessionTitleFallbackPreservesNewLines(t *testing.T) {
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
	if title != "First line\nSecond line\nThird line" {
		t.Errorf("title = %q, want %q", title, "First line\\nSecond line\\nThird line")
	}
}

func TestTitleFromPrompt(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"keeps multiline text", "Implement the following plan:\n\n# Fix the bug\n\nDetails...", "Implement the following plan:\n\n# Fix the bug\n\nDetails..."},
		{"trims outer spaces", "   hello world   ", "hello world"},
		{"long title truncated", strings.Repeat("a", 1001), strings.Repeat("a", 1000) + "..."},
		{"empty after trim", "   ", ""},
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

// --- isZrateDisplay tests ---

func TestIsZrateDisplay(t *testing.T) {
	tests := []struct {
		display string
		want    bool
	}{
		{"/bb", true},
		{"/bb 4", true},
		{"/bb 4 great work", true},
		{"/bb:rate", true},
		{"/bb:rate 3 note", true},
		{"/bbrate", true},
		{"/bbrate 4", true},
		{"/bbrate 3 some note", true},
		{"/bbrate ", true},
		{"hello", false},
		{"/bbb", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isZrateDisplay(tt.display)
		if got != tt.want {
			t.Errorf("isZrateDisplay(%q) = %v, want %v", tt.display, got, tt.want)
		}
	}
}

// --- parseZrateDisplay tests ---

func TestParseZrateDisplay(t *testing.T) {
	tests := []struct {
		display    string
		wantRating int
		wantNote   string
	}{
		{"/bb 4 great work", 4, "great work"},
		{"/bb 0", 0, ""},
		{"/bb 5", 5, ""},
		{"/bb 3 ", 3, ""},
		{"/bb abc", -1, ""},
		{"/bb 6", -1, ""},
		{"/bb -1", -1, ""},
		{"/bbrate 4", 4, ""},
		{"/bbrate 3 some note", 3, "some note"},
		{"/bbrate ", -1, ""},
		{"/bb:rate 4 nice", 4, "nice"},
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

	// Write history entries including a /bb entry for the real session.
	realSessionID := "real-sess-id"
	writeEntries(t, histPath, []historyEntry{
		{Display: "hello", Timestamp: now.UnixMilli() - 5000, SessionID: realSessionID, Project: "/proj/a", Type: "user"},
		{Display: "/bb 4 nice", Timestamp: now.UnixMilli(), SessionID: realSessionID, Project: "/proj/a", Type: "user"},
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
		{Display: "hi", Timestamp: 2000, SessionID: "sess-1", Project: "/proj/a", Type: "assistant", Model: "claude-3-7-sonnet"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	var agentName string
	err := database.QueryRow("SELECT agent FROM conversations WHERE id = 'sess-1'").Scan(&agentName)
	if err != nil {
		t.Fatalf("query agent: %v", err)
	}
	if agentName != "claude" {
		t.Errorf("agent = %q, want %q", agentName, "claude")
	}
	var model string
	err = database.QueryRow("SELECT model FROM messages WHERE conversation_id = 'sess-1' AND role = 'agent' ORDER BY timestamp DESC LIMIT 1").Scan(&model)
	if err != nil {
		t.Fatalf("query model: %v", err)
	}
	if model != "claude-3-7-sonnet" {
		t.Errorf("model = %q, want %q", model, "claude-3-7-sonnet")
	}
}

func TestConversationLogDedupDoesNotDropDuplicateContent(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	repo := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	projectPath := "/proj/edit"
	sessionID := "sess-multi-edit"

	// Create a history entry to establish the session.
	writeEntries(t, histPath, []historyEntry{
		{Display: "edit both functions", Timestamp: 1000, SessionID: sessionID, Project: projectPath, Type: "user"},
	})

	// Create a conversation JSONL with two Edit tool_result entries for the same file.
	// Both have identical content text but different structuredPatch data.
	dirName := "-proj-edit"
	convDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatalf("mkdir conv dir: %v", err)
	}

	filePath := filepath.Join(repo, "main.go")
	toolResult1 := fmt.Sprintf(`{"type":"user","sourceToolAssistantUUID":"assist-1","timestamp":"2026-02-18T10:00:01.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"edit1","content":"The file %s has been updated successfully."}]},"toolUseResult":{"filePath":%q,"structuredPatch":[{"oldStart":1,"oldLines":1,"newStart":1,"newLines":1,"lines":["-old1","+new1"]}]}}`, filePath, filePath)
	toolResult2 := fmt.Sprintf(`{"type":"user","sourceToolAssistantUUID":"assist-2","timestamp":"2026-02-18T10:00:02.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"edit2","content":"The file %s has been updated successfully."}]},"toolUseResult":{"filePath":%q,"structuredPatch":[{"oldStart":5,"oldLines":1,"newStart":5,"newLines":1,"lines":["-old2","+new2"]}]}}`, filePath, filePath)

	convPath := filepath.Join(convDir, sessionID+".jsonl")
	if err := os.WriteFile(convPath, []byte(toolResult1+"\n"+toolResult2+"\n"), 0o644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	// Count messages. We expect:
	// 1 history entry ("edit both functions") +
	// 1 tool_result (the second identical one is deduped by DB-layer dedup, which is fine) +
	// 2 derived diffs (one per tool_result, with different content so not deduped) = 4 total
	if n := countRows(t, database, "messages"); n != 4 {
		t.Errorf("messages = %d, want 4 (1 history + 1 tool_result + 2 derived diffs)", n)
	}

	// Verify both derived diffs are present by checking for the specific diff content.
	var diff1Found, diff2Found bool
	rows, err := database.Query("SELECT content FROM messages WHERE conversation_id = ?", sessionID)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var content string
		rows.Scan(&content)
		if strings.Contains(content, "-old1") && strings.Contains(content, "+new1") {
			diff1Found = true
		}
		if strings.Contains(content, "-old2") && strings.Contains(content, "+new2") {
			diff2Found = true
		}
	}
	if !diff1Found {
		t.Error("derived diff for first edit (old1->new1) not found")
	}
	if !diff2Found {
		t.Error("derived diff for second edit (old2->new2) not found")
	}
}

func TestConversationLogSidechainUserEntryStoredAsAgent(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	projectPath := "/proj/sidechain"
	sessionID := "sess-sidechain-role"

	writeEntries(t, histPath, []historyEntry{
		{Display: "normal user prompt", Timestamp: 1000, SessionID: sessionID, Project: projectPath, Type: "user"},
	})

	dirName := "-proj-sidechain"
	convDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatalf("mkdir conv dir: %v", err)
	}

	convPath := filepath.Join(convDir, sessionID+".jsonl")
	sidechainLine := fmt.Sprintf(`{"type":"user","isSidechain":true,"userType":"external","agentId":"agent-123","timestamp":"2026-02-23T06:34:49.335Z","sessionId":%q,"cwd":%q,"message":{"role":"user","content":"delegate this subtask"}}`, sessionID, projectPath)
	if err := os.WriteFile(convPath, []byte(sidechainLine+"\n"), 0o644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	var role string
	if err := database.QueryRow("SELECT role FROM messages WHERE conversation_id = ? AND content = ? LIMIT 1", sessionID, "delegate this subtask").Scan(&role); err != nil {
		t.Fatalf("query sidechain role: %v", err)
	}
	if role != "agent" {
		t.Errorf("role = %q, want %q", role, "agent")
	}
}

// --- Summary entry tests ---

func TestReadSummaryFromConversationFile(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := "/proj/summary"
	sessionID := "sess-summary"

	convPath := conversationPath(tmpDir, projectPath, sessionID)
	if err := os.MkdirAll(filepath.Dir(convPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	lines := []string{
		`{"type":"user","timestamp":"2026-02-23T10:00:00.000Z","sessionId":"sess-summary","cwd":"/proj/summary","message":{"content":"Fix the bug"}}`,
		`{"type":"summary","timestamp":"2026-02-23T10:01:00.000Z","sessionId":"sess-summary","cwd":"/proj/summary","summary":"Add reloadNow selector to TerminalClientApp"}`,
	}
	if err := os.WriteFile(convPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	got := readSummaryFromConversationFile(tmpDir, projectPath, sessionID)
	if got != "Add reloadNow selector to TerminalClientApp" {
		t.Errorf("readSummaryFromConversationFile() = %q, want %q", got, "Add reloadNow selector to TerminalClientApp")
	}
}

func TestReadSummaryFromConversationFileUsesLast(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := "/proj/summary2"
	sessionID := "sess-summary2"

	convPath := conversationPath(tmpDir, projectPath, sessionID)
	if err := os.MkdirAll(filepath.Dir(convPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	lines := []string{
		`{"type":"summary","timestamp":"2026-02-23T10:00:00.000Z","sessionId":"sess-summary2","cwd":"/proj/summary2","summary":"First summary"}`,
		`{"type":"summary","timestamp":"2026-02-23T10:05:00.000Z","sessionId":"sess-summary2","cwd":"/proj/summary2","summary":"Updated summary"}`,
	}
	if err := os.WriteFile(convPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	got := readSummaryFromConversationFile(tmpDir, projectPath, sessionID)
	if got != "Updated summary" {
		t.Errorf("readSummaryFromConversationFile() = %q, want %q", got, "Updated summary")
	}
}

func TestReadSummaryFromConversationFileMissing(t *testing.T) {
	got := readSummaryFromConversationFile(t.TempDir(), "/proj/missing", "sess-none")
	if got != "" {
		t.Errorf("readSummaryFromConversationFile() = %q, want empty", got)
	}
}

func TestSummaryEntryTitlePriorityOverSessionsIndex(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	projectPath := "/proj/summary-prio"
	sessionID := "sess-summary-prio"
	dirName := strings.ReplaceAll(projectPath, "/", "-")

	// Create sessions-index.json with a different summary.
	indexDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	indexContent := fmt.Sprintf(`{"version":1,"entries":[{"sessionId":"%s","summary":"Sessions index title"}]}`, sessionID)
	if err := os.WriteFile(filepath.Join(indexDir, "sessions-index.json"), []byte(indexContent), 0644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	// Create a conversation file with a summary entry.
	convPath := filepath.Join(indexDir, sessionID+".jsonl")
	convLines := []string{
		`{"type":"user","timestamp":"2026-02-23T10:00:00.000Z","sessionId":"sess-summary-prio","cwd":"/proj/summary-prio","message":{"content":"Fix the bug"}}`,
		`{"type":"summary","timestamp":"2026-02-23T10:01:00.000Z","sessionId":"sess-summary-prio","cwd":"/proj/summary-prio","summary":"Inline summary title"}`,
	}
	if err := os.WriteFile(convPath, []byte(strings.Join(convLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	// Write a history entry to trigger processEntries.
	writeEntries(t, histPath, []historyEntry{
		{Display: "Fix the bug", Timestamp: 1000, SessionID: sessionID, Project: projectPath, Type: "user"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	var title string
	if err := database.QueryRow("SELECT title FROM conversations WHERE id = ?", sessionID).Scan(&title); err != nil {
		t.Fatalf("query title: %v", err)
	}
	if title != "Inline summary title" {
		t.Errorf("title = %q, want %q", title, "Inline summary title")
	}
}

func TestProcessEntriesIncludesSummaryInMessages(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	projectPath := "/proj/summary-msg"
	sessionID := "sess-summary-msg"
	dirName := strings.ReplaceAll(projectPath, "/", "-")

	// Create conversation file with a summary entry.
	convDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
	if err := os.MkdirAll(convDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	convPath := filepath.Join(convDir, sessionID+".jsonl")
	convLines := []string{
		`{"type":"user","timestamp":"2026-02-23T10:00:00.000Z","sessionId":"sess-summary-msg","cwd":"/proj/summary-msg","message":{"content":"Fix the bug"}}`,
		`{"type":"summary","timestamp":"2026-02-23T10:01:00.000Z","sessionId":"sess-summary-msg","cwd":"/proj/summary-msg","summary":"Summary title"}`,
		`{"type":"assistant","timestamp":"2026-02-23T10:02:00.000Z","sessionId":"sess-summary-msg","cwd":"/proj/summary-msg","message":{"content":"Done"}}`,
	}
	if err := os.WriteFile(convPath, []byte(strings.Join(convLines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	// Write history entries including a summary type.
	writeEntries(t, histPath, []historyEntry{
		{Display: "Fix the bug", Timestamp: 1000, SessionID: sessionID, Project: projectPath, Type: "user"},
		{Display: "[summary]", Timestamp: 2000, SessionID: sessionID, Project: projectPath, Type: "summary", Summary: "Summary title"},
		{Display: "Done", Timestamp: 3000, SessionID: sessionID, Project: projectPath, Type: "assistant"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	// Summary entries should appear as messages using their summary text.
	rows, err := database.Query("SELECT content FROM messages WHERE conversation_id = ?", sessionID)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	defer rows.Close()

	var contents []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		contents = append(contents, c)
	}

	// Should have user + summary + assistant messages.
	hasFix := false
	hasSummary := false
	hasDone := false
	for _, c := range contents {
		if strings.Contains(c, "Fix the bug") {
			hasFix = true
		}
		if c == "Summary title" {
			hasSummary = true
		}
		if c == "Done" {
			hasDone = true
		}
	}
	if !hasFix {
		t.Error("expected 'Fix the bug' message")
	}
	if !hasSummary {
		t.Error("expected summary message")
	}
	if !hasDone {
		t.Error("expected 'Done' message")
	}
}

func TestReadConversationLogEntriesIncludesSummary(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := "/proj/logskip"
	sessionID := "sess-logskip"

	convPath := conversationPath(tmpDir, projectPath, sessionID)
	if err := os.MkdirAll(filepath.Dir(convPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	lines := []string{
		`{"type":"user","timestamp":"2026-02-23T10:00:00.000Z","message":{"content":"hello"}}`,
		`{"type":"summary","timestamp":"2026-02-23T10:01:00.000Z","summary":"A summary"}`,
		`{"type":"assistant","timestamp":"2026-02-23T10:02:00.000Z","message":{"content":"world"}}`,
	}
	if err := os.WriteFile(convPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write conv file: %v", err)
	}

	entries := readConversationLogEntries(tmpDir, projectPath, sessionID)
	foundSummary := false
	for _, e := range entries {
		if e.Type == "summary" && e.Content == "A summary" {
			foundSummary = true
		}
	}
	if !foundSummary {
		t.Error("expected summary entry from readConversationLogEntries")
	}
	if len(entries) != 3 {
		t.Errorf("entries len = %d, want 3", len(entries))
	}
}

func TestCollectSessionEntriesIncludesSummary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	entries := []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-1", Project: "/proj/a", Type: "user"},
		{Display: "[summary]", Timestamp: 2000, SessionID: "sess-1", Project: "/proj/a", Type: "summary", Summary: "Title"},
		{Display: "response", Timestamp: 3000, SessionID: "sess-1", Project: "/proj/a", Type: "assistant"},
	}
	writeHistoryFile(t, path, entries)

	result := collectSessionEntries(dir, path, "sess-1")
	if len(result) != 3 {
		t.Fatalf("got %d entries, want 3", len(result))
	}
	foundSummary := false
	for _, e := range result {
		if e.Display == "Title" && e.Role == "agent" {
			foundSummary = true
		}
	}
	if !foundSummary {
		t.Error("summary entry missing from collected session entries")
	}
}

func TestProcessEntriesInlineSummaryFromHistoryEntries(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	projectPath := "/proj/inline-sum"
	sessionID := "sess-inline-sum"

	// No sessions-index.json, no conversation file — only history entries.
	writeEntries(t, histPath, []historyEntry{
		{Display: "do the thing", Timestamp: 1000, SessionID: sessionID, Project: projectPath, Type: "user"},
		{Display: "[summary]", Timestamp: 5000, SessionID: sessionID, Project: projectPath, Type: "summary", Summary: "Inline title from history"},
	})

	a := newAgent(database, histPath, tmpDir)
	ctx := context.Background()
	a.scanSince(ctx, time.Time{})

	var title string
	if err := database.QueryRow("SELECT title FROM conversations WHERE id = ?", sessionID).Scan(&title); err != nil {
		t.Fatalf("query title: %v", err)
	}
	if title != "Inline title from history" {
		t.Errorf("title = %q, want %q", title, "Inline title from history")
	}
}

func TestWatcherExtractsModelFromNestedRawHistoryJSON(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	histPath := filepath.Join(tmpDir, "history.jsonl")

	raw := `{"display":"hello","timestamp":1000,"sessionId":"sess-nested","project":"/proj/a","type":"assistant","message":{"metadata":{"model":"claude-3-5-sonnet"}}}` + "\n"
	if err := os.WriteFile(histPath, []byte(raw), 0644); err != nil {
		t.Fatalf("write history: %v", err)
	}

	a := newAgent(database, histPath, tmpDir)
	a.scanSince(context.Background(), time.Time{})

	var model string
	if err := database.QueryRow("SELECT model FROM messages WHERE conversation_id = 'sess-nested' AND role = 'agent' LIMIT 1").Scan(&model); err != nil {
		t.Fatalf("query model: %v", err)
	}
	if model != "claude-3-5-sonnet" {
		t.Errorf("model = %q, want %q", model, "claude-3-5-sonnet")
	}
}

func TestExtractParentSessionID(t *testing.T) {
	tests := []struct {
		name    string
		entries []conversationLogEntry
		want    string
	}{
		{
			name:    "no entries",
			entries: nil,
			want:    "",
		},
		{
			name: "no user messages",
			entries: []conversationLogEntry{
				{Role: "agent", Content: "Hello"},
			},
			want: "",
		},
		{
			name: "user message without jsonl reference",
			entries: []conversationLogEntry{
				{Role: "user", Content: "Please implement the feature"},
			},
			want: "",
		},
		{
			name: "user message with jsonl reference",
			entries: []conversationLogEntry{
				{Role: "user", Content: "Implement the plan.\n\nread the full transcript at: /Users/david/.claude/projects/-Users-david-github-zrate/8baafe77-1234-5678-9abc-def012345678.jsonl"},
			},
			want: "8baafe77-1234-5678-9abc-def012345678",
		},
		{
			name: "skips system messages to find real first user message",
			entries: []conversationLogEntry{
				{Role: "user", Content: "<system-reminder>some system stuff</system-reminder>"},
				{Role: "user", Content: "read the full transcript at: /Users/david/.claude/projects/foo/abcdef01-2345-6789-abcd-ef0123456789.jsonl"},
			},
			want: "abcdef01-2345-6789-abcd-ef0123456789",
		},
		{
			name: "only checks first substantive user message",
			entries: []conversationLogEntry{
				{Role: "user", Content: "Please do the thing"},
				{Role: "user", Content: "read the full transcript at: /Users/david/.claude/projects/foo/8baafe77-1234-5678-9abc-def012345678.jsonl"},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractParentSessionID(tt.entries)
			if got != tt.want {
				t.Errorf("extractParentSessionID() = %q, want %q", got, tt.want)
			}
		})
	}
}
