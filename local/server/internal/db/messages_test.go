package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureProject(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	id, err := EnsureProject(ctx, db, "/home/user/myproject")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty project ID")
	}

	// Verify the project is actually in the database.
	var path string
	err = db.QueryRow("SELECT path FROM projects WHERE id = ?", id).Scan(&path)
	if err != nil {
		t.Fatalf("query project: %v", err)
	}
	if path != "/home/user/myproject" {
		t.Errorf("path = %q, want %q", path, "/home/user/myproject")
	}
}

func TestInsertMessagesPreservesDiffMessageTypeForAnyRole(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/home/user/myproject")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-diff-role", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	if err := InsertMessages(ctx, db, []Message{{
		Timestamp:      1000,
		ProjectID:      projectID,
		ConversationID: "conv-diff-role",
		Role:           "user",
		MessageType:    MessageTypeDiff,
		Content:        "```diff\none\n```",
		RawJSON:        `{"source":"derived_diff"}`,
	}}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var messageType string
	if err := db.QueryRow("SELECT message_type FROM messages WHERE conversation_id = 'conv-diff-role'").Scan(&messageType); err != nil {
		t.Fatalf("query message_type: %v", err)
	}
	if messageType != MessageTypeDiff {
		t.Fatalf("message_type = %q, want %q", messageType, MessageTypeDiff)
	}
}

func TestEnsureProjectSetsLabelFallback(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// When the path has no .git directory, falls back to last path component.
	id, err := EnsureProject(ctx, db, "/home/user/myproject")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	var label string
	err = db.QueryRow("SELECT label FROM projects WHERE id = ?", id).Scan(&label)
	if err != nil {
		t.Fatalf("query label: %v", err)
	}
	if label != "myproject" {
		t.Errorf("label = %q, want %q", label, "myproject")
	}
}

func TestEnsureProjectSetsLabelFromGitRoot(t *testing.T) {
	// Create a temp directory structure: reponame/.git/  and reponame/subdir/
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "myrepo")
	subDir := filepath.Join(repoDir, "packages", "frontend")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	db := setupTestDB(t)
	ctx := context.Background()

	// EnsureProject with a subdirectory path should detect the git root.
	id, err := EnsureProject(ctx, db, subDir)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	var label string
	err = db.QueryRow("SELECT label FROM projects WHERE id = ?", id).Scan(&label)
	if err != nil {
		t.Fatalf("query label: %v", err)
	}
	if label != "myrepo" {
		t.Errorf("label = %q, want %q", label, "myrepo")
	}
}

func TestRepoLabelNoGit(t *testing.T) {
	// When no .git exists, falls back to filepath.Base.
	label := RepoLabel("/some/fake/path")
	if label != "path" {
		t.Errorf("RepoLabel = %q, want %q", label, "path")
	}
}

func TestRepoLabelAtGitRoot(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "myrepo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	label := RepoLabel(repoDir)
	if label != "myrepo" {
		t.Errorf("RepoLabel = %q, want %q", label, "myrepo")
	}
}

func TestRepoLabelFromSubdir(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "myrepo")
	subDir := filepath.Join(repoDir, "src", "pkg")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	label := RepoLabel(subDir)
	if label != "myrepo" {
		t.Errorf("RepoLabel = %q, want %q", label, "myrepo")
	}
}

func TestEnsureProjectIdempotent(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	id1, err := EnsureProject(ctx, db, "/home/user/myproject")
	if err != nil {
		t.Fatalf("first EnsureProject: %v", err)
	}

	id2, err := EnsureProject(ctx, db, "/home/user/myproject")
	if err != nil {
		t.Fatalf("second EnsureProject: %v", err)
	}

	if id1 != id2 {
		t.Errorf("expected same ID for same path, got %q and %q", id1, id2)
	}
}

