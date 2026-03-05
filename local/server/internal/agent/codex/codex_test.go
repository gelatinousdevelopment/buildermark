package codex

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

// writeRolloutFile creates a JSONL file with the given rollout events.
func writeRolloutFile(t *testing.T, path string, events []rolloutEvent) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create rollout file: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("encode event: %v", err)
		}
	}
}

// writeJSONLObjects writes arbitrary objects to a JSONL file.
func writeJSONLObjects(t *testing.T, path string, objects []any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create jsonl file: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, obj := range objects {
		if err := enc.Encode(obj); err != nil {
			t.Fatalf("encode object: %v", err)
		}
	}
}

// --- Agent tests ---

func TestName(t *testing.T) {
	a := newAgent(nil, "", "")
	if a.Name() != "codex" {
		t.Errorf("Name() = %q, want %q", a.Name(), "codex")
	}
}

func TestParseCodexTimestampInvalidReturnsZero(t *testing.T) {
	if got := parseCodexTimestamp(nil); got != 0 {
		t.Fatalf("parseCodexTimestamp(nil) = %d, want 0", got)
	}
	if got := parseCodexTimestamp(json.RawMessage(`null`)); got != 0 {
		t.Fatalf("parseCodexTimestamp(null) = %d, want 0", got)
	}
	if got := parseCodexTimestamp(json.RawMessage(`"not-a-time"`)); got != 0 {
		t.Fatalf("parseCodexTimestamp(invalid) = %d, want 0", got)
	}
}

// --- Watcher tests ---

func TestWatcherProcessSessionFile(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	events := []rolloutEvent{
		{Type: "input", ThreadID: "thread-1", WorkingDir: "/proj/a", Content: "hello", Timestamp: 1000, Role: "user"},
		{Type: "item.completed", ThreadID: "thread-1", WorkingDir: "/proj/a", Timestamp: 2000, Item: rolloutItem{
			Type: "agent_message",
			Role: "assistant",
			Content: []rolloutContentBlock{
				{Type: "text", Text: "hi there"},
			},
		}},
		{Type: "input", ThreadID: "thread-1", WorkingDir: "/proj/a", Content: "thanks", Timestamp: 3000, Role: "user"},
	}

	rolloutPath := filepath.Join(sessionsDir, "2025", "01", "01", "rollout-1234567890-thread-1.jsonl")
	writeRolloutFile(t, rolloutPath, events)

	a := newAgent(database, sessionsDir, tmpDir)
	ctx := context.Background()
	a.processSessionFile(ctx, rolloutPath, nil)

	if n := countRows(t, database, "projects"); n != 1 {
		t.Errorf("projects: got %d, want 1", n)
	}
	if n := countRows(t, database, "conversations"); n != 1 {
		t.Errorf("conversations: got %d, want 1", n)
	}
	if n := countRows(t, database, "messages"); n != 3 {
		t.Errorf("messages: got %d, want 3", n)
	}

	// Verify role mapping.
	var role string
	err := database.QueryRow("SELECT role FROM messages WHERE conversation_id = 'thread-1' ORDER BY timestamp LIMIT 1").Scan(&role)
	if err != nil {
		t.Fatalf("query role: %v", err)
	}
	if role != "user" {
		t.Errorf("role = %q, want %q", role, "user")
	}
	err = database.QueryRow("SELECT role FROM messages WHERE conversation_id = 'thread-1' AND role = 'agent' LIMIT 1").Scan(&role)
	if err != nil {
		t.Fatalf("query agent role: %v", err)
	}
	if role != "agent" {
		t.Errorf("role = %q, want %q", role, "agent")
	}

	// Verify agent name stored on conversation.
	var agentName string
	err = database.QueryRow("SELECT agent FROM conversations WHERE id = 'thread-1'").Scan(&agentName)
	if err != nil {
		t.Fatalf("query agent: %v", err)
	}
	if agentName != "codex" {
		t.Errorf("agent = %q, want %q", agentName, "codex")
	}
}

func TestWatcherScanSinceFiltersOldFiles(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	events := []rolloutEvent{
		{Type: "input", ThreadID: "thread-old", WorkingDir: "/proj/old", Content: "old message", Timestamp: 1000, Role: "user"},
	}

	rolloutPath := filepath.Join(sessionsDir, "2020", "01", "01", "rollout-1000-thread-old.jsonl")
	writeRolloutFile(t, rolloutPath, events)

	// Set the file modification time to long ago.
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(rolloutPath, oldTime, oldTime)

	a := newAgent(database, sessionsDir, tmpDir)
	ctx := context.Background()
	n := a.ScanSince(ctx, time.Now().Add(-1*time.Hour), nil)

	if n != 0 {
		t.Errorf("ScanSince processed %d files, want 0 (old file should be skipped)", n)
	}
}

