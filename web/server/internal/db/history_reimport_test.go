package db

import (
	"context"
	"testing"
	"time"
)

func TestDeleteConversationsAndMessagesByStartedAtWindow(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, database, "/tmp/reimport-window")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, database, "conv-old", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation conv-old: %v", err)
	}
	if err := EnsureConversation(ctx, database, "conv-recent", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation conv-recent: %v", err)
	}

	nowMs := time.Now().UnixMilli()
	oldTs := nowMs - int64((14*24*time.Hour)/time.Millisecond)
	recentTs := nowMs - int64((2*24*time.Hour)/time.Millisecond)

	if err := InsertMessages(ctx, database, []Message{
		{Timestamp: oldTs, ProjectID: projectID, ConversationID: "conv-old", Role: "user", Content: "old", RawJSON: "{}"},
		{Timestamp: recentTs, ProjectID: projectID, ConversationID: "conv-recent", Role: "user", Content: "recent", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	since := time.Now().Add(-7 * 24 * time.Hour)
	deletedConversations, deletedMessages, err := DeleteConversationsAndMessagesByStartedAtWindow(ctx, database, since)
	if err != nil {
		t.Fatalf("DeleteConversationsAndMessagesByStartedAtWindow: %v", err)
	}
	if deletedConversations != 1 {
		t.Fatalf("deleted conversations = %d, want 1", deletedConversations)
	}
	if deletedMessages != 1 {
		t.Fatalf("deleted messages = %d, want 1", deletedMessages)
	}

	var convCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM conversations").Scan(&convCount); err != nil {
		t.Fatalf("count conversations: %v", err)
	}
	if convCount != 1 {
		t.Fatalf("conversation count = %d, want 1", convCount)
	}

	var msgCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM messages").Scan(&msgCount); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if msgCount != 1 {
		t.Fatalf("message count = %d, want 1", msgCount)
	}

	var remainingID string
	if err := database.QueryRow("SELECT id FROM conversations LIMIT 1").Scan(&remainingID); err != nil {
		t.Fatalf("select remaining conversation: %v", err)
	}
	if remainingID != "conv-old" {
		t.Fatalf("remaining conversation = %q, want conv-old", remainingID)
	}
}

func TestDeleteConversationsAndMessagesByStartedAtWindowUpdatesMessagesFTS(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, database, "/tmp/reimport-fts")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, database, "conv-fts", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	ts := time.Now().Add(-2 * 24 * time.Hour).UnixMilli()
	if err := InsertMessages(ctx, database, []Message{
		{Timestamp: ts, ProjectID: projectID, ConversationID: "conv-fts", Role: "user", Content: "needle content", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var before int
	if err := database.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE conversation_id = ?", "conv-fts").Scan(&before); err != nil {
		t.Fatalf("count messages_fts before delete: %v", err)
	}
	if before != 1 {
		t.Fatalf("messages_fts before = %d, want 1", before)
	}

	since := time.Now().Add(-7 * 24 * time.Hour)
	if _, _, err := DeleteConversationsAndMessagesByStartedAtWindow(ctx, database, since); err != nil {
		t.Fatalf("DeleteConversationsAndMessagesByStartedAtWindow: %v", err)
	}

	var after int
	if err := database.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE conversation_id = ?", "conv-fts").Scan(&after); err != nil {
		t.Fatalf("count messages_fts after delete: %v", err)
	}
	if after != 0 {
		t.Fatalf("messages_fts after = %d, want 0", after)
	}
}