func TestEnsureProjectMatchesOldPaths(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	id, err := EnsureProject(ctx, db, "/home/user/newproject")
	if err != nil {
		t.Fatalf("EnsureProject new path: %v", err)
	}
	if err := SetProjectOldPaths(ctx, db, id, "/home/user/oldproject\n/home/user/even-older"); err != nil {
		t.Fatalf("SetProjectOldPaths: %v", err)
	}

	oldID, err := EnsureProject(ctx, db, "/home/user/oldproject")
	if err != nil {
		t.Fatalf("EnsureProject old path: %v", err)
	}
	if oldID != id {
		t.Fatalf("EnsureProject old path returned %q, want %q", oldID, id)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM projects").Scan(&count); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if count != 1 {
		t.Fatalf("project row count = %d, want 1", count)
	}
}

func TestEnsureProjectPrefersOldPathAliasOverLegacyExactPathProject(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	legacyID, err := EnsureProject(ctx, db, "/home/user/oldproject")
	if err != nil {
		t.Fatalf("EnsureProject legacy: %v", err)
	}
	canonicalID, err := EnsureProject(ctx, db, "/home/user/newproject")
	if err != nil {
		t.Fatalf("EnsureProject canonical: %v", err)
	}
	if err := SetProjectOldPaths(ctx, db, canonicalID, "/home/user/oldproject"); err != nil {
		t.Fatalf("SetProjectOldPaths: %v", err)
	}

	gotID, err := EnsureProject(ctx, db, "/home/user/oldproject")
	if err != nil {
		t.Fatalf("EnsureProject old path: %v", err)
	}
	if gotID != canonicalID {
		t.Fatalf("EnsureProject returned %q, want canonical %q (legacy %q)", gotID, canonicalID, legacyID)
	}
}

func TestEnsureProjectDifferentPaths(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	id1, err := EnsureProject(ctx, db, "/project/a")
	if err != nil {
		t.Fatalf("EnsureProject a: %v", err)
	}
	id2, err := EnsureProject(ctx, db, "/project/b")
	if err != nil {
		t.Fatalf("EnsureProject b: %v", err)
	}

	if id1 == id2 {
		t.Error("expected different IDs for different paths")
	}
}

func TestEnsureConversation(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	err = EnsureConversation(ctx, db, "conv-1", projectID, "claude")
	if err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Verify conversation exists.
	var agent string
	var startedAt int64
	var endedAt int64
	err = db.QueryRow("SELECT agent, started_at, ended_at FROM conversations WHERE id = ?", "conv-1").Scan(&agent, &startedAt, &endedAt)
	if err != nil {
		t.Fatalf("query conversation: %v", err)
	}
	if agent != "claude" {
		t.Errorf("agent = %q, want %q", agent, "claude")
	}
	if startedAt != 0 || endedAt != 0 {
		t.Errorf("expected started_at/ended_at defaults to 0, got %d/%d", startedAt, endedAt)
	}
}

func TestEnsureConversationIdempotent(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	err = EnsureConversation(ctx, db, "conv-1", projectID, "claude")
	if err != nil {
		t.Fatalf("first EnsureConversation: %v", err)
	}

	// Second call with same ID should not error.
	err = EnsureConversation(ctx, db, "conv-1", projectID, "claude")
	if err != nil {
		t.Fatalf("second EnsureConversation: %v", err)
	}

	// Should still be exactly one conversation.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM conversations WHERE id = ?", "conv-1").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 conversation, got %d", count)
	}

}

func TestInsertMessages(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	messages := []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: `{"type":"user"}`},
		{Timestamp: 2000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "hi there", RawJSON: `{"type":"assistant"}`},
		{Timestamp: 3000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "thanks", RawJSON: `{"type":"user"}`},
	}

	if err := InsertMessages(ctx, db, messages); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ?", "conv-1").Scan(&count)
	if err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 messages, got %d", count)
	}

	// Verify content is stored correctly.
	var content string
	err = db.QueryRow("SELECT content FROM messages WHERE conversation_id = ? ORDER BY timestamp LIMIT 1", "conv-1").Scan(&content)
	if err != nil {
		t.Fatalf("query message: %v", err)
	}
	if content != "hello" {
		t.Errorf("content = %q, want %q", content, "hello")
	}

	var startedAt int64
	var endedAt int64
	err = db.QueryRow("SELECT started_at, ended_at FROM conversations WHERE id = ?", "conv-1").Scan(&startedAt, &endedAt)
	if err != nil {
		t.Fatalf("query conversation bounds: %v", err)
	}
	if startedAt != 1000 {
		t.Errorf("started_at = %d, want 1000", startedAt)
	}
	if endedAt != 3000 {
		t.Errorf("ended_at = %d, want 3000", endedAt)
	}
}