func TestWatcherScanSinceUsesEventTimestampsEvenWhenFileMtimeIsOld(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	now := time.Now().UTC()
	events := []rolloutEvent{
		{
			Type:       "input",
			ThreadID:   "thread-recent",
			WorkingDir: "/proj/recent",
			Content:    "recent message",
			Timestamp:  now.Add(-2 * time.Hour).UnixMilli(),
			Role:       "user",
		},
	}

	rolloutPath := filepath.Join(sessionsDir, "2020", "01", "01", "rollout-1000-thread-recent.jsonl")
	writeRolloutFile(t, rolloutPath, events)

	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(rolloutPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	a := newAgent(database, sessionsDir, tmpDir)
	ctx := context.Background()

	// Pre-create the project so it is tracked.
	if _, err := db.EnsureProject(ctx, database, "/proj/recent"); err != nil {
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

func TestWatcherSkipsFilesWithoutThreadID(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	// File with no thread_id and no working_dir — should be skipped entirely.
	events := []rolloutEvent{
		{Type: "input", Content: "hello", Timestamp: 1000, Role: "user"},
	}

	rolloutPath := filepath.Join(sessionsDir, "2025", "01", "01", "rollout-1000-unknown.jsonl")
	writeRolloutFile(t, rolloutPath, events)

	a := newAgent(database, sessionsDir, tmpDir)
	ctx := context.Background()
	a.processSessionFile(ctx, rolloutPath, nil)

	if n := countRows(t, database, "messages"); n != 0 {
		t.Errorf("messages: got %d, want 0 (no thread_id or working_dir)", n)
	}
}

func TestAppendDiffEntries(t *testing.T) {
	entries := []agent.Entry{
		{
			Timestamp: 1000,
			SessionID: "sess-1",
			Project:   "/proj/a",
			Role:      "agent",
			Display:   "```diff\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n```",
		},
	}

	out := agent.AppendDiffEntries(entries)
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[1].Timestamp != 1001 {
		t.Fatalf("diff timestamp = %d, want 1001", out[1].Timestamp)
	}
	if !strings.Contains(out[1].Display, "--- a/a.txt") {
		t.Fatalf("diff entry missing expected content: %q", out[1].Display)
	}
}

func TestAppendDiffEntriesFromRawJSON(t *testing.T) {
	entries := []agent.Entry{
		{
			Timestamp: 1000,
			SessionID: "sess-1",
			Project:   "/proj/a",
			Role:      "agent",
			Display:   "[response_item]",
			RawJSON:   `{"type":"response_item","payload":{"type":"function_call_output","output":"{\"resultDisplay\":\"diff --git a/x.txt b/x.txt\n--- a/x.txt\n+++ b/x.txt\n@@ -1 +1 @@\n-old\n+new\n\"}"}}`,
		},
	}

	out := agent.AppendDiffEntries(entries)
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[1].Timestamp != 1001 {
		t.Fatalf("diff timestamp = %d, want 1001", out[1].Timestamp)
	}
	if !strings.Contains(out[1].Display, "diff --git a/x.txt b/x.txt") {
		t.Fatalf("diff entry missing expected content: %q", out[1].Display)
	}
}

func TestWatcherReconcileOrphanedRating(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	// Insert an orphaned rating first.
	rating, err := db.InsertRating(context.Background(), database, "orphan-cid", 4, "great work", "")
	if err != nil {
		t.Fatalf("insert rating: %v", err)
	}

	// Create a session file that contains $bb.
	now := time.Now().UnixMilli()
	events := []rolloutEvent{
		{Type: "input", ThreadID: "thread-rated", WorkingDir: "/proj/rated", Content: "do something", Timestamp: now - 1000, Role: "user"},
		{Type: "item.completed", ThreadID: "thread-rated", WorkingDir: "/proj/rated", Timestamp: now - 500, Item: rolloutItem{
			Type: "agent_message",
			Role: "assistant",
			Content: []rolloutContentBlock{
				{Type: "text", Text: "done"},
			},
		}},
		{Type: "input", ThreadID: "thread-rated", WorkingDir: "/proj/rated", Content: "$bb 4 great work", Timestamp: now, Role: "user"},
	}

	rolloutPath := filepath.Join(sessionsDir, "2025", "01", "01", "rollout-12345-thread-rated.jsonl")
	writeRolloutFile(t, rolloutPath, events)

	a := newAgent(database, sessionsDir, tmpDir)
	ctx := context.Background()
	a.processSessionFile(ctx, rolloutPath, nil)

	// Check that the rating was reconciled.
	var conversationID string
	err = database.QueryRow("SELECT conversation_id FROM ratings WHERE id = ?", rating.ID).Scan(&conversationID)
	if err != nil {
		t.Fatalf("query rating: %v", err)
	}
	if conversationID != "thread-rated" {
		t.Errorf("conversation_id = %q, want %q", conversationID, "thread-rated")
	}
}

func TestWatcherProcessCurrentSchemaSessionFile(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	now := time.Now().UTC()
	rolloutPath := filepath.Join(sessionsDir, "2026", "02", "13", "rollout-2026-02-13T04-02-54-thread-current.jsonl")
	writeJSONLObjects(t, rolloutPath, []any{
		map[string]any{
			"timestamp": now.Add(-5 * time.Second).Format(time.RFC3339Nano),
			"type":      "session_meta",
			"payload": map[string]any{
				"id":    "thread-current",
				"cwd":   "/proj/current",
				"model": "gpt-5-codex",
			},
		},
		map[string]any{
			"timestamp": now.Add(-4 * time.Second).Format(time.RFC3339Nano),
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "internal system-expanded prompt"},
				},
			},
		},
		map[string]any{
			"timestamp": now.Add(-4 * time.Second).Format(time.RFC3339Nano),
			"type":      "event_msg",
			"payload": map[string]any{
				"type":    "user_message",
				"message": "run tests",
			},
		},
		map[string]any{
			"timestamp": now.Add(-3 * time.Second).Format(time.RFC3339Nano),
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{
					{"type": "output_text", "text": "all tests passed"},
				},
			},
		},
	})

	a := newAgent(database, sessionsDir, tmpDir)
	ctx := context.Background()
	a.processSessionFile(ctx, rolloutPath, nil)

	if n := countRows(t, database, "projects"); n != 1 {
		t.Errorf("projects: got %d, want 1", n)
	}
	if n := countRows(t, database, "conversations"); n != 1 {
		t.Errorf("conversations: got %d, want 1", n)
	}
	if n := countRows(t, database, "messages"); n != 4 {
		t.Errorf("messages: got %d, want 4", n)
	}

	var userCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = 'thread-current' AND role = 'user'").Scan(&userCount); err != nil {
		t.Fatalf("count user messages: %v", err)
	}
	if userCount != 1 {
		t.Errorf("user messages: got %d, want 1 (event_msg should be canonical user input)", userCount)
	}
	var promptCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = 'thread-current' AND message_type = 'prompt'").Scan(&promptCount); err != nil {
		t.Fatalf("count prompt messages: %v", err)
	}
	if promptCount != 1 {
		t.Errorf("prompt messages: got %d, want 1 (only the canonical user prompt)", promptCount)
	}
	var internalRole, internalType string
	if err := database.QueryRow(
		"SELECT role, message_type FROM messages WHERE conversation_id = 'thread-current' AND content = ?",
		"internal system-expanded prompt",
	).Scan(&internalRole, &internalType); err != nil {
		t.Fatalf("query internal wrapper message: %v", err)
	}
	if internalRole != "agent" || internalType != "log" {
		t.Errorf("internal wrapper message = (%q, %q), want (agent, log)", internalRole, internalType)
	}
	if err := database.QueryRow("SELECT user_prompt_count FROM conversations WHERE id = 'thread-current'").Scan(&promptCount); err != nil {
		t.Fatalf("query user_prompt_count: %v", err)
	}
	if promptCount != 1 {
		t.Errorf("user_prompt_count = %d, want 1", promptCount)
	}

	var agentName string
	if err := database.QueryRow("SELECT agent FROM conversations WHERE id = 'thread-current'").Scan(&agentName); err != nil {
		t.Fatalf("query conversation agent: %v", err)
	}
	if agentName != "codex" {
		t.Errorf("agent = %q, want %q", agentName, "codex")
	}
	var model string
	if err := database.QueryRow("SELECT model FROM messages WHERE conversation_id = 'thread-current' AND role = 'agent' ORDER BY timestamp DESC LIMIT 1").Scan(&model); err != nil {
		t.Fatalf("query message model: %v", err)
	}
	if model != "gpt-5-codex" {
		t.Errorf("model = %q, want %q", model, "gpt-5-codex")
	}
}

