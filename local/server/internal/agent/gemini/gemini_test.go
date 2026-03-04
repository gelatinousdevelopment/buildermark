package gemini

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
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

func countRows(t *testing.T, database *sql.DB, table string) int {
	t.Helper()
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
	if err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func writeConversationFile(t *testing.T, path string, conv map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(conv, "", "  ")
	if err != nil {
		t.Fatalf("marshal conversation: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write conversation: %v", err)
	}
}

func writeLogsFile(t *testing.T, path string, entries []map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		t.Fatalf("marshal logs: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write logs: %v", err)
	}
}

func TestName(t *testing.T) {
	a := newAgent(nil, "", "")
	if a.Name() != "gemini" {
		t.Errorf("Name() = %q, want %q", a.Name(), "gemini")
	}
}

func TestParseGeminiTimestampInvalidReturnsZero(t *testing.T) {
	if got := parseGeminiTimestamp(""); got != 0 {
		t.Fatalf("parseGeminiTimestamp(empty) = %d, want 0", got)
	}
	if got := parseGeminiTimestamp("not-a-time"); got != 0 {
		t.Fatalf("parseGeminiTimestamp(invalid) = %d, want 0", got)
	}
}

func TestWatcherProcessSessionFile(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	hash := "abc123hash"
	sessionID := "11111111-2222-3333-4444-555555555555"
	now := time.Now().UTC()

	rating, err := db.InsertRating(context.Background(), database, "orphan-cid", 4, "great work", "")
	if err != nil {
		t.Fatalf("insert rating: %v", err)
	}

	convPath := filepath.Join(tmpDir, hash, "chats", "session-2026-02-12T12-00-11111111.json")
	writeConversationFile(t, convPath, map[string]any{
		"sessionId":   sessionID,
		"projectHash": hash,
		"model":       "gemini-2.5-pro",
		"directories": []string{"/proj/gemini"},
		"messages": []map[string]any{
			{
				"id":        "m1",
				"timestamp": now.Add(-2 * time.Second).Format(time.RFC3339Nano),
				"type":      "user",
				"content":   "hello",
			},
			{
				"id":        "m2",
				"timestamp": now.Add(-1 * time.Second).Format(time.RFC3339Nano),
				"type":      "gemini",
				"content":   "hi",
			},
			{
				"id":        "m3",
				"timestamp": now.Format(time.RFC3339Nano),
				"type":      "user",
				"content":   "/bb 4 great work",
			},
		},
	})

	a := newAgent(database, tmpDir, tmpDir)
	a.processSessionFile(context.Background(), convPath)

	if n := countRows(t, database, "projects"); n != 1 {
		t.Errorf("projects: got %d, want 1", n)
	}
	if n := countRows(t, database, "conversations"); n != 1 {
		t.Errorf("conversations: got %d, want 1", n)
	}
	if n := countRows(t, database, "messages"); n != 3 {
		t.Errorf("messages: got %d, want 3", n)
	}

	var conversationID string
	err = database.QueryRow("SELECT conversation_id FROM ratings WHERE id = ?", rating.ID).Scan(&conversationID)
	if err != nil {
		t.Fatalf("query rating: %v", err)
	}
	if conversationID != sessionID {
		t.Errorf("conversation_id = %q, want %q", conversationID, sessionID)
	}

	var model string
	if err := database.QueryRow("SELECT model FROM messages WHERE conversation_id = ? AND role = 'agent' ORDER BY timestamp DESC LIMIT 1", sessionID).Scan(&model); err != nil {
		t.Fatalf("query message model: %v", err)
	}
	if model != "gemini-2.5-pro" {
		t.Errorf("model = %q, want %q", model, "gemini-2.5-pro")
	}
}