func TestInsertMessagesFiltersShortBracketedMessagesAtIngest(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	messages := []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "<command-name>/clear</command-name>"},
		{Timestamp: 2000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "[progress]"},
		{Timestamp: 3000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "real prompt"},
		{Timestamp: 4000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "[assistant]"},
	}
	if err := InsertMessages(ctx, db, messages); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ?", "conv-1").Scan(&count); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 1 {
		t.Fatalf("message count = %d, want 1", count)
	}
}

func TestInsertMessagesFiltersHiddenMessagesEvenWithRawJSON(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// The critical bug: messages with RawJSON were bypassing the filter.
	// All of these should be filtered even though they have RawJSON set.
	messages := []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "[progress]", RawJSON: `{"type":"progress"}`},
		{Timestamp: 2000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "[assistant]", RawJSON: `{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"..."}]}}`},
		{Timestamp: 3000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "<command-name>/clear</command-name>", RawJSON: `{"type":"user"}`},
		{Timestamp: 4000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "<local-command-caveat>Caveat...</local-command-caveat>", RawJSON: `{"type":"user"}`},
		{Timestamp: 5000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "[Request interrupted by user for tool use]", RawJSON: `{"type":"user"}`},
		{Timestamp: 6000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "[user]", RawJSON: `{"type":"user"}`},
		// This one should be kept.
		{Timestamp: 7000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "real user message", RawJSON: `{"type":"user"}`},
	}
	if err := InsertMessages(ctx, db, messages); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ?", "conv-1").Scan(&count); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 1 {
		rows, _ := db.Query("SELECT content FROM messages WHERE conversation_id = ? ORDER BY timestamp", "conv-1")
		defer rows.Close()
		for rows.Next() {
			var c string
			rows.Scan(&c)
			t.Logf("  got message: %q", c)
		}
		t.Fatalf("message count = %d, want 1 (only 'real user message')", count)
	}
}

func TestInsertMessagesKeepsLongBracketedMessages(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	longPayload := "<" + strings.Repeat("a", 260) + ">"
	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: longPayload},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ?", "conv-1").Scan(&count); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 1 {
		t.Fatalf("message count = %d, want 1", count)
	}
}

func TestInsertMessagesEmpty(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Inserting zero messages should succeed.
	if err := InsertMessages(ctx, db, []Message{}); err != nil {
		t.Fatalf("InsertMessages with empty slice: %v", err)
	}
}

func TestInsertMessagesDuplicateTimestamp(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	messages := []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: "{}"},
	}

	if err := InsertMessages(ctx, db, messages); err != nil {
		t.Fatalf("first InsertMessages: %v", err)
	}

	// Inserting again with the same conversation_id + timestamp should be ignored (INSERT OR IGNORE).
	if err := InsertMessages(ctx, db, messages); err != nil {
		t.Fatalf("second InsertMessages: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ?", "conv-1").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 message after duplicate insert, got %d", count)
	}
}

func TestInsertMessagesNearDuplicateContent(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Insert a message, then try to insert the same content with a slightly different timestamp.
	first := []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: "{}"},
	}
	if err := InsertMessages(ctx, db, first); err != nil {
		t.Fatalf("first InsertMessages: %v", err)
	}

	// Same content, 2ms later — should be deduplicated.
	second := []Message{
		{Timestamp: 1002, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: "{}"},
	}
	if err := InsertMessages(ctx, db, second); err != nil {
		t.Fatalf("second InsertMessages: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ?", "conv-1").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 message after near-duplicate insert, got %d", count)
	}
}

func TestInsertMessagesDifferentModelNotDeduplicated(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	msgs := []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Model: "claude-3-7-sonnet", Content: "same", RawJSON: "{}"},
		{Timestamp: 1001, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Model: "claude-3-5-sonnet", Content: "same", RawJSON: "{}"},
	}
	if err := InsertMessages(ctx, db, msgs); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ?", "conv-1").Scan(&count); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 messages (different models), got %d", count)
	}
}