func TestWatcherDerivesDiffFromCurrentSchemaApplyPatch(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	now := time.Now().UTC()
	rolloutPath := filepath.Join(sessionsDir, "2026", "02", "14", "rollout-2026-02-14T08-00-00-thread-apply-patch.jsonl")
	writeJSONLObjects(t, rolloutPath, []any{
		map[string]any{
			"timestamp": now.Add(-3 * time.Second).Format(time.RFC3339Nano),
			"type":      "session_meta",
			"payload": map[string]any{
				"id":    "thread-apply-patch",
				"cwd":   "/proj/apply-patch",
				"model": "gpt-5-codex",
			},
		},
		map[string]any{
			"timestamp": now.Add(-2 * time.Second).Format(time.RFC3339Nano),
			"type":      "event_msg",
			"payload":   map[string]any{"type": "user_message", "message": "patch the file"},
		},
		map[string]any{
			"timestamp": now.Add(-1 * time.Second).Format(time.RFC3339Nano),
			"type":      "response_item",
			"payload": map[string]any{
				"type":  "custom_tool_call",
				"name":  "apply_patch",
				"input": "*** Begin Patch\n*** Update File: x.txt\n@@\n-old\n+new\n*** End Patch\n",
			},
		},
	})

	a := newAgent(database, sessionsDir, tmpDir)
	a.processSessionFile(context.Background(), rolloutPath, nil)

	var diffCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = 'thread-apply-patch' AND content LIKE '```diff%'").Scan(&diffCount); err != nil {
		t.Fatalf("count diff messages: %v", err)
	}
	if diffCount != 1 {
		t.Fatalf("diff messages = %d, want 1", diffCount)
	}
}