func TestWatcherScanSinceUsesMessageTimestampsEvenWhenFileMtimeIsOld(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	hash := "stale-mtime-hash"
	sessionID := "scan-since-stale-mtime"
	now := time.Now().UTC()

	convPath := filepath.Join(tmpDir, hash, "chats", "session-2026-02-12T12-00-stale.json")
	writeConversationFile(t, convPath, map[string]any{
		"sessionId":   sessionID,
		"projectHash": hash,
		"directories": []string{"/proj/gemini"},
		"messages": []map[string]any{
			{
				"id":        "m1",
				"timestamp": now.Add(-2 * time.Hour).Format(time.RFC3339Nano),
				"type":      "user",
				"content":   "recent within window",
			},
		},
	})

	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(convPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	a := newAgent(database, tmpDir, tmpDir)
	ctx := context.Background()

	// Pre-create the project so it is tracked.
	if _, err := db.EnsureProject(ctx, database, "/proj/gemini"); err != nil {
		t.Fatalf("ensure project: %v", err)
	}

	n := a.ScanSince(ctx, time.Now().Add(-24*time.Hour), nil)
	if n != 1 {
		t.Fatalf("ScanSince processed %d files, want 1", n)
	}

	if got := countRows(t, database, "messages"); got != 1 {
		t.Fatalf("messages: got %d, want 1", got)
	}
}

func TestWatcherDetectsNestedConversationModel(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	hash := "nested-hash"
	sessionID := "99999999-2222-3333-4444-555555555555"
	now := time.Now().UTC()

	convPath := filepath.Join(tmpDir, hash, "chats", "session-2026-02-12T12-00-99999999.json")
	writeConversationFile(t, convPath, map[string]any{
		"sessionId":   sessionID,
		"projectHash": hash,
		"config": map[string]any{
			"model": "gemini-2.0-flash",
		},
		"directories": []string{"/proj/gemini"},
		"messages": []map[string]any{
			{
				"id":        "m1",
				"timestamp": now.Add(-1 * time.Second).Format(time.RFC3339Nano),
				"type":      "gemini",
				"content":   "hello",
			},
		},
	})

	a := newAgent(database, tmpDir, tmpDir)
	a.processSessionFile(context.Background(), convPath)

	var model string
	if err := database.QueryRow("SELECT model FROM messages WHERE conversation_id = ? AND role = 'agent' LIMIT 1", sessionID).Scan(&model); err != nil {
		t.Fatalf("query model: %v", err)
	}
	if model != "gemini-2.0-flash" {
		t.Errorf("model = %q, want %q", model, "gemini-2.0-flash")
	}
}

func TestAppendDiffEntries(t *testing.T) {
	entries := []agent.Entry{
		{
			Timestamp: 3000,
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
	if out[1].Timestamp != 3001 {
		t.Fatalf("diff timestamp = %d, want 3001", out[1].Timestamp)
	}
	if !strings.Contains(out[1].Display, "@@ -1 +1 @@") {
		t.Fatalf("diff entry missing expected content: %q", out[1].Display)
	}
}

func TestAppendDiffEntriesFromRawJSON(t *testing.T) {
	entries := []agent.Entry{
		{
			Timestamp: 3000,
			SessionID: "sess-1",
			Project:   "/proj/a",
			Role:      "agent",
			Display:   "[tool call]",
			RawJSON:   `{"id":"m2","type":"gemini","toolCalls":[{"id":"run-shell","name":"run_shell_command","resultDisplay":"diff --git a/x.txt b/x.txt\n--- a/x.txt\n+++ b/x.txt\n@@ -1 +1 @@\n-old\n+new\n"}]}`,
		},
	}

	out := appendDiffEntries(entries)
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[1].Timestamp != 3001 {
		t.Fatalf("diff timestamp = %d, want 3001", out[1].Timestamp)
	}
	if !strings.Contains(out[1].Display, "+++ b/x.txt") {
		t.Fatalf("diff entry missing expected content: %q", out[1].Display)
	}
}

func TestSessionResolverFromLogs(t *testing.T) {
	tmpDir := t.TempDir()
	hash := "hash-resolve"
	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	now := time.Now().UTC()

	convPath := filepath.Join(tmpDir, hash, "chats", "session-2026-02-12T12-00-aaaaaaaa.json")
	writeConversationFile(t, convPath, map[string]any{
		"sessionId":   sessionID,
		"projectHash": hash,
		"directories": []string{"/proj/resolve"},
		"messages": []map[string]any{
			{
				"id":        "u1",
				"timestamp": now.Add(-1 * time.Second).Format(time.RFC3339Nano),
				"type":      "user",
				"content":   "do thing",
			},
			{
				"id":        "a1",
				"timestamp": now.Format(time.RFC3339Nano),
				"type":      "gemini",
				"content":   "done",
			},
		},
	})

	logsPath := filepath.Join(tmpDir, hash, "logs.json")
	writeLogsFile(t, logsPath, []map[string]any{
		{
			"sessionId": sessionID,
			"type":      "user",
			"message":   "/bb 5",
			"timestamp": now.Format(time.RFC3339Nano),
		},
	})

	a := newAgent(nil, tmpDir, tmpDir)
	res := a.ResolveSession(5, "", "fallback-id")

	if res.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", res.SessionID, sessionID)
	}
	if res.Project != "/proj/resolve" {
		t.Errorf("Project = %q, want %q", res.Project, "/proj/resolve")
	}
	if len(res.Entries) != 2 {
		t.Errorf("entries: got %d, want 2", len(res.Entries))
	}
}

func TestSessionResolverFallback(t *testing.T) {
	a := newAgent(nil, t.TempDir(), t.TempDir())
	res := a.ResolveSession(3, "note", "fallback-id")
	if res.SessionID != "fallback-id" {
		t.Errorf("SessionID = %q, want %q", res.SessionID, "fallback-id")
	}
	if len(res.Entries) != 0 {
		t.Errorf("entries: got %d, want 0", len(res.Entries))
	}
}

func TestParseRatingDisplay(t *testing.T) {
	tests := []struct {
		input      string
		wantRating int
		wantNote   string
	}{
		{"/bb 4 great work", 4, "great work"},
		{"/bb 5", 5, ""},
		{"/bb abc", -1, ""},
		{"[/bb](/tmp/commands/bb.toml) 2 meh", 2, "meh"},
	}

	for _, tt := range tests {
		rating, note := parseRatingDisplay(tt.input)
		if rating != tt.wantRating || note != tt.wantNote {
			t.Errorf("parseRatingDisplay(%q) = (%d, %q), want (%d, %q)", tt.input, rating, note, tt.wantRating, tt.wantNote)
		}
	}
}

func TestReadSessionTitle(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.json")
	writeConversationFile(t, path, map[string]any{
		"sessionId": "sid",
		"messages": []map[string]any{
			{"id": "1", "timestamp": "2026-02-12T00:00:00Z", "type": "user", "content": "# Fix Build\n\nPlease help"},
		},
	})

	title := readSessionTitle(path)
	if title != "# Fix Build\n\nPlease help" {
		t.Errorf("title = %q, want %q", title, "# Fix Build\n\nPlease help")
	}
}

func TestInferProjectPath(t *testing.T) {
	conv := &geminiConversation{
		ProjectHash: "abc",
		Messages: []geminiMessage{{
			ToolCalls: []geminiToolCall{{
				Args: map[string]any{"absolute_path": "/tmp/proj/file.txt"},
			}},
		}},
	}
	got := inferProjectPath(conv)
	if !strings.HasSuffix(got, "/tmp/proj") {
		t.Errorf("inferProjectPath = %q, want /tmp/proj", got)
	}
}

func TestResolveProjectPathFromHashKnownProjects(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	if _, err := db.EnsureProject(ctx, database, "/Users/davidcann/github/bb"); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	a := newAgent(database, t.TempDir(), t.TempDir())
	conv := &geminiConversation{
		ProjectHash: hashProjectPath("/Users/davidcann/github/bb"),
	}

	got := a.resolveProjectPath(conv)
	if got != "/Users/davidcann/github/bb" {
		t.Errorf("resolveProjectPath = %q, want %q", got, "/Users/davidcann/github/bb")
	}
}

func TestResolveProjectPathFromHashOldProjectPaths(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	projectID, err := db.EnsureProject(ctx, database, "/Users/davidcann/github/bb")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.SetProjectOldPaths(ctx, database, projectID, "/Users/davidcann/dev/bb-old"); err != nil {
		t.Fatalf("SetProjectOldPaths: %v", err)
	}

	a := newAgent(database, t.TempDir(), t.TempDir())
	conv := &geminiConversation{
		ProjectHash: hashProjectPath("/Users/davidcann/dev/bb-old"),
	}

	got := a.resolveProjectPath(conv)
	if got != "/Users/davidcann/dev/bb-old" {
		t.Errorf("resolveProjectPath = %q, want %q", got, "/Users/davidcann/dev/bb-old")
	}
}

func TestWatcherRepairsWrongHashedProject(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	hash := hashProjectPath("/Users/davidcann/github/bb")
	sessionID := "session-repair-0001"

	// Seed known real project path so hash can be resolved.
	realPID, err := db.EnsureProject(context.Background(), database, "/Users/davidcann/github/bb")
	if err != nil {
		t.Fatalf("EnsureProject real: %v", err)
	}
	// Seed wrong project/conversation (the old bug behavior).
	wrongPID, err := db.EnsureProject(context.Background(), database, filepath.Join(tmpDir, hash))
	if err != nil {
		t.Fatalf("EnsureProject wrong: %v", err)
	}
	if err := db.EnsureConversation(context.Background(), database, sessionID, wrongPID, "gemini"); err != nil {
		t.Fatalf("EnsureConversation wrong: %v", err)
	}

	convPath := filepath.Join(tmpDir, hash, "chats", "session-2026-02-12T12-00-00000000.json")
	writeConversationFile(t, convPath, map[string]any{
		"sessionId":   sessionID,
		"projectHash": hash,
		"messages": []map[string]any{
			{
				"id":        "m1",
				"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
				"type":      "user",
				"content":   "run tests",
			},
		},
	})

	a := newAgent(database, tmpDir, tmpDir)
	a.processSessionFile(context.Background(), convPath)

	detail, err := db.GetConversationDetail(context.Background(), database, sessionID)
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail.ProjectID != realPID {
		t.Errorf("project_id = %q, want %q (conversation should be repaired)", detail.ProjectID, realPID)
	}
}