func TestInsertMessagesBackfillsModelOnExistingTimestampRow(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	initial := []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "hello", RawJSON: "{}"},
	}
	if err := InsertMessages(ctx, db, initial); err != nil {
		t.Fatalf("InsertMessages initial: %v", err)
	}

	rescan := []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Model: "claude-3-7-sonnet", Content: "hello", RawJSON: "{}"},
	}
	if err := InsertMessages(ctx, db, rescan); err != nil {
		t.Fatalf("InsertMessages rescan: %v", err)
	}

	var model string
	if err := db.QueryRow("SELECT model FROM messages WHERE conversation_id = ? AND timestamp = 1000", "conv-1").Scan(&model); err != nil {
		t.Fatalf("query model: %v", err)
	}
	if model != "claude-3-7-sonnet" {
		t.Errorf("model = %q, want %q", model, "claude-3-7-sonnet")
	}
}

func TestInsertMessagesSameContentFarApart(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Same content but far apart (>10s) — both should be inserted.
	messages := []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "yes", RawJSON: "{}"},
		{Timestamp: 60000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "yes", RawJSON: "{}"},
	}
	if err := InsertMessages(ctx, db, messages); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ?", "conv-1").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 messages for same content far apart, got %d", count)
	}
}

func TestInsertMessagesBatchDeduplication(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Same content in a single batch with close timestamps — should deduplicate within the batch.
	messages := []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: `{"v":1}`},
		{Timestamp: 1005, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: `{"v":2}`},
	}
	if err := InsertMessages(ctx, db, messages); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ?", "conv-1").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 message after batch dedup, got %d", count)
	}
}

func TestInsertMessagesUpdatesConversationBounds(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 2000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "middle", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages initial: %v", err)
	}

	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "first", RawJSON: "{}"},
		{Timestamp: 3000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "last", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages second: %v", err)
	}

	var startedAt int64
	var endedAt int64
	if err := db.QueryRow("SELECT started_at, ended_at FROM conversations WHERE id = ?", "conv-1").Scan(&startedAt, &endedAt); err != nil {
		t.Fatalf("query conversation bounds: %v", err)
	}
	if startedAt != 1000 {
		t.Errorf("started_at = %d, want 1000", startedAt)
	}
	// ended_at should reflect only user messages, not the agent message at 3000.
	if endedAt != 2000 {
		t.Errorf("ended_at = %d, want 2000 (last user message)", endedAt)
	}
}

func TestInsertMessagesEndedAtOnlyFromUserMessages(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Insert user and agent messages where agent has later timestamp.
	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "prompt", RawJSON: "{}"},
		{Timestamp: 2000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "thinking...", RawJSON: "{}"},
		{Timestamp: 3000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "follow-up", RawJSON: "{}"},
		{Timestamp: 5000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "final answer", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var startedAt, endedAt int64
	if err := db.QueryRow("SELECT started_at, ended_at FROM conversations WHERE id = ?", "conv-1").Scan(&startedAt, &endedAt); err != nil {
		t.Fatalf("query bounds: %v", err)
	}
	if startedAt != 1000 {
		t.Errorf("started_at = %d, want 1000", startedAt)
	}
	if endedAt != 3000 {
		t.Errorf("ended_at = %d, want 3000 (last user message, not agent at 5000)", endedAt)
	}
}

func TestInsertMessagesAgentOnlyDoesNotChangeEndedAt(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// First insert a user message to set ended_at.
	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages user: %v", err)
	}

	// Now insert only agent messages with later timestamps.
	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 5000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "response", RawJSON: "{}"},
		{Timestamp: 8000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "more response", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages agent: %v", err)
	}

	var startedAt, endedAt int64
	if err := db.QueryRow("SELECT started_at, ended_at FROM conversations WHERE id = ?", "conv-1").Scan(&startedAt, &endedAt); err != nil {
		t.Fatalf("query bounds: %v", err)
	}
	if startedAt != 1000 {
		t.Errorf("started_at = %d, want 1000", startedAt)
	}
	// ended_at should remain at 1000 (the user message), not advance to 8000.
	if endedAt != 1000 {
		t.Errorf("ended_at = %d, want 1000 (should not advance from agent-only messages)", endedAt)
	}
}