func TestWatcherDerivesMultipleDiffsFromSingleJSONPayload(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	now := time.Now().UTC()
	rolloutPath := filepath.Join(sessionsDir, "2026", "02", "14", "rollout-2026-02-14T08-00-00-thread-multi-diff.jsonl")
	writeJSONLObjects(t, rolloutPath, []any{
		map[string]any{
			"timestamp": now.Add(-3 * time.Second).Format(time.RFC3339Nano),
			"type":      "session_meta",
			"payload": map[string]any{
				"id":    "thread-multi-diff",
				"cwd":   "/proj/multi-diff",
				"model": "gpt-5-codex",
			},
		},
		map[string]any{
			"timestamp": now.Add(-2 * time.Second).Format(time.RFC3339Nano),
			"type":      "event_msg",
			"payload":   map[string]any{"type": "user_message", "message": "apply edits"},
		},
		map[string]any{
			"timestamp": now.Add(-1 * time.Second).Format(time.RFC3339Nano),
			"type":      "response_item",
			"payload": map[string]any{
				"type":   "function_call_output",
				"output": "{\"diffA\":\"diff --git a/a.txt b/a.txt\\n--- a/a.txt\\n+++ b/a.txt\\n@@ -1 +1 @@\\n-old-a\\n+new-a\\n\",\"diffB\":\"diff --git a/b.txt b/b.txt\\n--- a/b.txt\\n+++ b/b.txt\\n@@ -1 +1 @@\\n-old-b\\n+new-b\\n\",\"dupA\":\"diff --git a/a.txt b/a.txt\\n--- a/a.txt\\n+++ b/a.txt\\n@@ -1 +1 @@\\n-old-a\\n+new-a\\n\"}",
			},
		},
	})

	a := newAgent(database, sessionsDir, tmpDir)
	a.processSessionFile(context.Background(), rolloutPath, nil)

	var diffCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = 'thread-multi-diff' AND content LIKE '```diff%'").Scan(&diffCount); err != nil {
		t.Fatalf("count diff messages: %v", err)
	}
	if diffCount != 2 {
		t.Fatalf("diff messages = %d, want 2 (A + B, deduplicated)", diffCount)
	}
}

func TestEnrichCodexRawJSONAddsCwdWhenMissing(t *testing.T) {
	raw := `{"type":"response_item","payload":{"type":"function_call_output","output":"ok"}}`
	updated := enrichCodexRawJSON(raw, "/proj/enriched")
	if updated == raw {
		t.Fatal("expected enriched raw json to differ when cwd is missing")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(updated), &parsed); err != nil {
		t.Fatalf("unmarshal enriched json: %v", err)
	}
	if got, _ := parsed["cwd"].(string); got != "/proj/enriched" {
		t.Fatalf("cwd = %q, want %q", got, "/proj/enriched")
	}
}

