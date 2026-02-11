package db

import (
	"context"
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

func TestEnsureProjectSetsLabel(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

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
	err = db.QueryRow("SELECT agent FROM conversations WHERE id = ?", "conv-1").Scan(&agent)
	if err != nil {
		t.Fatalf("query conversation: %v", err)
	}
	if agent != "claude" {
		t.Errorf("agent = %q, want %q", agent, "claude")
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

func TestInsertTurns(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	turns := []Turn{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: `{"type":"user"}`},
		{Timestamp: 2000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "hi there", RawJSON: `{"type":"assistant"}`},
		{Timestamp: 3000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "thanks", RawJSON: `{"type":"user"}`},
	}

	if err := InsertTurns(ctx, db, turns); err != nil {
		t.Fatalf("InsertTurns: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM turns WHERE conversation_id = ?", "conv-1").Scan(&count)
	if err != nil {
		t.Fatalf("count turns: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 turns, got %d", count)
	}

	// Verify content is stored correctly.
	var content string
	err = db.QueryRow("SELECT content FROM turns WHERE conversation_id = ? ORDER BY timestamp LIMIT 1", "conv-1").Scan(&content)
	if err != nil {
		t.Fatalf("query turn: %v", err)
	}
	if content != "hello" {
		t.Errorf("content = %q, want %q", content, "hello")
	}
}

func TestInsertTurnsEmpty(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Inserting zero turns should succeed.
	if err := InsertTurns(ctx, db, []Turn{}); err != nil {
		t.Fatalf("InsertTurns with empty slice: %v", err)
	}
}

func TestInsertTurnsDuplicateTimestamp(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	turns := []Turn{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: "{}"},
	}

	if err := InsertTurns(ctx, db, turns); err != nil {
		t.Fatalf("first InsertTurns: %v", err)
	}

	// Inserting again with the same conversation_id + timestamp should be ignored (INSERT OR IGNORE).
	if err := InsertTurns(ctx, db, turns); err != nil {
		t.Fatalf("second InsertTurns: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM turns WHERE conversation_id = ?", "conv-1").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 turn after duplicate insert, got %d", count)
	}
}

func TestInsertTurnsNearDuplicateContent(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Insert a turn, then try to insert the same content with a slightly different timestamp.
	first := []Turn{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: "{}"},
	}
	if err := InsertTurns(ctx, db, first); err != nil {
		t.Fatalf("first InsertTurns: %v", err)
	}

	// Same content, 2ms later — should be deduplicated.
	second := []Turn{
		{Timestamp: 1002, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: "{}"},
	}
	if err := InsertTurns(ctx, db, second); err != nil {
		t.Fatalf("second InsertTurns: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM turns WHERE conversation_id = ?", "conv-1").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 turn after near-duplicate insert, got %d", count)
	}
}

func TestInsertTurnsSameContentFarApart(t *testing.T) {
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
	turns := []Turn{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "yes", RawJSON: "{}"},
		{Timestamp: 60000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "yes", RawJSON: "{}"},
	}
	if err := InsertTurns(ctx, db, turns); err != nil {
		t.Fatalf("InsertTurns: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM turns WHERE conversation_id = ?", "conv-1").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 turns for same content far apart, got %d", count)
	}
}

func TestInsertTurnsBatchDeduplication(t *testing.T) {
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
	turns := []Turn{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: `{"v":1}`},
		{Timestamp: 1005, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: `{"v":2}`},
	}
	if err := InsertTurns(ctx, db, turns); err != nil {
		t.Fatalf("InsertTurns: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM turns WHERE conversation_id = ?", "conv-1").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 turn after batch dedup, got %d", count)
	}
}

func TestDeduplicateTurns(t *testing.T) {
	turns := []Turn{
		{Timestamp: 1000, ConversationID: "c1", Role: "user", Content: "hello"},
		{Timestamp: 1002, ConversationID: "c1", Role: "user", Content: "hello"},     // dup of first
		{Timestamp: 2000, ConversationID: "c1", Role: "agent", Content: "hello"},    // different role
		{Timestamp: 3000, ConversationID: "c1", Role: "user", Content: "goodbye"},   // different content
		{Timestamp: 3001, ConversationID: "c2", Role: "user", Content: "hello"},     // different conversation
		{Timestamp: 60000, ConversationID: "c1", Role: "user", Content: "hello"},    // same content, far apart
	}

	result := deduplicateTurns(turns)
	if len(result) != 5 {
		t.Errorf("expected 5 turns after dedup, got %d", len(result))
		for i, r := range result {
			t.Logf("  [%d] ts=%d conv=%s role=%s content=%q", i, r.Timestamp, r.ConversationID, r.Role, r.Content)
		}
	}
}