func TestInsertMessagesStartedAtUsesAllMessages(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Insert user message first.
	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 5000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "prompt", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages user: %v", err)
	}

	// Insert agent message with earlier timestamp.
	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "system init", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages agent: %v", err)
	}

	var startedAt, endedAt int64
	if err := db.QueryRow("SELECT started_at, ended_at FROM conversations WHERE id = ?", "conv-1").Scan(&startedAt, &endedAt); err != nil {
		t.Fatalf("query bounds: %v", err)
	}
	// started_at should use the agent message (earliest overall).
	if startedAt != 1000 {
		t.Errorf("started_at = %d, want 1000 (agent message is earliest)", startedAt)
	}
	// ended_at should use the user message only.
	if endedAt != 5000 {
		t.Errorf("ended_at = %d, want 5000 (user message)", endedAt)
	}
}

func TestDeduplicateMessages(t *testing.T) {
	messages := []Message{
		{Timestamp: 1000, ConversationID: "c1", Role: "user", Content: "hello"},
		{Timestamp: 1002, ConversationID: "c1", Role: "user", Content: "hello"},   // dup of first
		{Timestamp: 2000, ConversationID: "c1", Role: "agent", Content: "hello"},  // different role
		{Timestamp: 3000, ConversationID: "c1", Role: "user", Content: "goodbye"}, // different content
		{Timestamp: 3001, ConversationID: "c2", Role: "user", Content: "hello"},   // different conversation
		{Timestamp: 60000, ConversationID: "c1", Role: "user", Content: "hello"},  // same content, far apart
	}

	result := deduplicateMessages(messages)
	if len(result) != 5 {
		t.Errorf("expected 5 messages after dedup, got %d", len(result))
		for i, r := range result {
			t.Logf("  [%d] ts=%d conv=%s role=%s content=%q", i, r.Timestamp, r.ConversationID, r.Role, r.Content)
		}
	}
}

func TestInsertMessagesSlashCommandDoesNotAdvanceEndedAt(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Insert a real user prompt.
	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "real prompt", RawJSON: `{}`},
	}); err != nil {
		t.Fatalf("InsertMessages real: %v", err)
	}

	// Insert slash commands and $bb commands (type=log) with later timestamps.
	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 5000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "/rate-buildermark 5", RawJSON: `{}`},
		{Timestamp: 6000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "/clear", RawJSON: `{}`},
		{Timestamp: 7000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "$bb rate 3", RawJSON: `{}`},
	}); err != nil {
		t.Fatalf("InsertMessages commands: %v", err)
	}

	var startedAt, endedAt int64
	if err := db.QueryRow("SELECT started_at, ended_at FROM conversations WHERE id = ?", "conv-1").Scan(&startedAt, &endedAt); err != nil {
		t.Fatalf("query bounds: %v", err)
	}
	if startedAt != 1000 {
		t.Errorf("started_at = %d, want 1000", startedAt)
	}
	// ended_at should remain at 1000 (the real prompt), not advance to 7000.
	if endedAt != 1000 {
		t.Errorf("ended_at = %d, want 1000 (slash commands should not advance ended_at)", endedAt)
	}
}

func TestInsertMessagesExcludesBbCommandsFromPromptCount(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	messages := []Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "real user prompt", RawJSON: `{"type":"user"}`},
		{Timestamp: 2000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "/rate-buildermark 5", RawJSON: `{"type":"user"}`},
		{Timestamp: 3000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "$bb rate 3 good work", RawJSON: `{"type":"user"}`},
		{Timestamp: 4000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "agent response", RawJSON: `{"type":"assistant"}`},
		{Timestamp: 5000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "/clear ", RawJSON: `{"type":"user"}`},
		{Timestamp: 6000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "/help", RawJSON: `{"type":"user"}`},
	}
	if err := InsertMessages(ctx, db, messages); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var promptCount int
	if err := db.QueryRow("SELECT user_prompt_count FROM conversations WHERE id = ?", "conv-1").Scan(&promptCount); err != nil {
		t.Fatalf("query user_prompt_count: %v", err)
	}
	if promptCount != 1 {
		t.Errorf("user_prompt_count = %d, want 1 (only 'real user prompt')", promptCount)
	}
}