func TestWatcherImportsReasoningSummaryText(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	now := time.Now().UTC()
	rolloutPath := filepath.Join(sessionsDir, "2026", "02", "22", "rollout-2026-02-22T17-59-57-thread-reasoning.jsonl")
	writeJSONLObjects(t, rolloutPath, []any{
		map[string]any{
			"timestamp": now.Add(-3 * time.Second).Format(time.RFC3339Nano),
			"type":      "session_meta",
			"payload": map[string]any{
				"id":    "thread-reasoning",
				"cwd":   "/proj/reasoning",
				"model": "gpt-5-codex",
			},
		},
		map[string]any{
			"timestamp": now.Add(-2 * time.Second).Format(time.RFC3339Nano),
			"type":      "event_msg",
			"payload":   map[string]any{"type": "user_message", "message": "reformat files"},
		},
		map[string]any{
			"timestamp": now.Add(-1 * time.Second).Format(time.RFC3339Nano),
			"type":      "response_item",
			"payload": map[string]any{
				"type": "reasoning",
				"summary": []map[string]any{
					{"type": "summary_text", "text": "**Reformatting changed files**"},
				},
				"content": nil,
			},
		},
	})

	a := newAgent(database, sessionsDir, tmpDir)
	a.processSessionFile(context.Background(), rolloutPath, nil)

	var got string
	if err := database.QueryRow("SELECT content FROM messages WHERE conversation_id = 'thread-reasoning' AND json_extract(raw_json, '$.type') = 'response_item' LIMIT 1").Scan(&got); err != nil {
		t.Fatalf("query reasoning summary message: %v", err)
	}
	if got != "**Reformatting changed files**" {
		t.Fatalf("reasoning summary content = %q, want %q", got, "**Reformatting changed files**")
	}
}

func TestWatcherUsesReasoningSummaryForConversationTitle(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	now := time.Now().UTC()
	rolloutPath := filepath.Join(sessionsDir, "2026", "02", "22", "rollout-2026-02-22T18-10-00-thread-reasoning-title.jsonl")
	writeJSONLObjects(t, rolloutPath, []any{
		map[string]any{
			"timestamp": now.Add(-3 * time.Second).Format(time.RFC3339Nano),
			"type":      "session_meta",
			"payload": map[string]any{
				"id":    "thread-reasoning-title",
				"cwd":   "/proj/reasoning-title",
				"model": "gpt-5-codex",
			},
		},
		map[string]any{
			"timestamp": now.Add(-2 * time.Second).Format(time.RFC3339Nano),
			"type":      "event_msg",
			"payload": map[string]any{
				"type":    "user_message",
				"message": "A long initial prompt that should not be the final title",
			},
		},
		map[string]any{
			"timestamp": now.Add(-1 * time.Second).Format(time.RFC3339Nano),
			"type":      "response_item",
			"payload": map[string]any{
				"type": "reasoning",
				"summary": []map[string]any{
					{"type": "summary_text", "text": "**Reformatting changed files**"},
				},
			},
		},
	})

	a := newAgent(database, sessionsDir, tmpDir)
	a.processSessionFile(context.Background(), rolloutPath, nil)

	var title string
	if err := database.QueryRow("SELECT title FROM conversations WHERE id = 'thread-reasoning-title'").Scan(&title); err != nil {
		t.Fatalf("query title: %v", err)
	}
	if title != "Reformatting changed files" {
		t.Fatalf("title = %q, want %q", title, "Reformatting changed files")
	}
}

func TestWatcherReconcileOrphanedRatingCurrentSchema(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	rating, err := db.InsertRating(context.Background(), database, "orphan-cid", 4, "great work", "")
	if err != nil {
		t.Fatalf("insert rating: %v", err)
	}

	now := time.Now().UTC()
	rolloutPath := filepath.Join(sessionsDir, "2026", "02", "13", "rollout-2026-02-13T04-02-54-thread-reconcile.jsonl")
	writeJSONLObjects(t, rolloutPath, []any{
		map[string]any{
			"timestamp": now.Add(-3 * time.Second).Format(time.RFC3339Nano),
			"type":      "session_meta",
			"payload": map[string]any{
				"id":  "thread-reconcile",
				"cwd": "/proj/reconcile",
			},
		},
		map[string]any{
			"timestamp": now.Add(-2 * time.Second).Format(time.RFC3339Nano),
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "[$bb](/tmp/skills/bb/SKILL.md) 4 great work"},
				},
			},
		},
	})

	a := newAgent(database, sessionsDir, tmpDir)
	ctx := context.Background()
	a.processSessionFile(ctx, rolloutPath, nil)

	var conversationID string
	err = database.QueryRow("SELECT conversation_id FROM ratings WHERE id = ?", rating.ID).Scan(&conversationID)
	if err != nil {
		t.Fatalf("query rating: %v", err)
	}
	if conversationID != "thread-reconcile" {
		t.Errorf("conversation_id = %q, want %q", conversationID, "thread-reconcile")
	}
}

// --- Session resolver tests ---

func TestSessionResolverWithThreadID(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	events := []rolloutEvent{
		{Type: "input", ThreadID: "thread-known", WorkingDir: "/proj/known", Content: "hello", Timestamp: 1000, Role: "user"},
		{Type: "item.completed", ThreadID: "thread-known", WorkingDir: "/proj/known", Timestamp: 2000, Item: rolloutItem{
			Type: "agent_message",
			Role: "assistant",
			Content: []rolloutContentBlock{
				{Type: "text", Text: "hi"},
			},
		}},
	}

	rolloutPath := filepath.Join(sessionsDir, "2025", "01", "01", "rollout-1000-thread-known.jsonl")
	writeRolloutFile(t, rolloutPath, events)

	a := newAgent(nil, sessionsDir, tmpDir)
	result := a.ResolveSession(4, "", "thread-known")

	if result.SessionID != "thread-known" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "thread-known")
	}
	if result.Project != "/proj/known" {
		t.Errorf("Project = %q, want %q", result.Project, "/proj/known")
	}
	if len(result.Entries) != 2 {
		t.Errorf("entries: got %d, want 2", len(result.Entries))
	}
}

func TestSessionResolverFallback(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	// No session files exist — should return fallback.
	a := newAgent(nil, sessionsDir, tmpDir)
	// Use a short timeout by checking the default behavior.
	result := a.ResolveSession(4, "test", "fallback-id")

	if result.SessionID != "fallback-id" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "fallback-id")
	}
	if len(result.Entries) != 0 {
		t.Errorf("entries: got %d, want 0", len(result.Entries))
	}
}

func TestSessionResolverCurrentSchema(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	now := time.Now().UTC()
	rolloutPath := filepath.Join(sessionsDir, "2026", "02", "13", "rollout-2026-02-13T04-02-54-thread-current.jsonl")
	writeJSONLObjects(t, rolloutPath, []any{
		map[string]any{
			"timestamp": now.Add(-4 * time.Second).Format(time.RFC3339Nano),
			"type":      "session_meta",
			"payload": map[string]any{
				"id":  "thread-current",
				"cwd": "/proj/current",
			},
		},
		map[string]any{
			"timestamp": now.Add(-3 * time.Second).Format(time.RFC3339Nano),
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "[$bb](/tmp/skills/bb/SKILL.md) 4 great work"},
				},
			},
		},
		map[string]any{
			"timestamp": now.Add(-2 * time.Second).Format(time.RFC3339Nano),
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{
					{"type": "output_text", "text": "rated"},
				},
			},
		},
	})

	a := newAgent(nil, sessionsDir, tmpDir)
	result := a.ResolveSession(4, "great work", "fallback-id")

	if result.SessionID != "thread-current" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "thread-current")
	}
	if result.Project != "/proj/current" {
		t.Errorf("Project = %q, want %q", result.Project, "/proj/current")
	}
	if len(result.Entries) != 2 {
		t.Errorf("entries: got %d, want 2", len(result.Entries))
	}
}

// --- collectSessionEntries tests ---

func TestCollectSessionEntries(t *testing.T) {
	tmpDir := t.TempDir()

	events := []rolloutEvent{
		{Type: "input", ThreadID: "thread-1", WorkingDir: "/proj/test", Content: "hello", Timestamp: 1000, Role: "user"},
		{Type: "item.completed", ThreadID: "thread-1", Timestamp: 2000, Item: rolloutItem{
			Type: "agent_message",
			Role: "assistant",
			Content: []rolloutContentBlock{
				{Type: "text", Text: "response one"},
				{Type: "text", Text: "response two"},
			},
		}},
		{Type: "input", ThreadID: "thread-1", Content: "follow up", Timestamp: 3000, Role: "user"},
	}

	path := filepath.Join(tmpDir, "rollout.jsonl")
	writeRolloutFile(t, path, events)

	entries, project := collectSessionEntries(path)

	if project != "/proj/test" {
		t.Errorf("project = %q, want %q", project, "/proj/test")
	}
	if len(entries) != 3 {
		t.Fatalf("entries: got %d, want 3", len(entries))
	}

	if entries[0].Role != "user" || entries[0].Display != "hello" {
		t.Errorf("entry[0] = %q/%q, want user/hello", entries[0].Role, entries[0].Display)
	}
	if entries[1].Role != "agent" || entries[1].Display != "response one\nresponse two" {
		t.Errorf("entry[1] = %q/%q, want agent/response one\\nresponse two", entries[1].Role, entries[1].Display)
	}
	if entries[2].Role != "user" || entries[2].Display != "follow up" {
		t.Errorf("entry[2] = %q/%q, want user/follow up", entries[2].Role, entries[2].Display)
	}
}

func TestCollectSessionEntriesMissingFile(t *testing.T) {
	entries, project := collectSessionEntries("/nonexistent/path.jsonl")
	if len(entries) != 0 {
		t.Errorf("entries: got %d, want 0", len(entries))
	}
	if project != "" {
		t.Errorf("project = %q, want empty", project)
	}
}

// --- parseRatingDisplay tests ---

func TestParseRatingDisplay(t *testing.T) {
	tests := []struct {
		input      string
		wantRating int
		wantNote   string
	}{
		{"$bb 4 great work", 4, "great work"},
		{"$bb 5", 5, ""},
		{"$bb 0 terrible", 0, "terrible"},
		{"$bb 3", 3, ""},
		{"$bb abc", -1, ""},
		{"$bb 6", -1, ""},
		{"$bb -1", -1, ""},
		{"[$bb](/tmp/path/SKILL.md) 4 great work", 4, "great work"},
	}

	for _, tt := range tests {
		rating, note := parseRatingDisplay(tt.input)
		if rating != tt.wantRating || note != tt.wantNote {
			t.Errorf("parseRatingDisplay(%q) = (%d, %q), want (%d, %q)",
				tt.input, rating, note, tt.wantRating, tt.wantNote)
		}
	}
}

// --- Conversation title tests ---

func TestReadSessionTitle(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	events := []rolloutEvent{
		{Type: "input", ThreadID: "thread-titled", WorkingDir: "/proj/titled", Content: "Help me refactor the auth module", Timestamp: 1000, Role: "user"},
		{Type: "item.completed", ThreadID: "thread-titled", Timestamp: 2000, Item: rolloutItem{
			Type: "agent_message",
			Role: "assistant",
			Content: []rolloutContentBlock{
				{Type: "text", Text: "Sure, let me look at the code."},
			},
		}},
	}

	rolloutPath := filepath.Join(sessionsDir, "2025", "01", "01", "rollout-1000-thread-titled.jsonl")
	writeRolloutFile(t, rolloutPath, events)

	title := readSessionTitle(sessionsDir, "thread-titled")
	if title != "Help me refactor the auth module" {
		t.Errorf("title = %q, want %q", title, "Help me refactor the auth module")
	}
}

func TestReadSessionTitleMarkdownHeading(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	events := []rolloutEvent{
		{Type: "input", ThreadID: "thread-md", WorkingDir: "/proj/md", Content: "# Refactor Auth\n\nPlease help me.", Timestamp: 1000, Role: "user"},
	}

	rolloutPath := filepath.Join(sessionsDir, "2025", "01", "01", "rollout-1000-thread-md.jsonl")
	writeRolloutFile(t, rolloutPath, events)

	title := readSessionTitle(sessionsDir, "thread-md")
	if title != "# Refactor Auth\n\nPlease help me." {
		t.Errorf("title = %q, want %q", title, "# Refactor Auth\n\nPlease help me.")
	}
}

func TestReadSessionTitleCurrentSchema(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "02", "13", "rollout-2026-02-13T04-02-54-thread-title-new.jsonl")
	writeJSONLObjects(t, rolloutPath, []any{
		map[string]any{
			"timestamp": "2026-02-13T04:02:54.264Z",
			"type":      "session_meta",
			"payload": map[string]any{
				"id":  "thread-title-new",
				"cwd": "/proj/title-new",
			},
		},
		map[string]any{
			"timestamp": "2026-02-13T04:02:59.909Z",
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "# Improve Session Parsing\n\nNeed to update codex parser."},
				},
			},
		},
	})

	title := readSessionTitle(sessionsDir, "thread-title-new")
	if title != "# Improve Session Parsing\n\nNeed to update codex parser." {
		t.Errorf("title = %q, want %q", title, "# Improve Session Parsing\n\nNeed to update codex parser.")
	}
}

func TestReadSessionTitlePrefersEventMsgUser(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "02", "13", "rollout-2026-02-13T04-02-54-thread-title-event-msg.jsonl")
	writeJSONLObjects(t, rolloutPath, []any{
		map[string]any{
			"timestamp": "2026-02-13T04:02:54.264Z",
			"type":      "session_meta",
			"payload": map[string]any{
				"id":  "thread-title-event-msg",
				"cwd": "/proj/title-new",
			},
		},
		map[string]any{
			"timestamp": "2026-02-13T04:02:55.000Z",
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "internal wrapper prompt"},
				},
			},
		},
		map[string]any{
			"timestamp": "2026-02-13T04:02:55.100Z",
			"type":      "event_msg",
			"payload": map[string]any{
				"type":    "user_message",
				"message": "# Real User Prompt\n\nDo the actual task.",
			},
		},
	})

	title := readSessionTitle(sessionsDir, "thread-title-event-msg")
	if title != "# Real User Prompt\n\nDo the actual task." {
		t.Errorf("title = %q, want %q", title, "# Real User Prompt\n\nDo the actual task.")
	}
}

func TestReadSessionTitleMissingFile(t *testing.T) {
	title := readSessionTitle("/nonexistent", "thread-none")
	if title != "" {
		t.Errorf("title = %q, want empty", title)
	}
}

func TestTitleFromPrompt(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple prompt", "simple prompt"},
		{"# Heading\nrest of prompt", "# Heading\nrest of prompt"},
		{"line one\n# Heading in line two\nmore", "line one\n# Heading in line two\nmore"},
		{strings.Repeat("a", 1001), strings.Repeat("a", 1000) + "..."},
	}

	for _, tt := range tests {
		got := agent.TitleFromPrompt(tt.input)
		if got != tt.want {
			t.Errorf("agent.TitleFromPrompt(%q) = %q, want %q", tt.input[:min(len(tt.input), 30)], got, tt.want)
		}
	}
}

// --- threadIDFromFilename tests ---

func TestThreadIDFromFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"rollout-1234567890-abc123.jsonl", "abc123"},
		{"rollout-1234567890-thread-with-dashes.jsonl", "thread-with-dashes"},
		{"other-format.jsonl", "other-format"},
	}

	for _, tt := range tests {
		got := threadIDFromFilename(tt.input)
		if got != tt.want {
			t.Errorf("threadIDFromFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- findSessionFile tests ---

func TestFindSessionFileByName(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	events := []rolloutEvent{
		{Type: "input", ThreadID: "thread-find", Content: "hello", Timestamp: 1000, Role: "user"},
	}

	rolloutPath := filepath.Join(sessionsDir, "2025", "01", "01", "rollout-1000-thread-find.jsonl")
	writeRolloutFile(t, rolloutPath, events)

	got := findSessionFile(sessionsDir, "thread-find")
	if got != rolloutPath {
		t.Errorf("findSessionFile = %q, want %q", got, rolloutPath)
	}
}

func TestFindSessionFileByThreadStarted(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	events := []rolloutEvent{
		{Type: "input", ThreadID: "hidden-id", WorkingDir: "/proj", Content: "hello", Timestamp: 1000, Role: "user"},
	}

	// Filename does NOT contain the thread ID.
	rolloutPath := filepath.Join(sessionsDir, "2025", "01", "01", "rollout-1000-other.jsonl")
	writeRolloutFile(t, rolloutPath, events)

	got := findSessionFile(sessionsDir, "hidden-id")
	if got != rolloutPath {
		t.Errorf("findSessionFile = %q, want %q", got, rolloutPath)
	}
}

func TestFindSessionFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	got := findSessionFile(tmpDir, "nonexistent")
	if got != "" {
		t.Errorf("findSessionFile = %q, want empty", got)
	}
}

// --- Multiple session files test ---

func TestWatcherMultipleSessionFiles(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	// Session 1
	events1 := []rolloutEvent{
		{Type: "input", ThreadID: "thread-a", WorkingDir: "/proj/a", Content: "task a", Timestamp: 1000, Role: "user"},
		{Type: "item.completed", ThreadID: "thread-a", Timestamp: 2000, Item: rolloutItem{
			Type: "agent_message", Role: "assistant",
			Content: []rolloutContentBlock{{Type: "text", Text: "done a"}},
		}},
	}
	writeRolloutFile(t, filepath.Join(sessionsDir, "2025", "01", "01", "rollout-1000-thread-a.jsonl"), events1)

	// Session 2
	events2 := []rolloutEvent{
		{Type: "input", ThreadID: "thread-b", WorkingDir: "/proj/b", Content: "task b", Timestamp: 3000, Role: "user"},
	}
	writeRolloutFile(t, filepath.Join(sessionsDir, "2025", "01", "01", "rollout-3000-thread-b.jsonl"), events2)

	a := newAgent(database, sessionsDir, tmpDir)
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
}
