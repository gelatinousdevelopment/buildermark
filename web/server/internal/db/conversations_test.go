package db

import (
	"context"
	"testing"
	"time"
)

func TestListConversations(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Set up projects and conversations.
	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	for _, id := range []string{"conv-a", "conv-b", "conv-c"} {
		if err := EnsureConversation(ctx, db, id, pid, "claude"); err != nil {
			t.Fatalf("EnsureConversation %s: %v", id, err)
		}
	}

	conversations, err := ListConversations(ctx, db, 100)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(conversations) != 3 {
		t.Fatalf("got %d conversations, want 3", len(conversations))
	}

	// Verify ordering by id.
	if conversations[0].ID != "conv-a" {
		t.Errorf("first conversation ID = %q, want %q", conversations[0].ID, "conv-a")
	}
	if conversations[0].ProjectID != pid {
		t.Errorf("first conversation ProjectID = %q, want %q", conversations[0].ProjectID, pid)
	}
}

func TestListConversationsEmpty(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	conversations, err := ListConversations(ctx, db, 100)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if conversations == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(conversations) != 0 {
		t.Errorf("got %d conversations, want 0", len(conversations))
	}
}

func TestGetConversationDetail(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Set up a project, conversation, turns, and a rating.
	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	turns := []Turn{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "conv-1", Role: "user", Content: "hello"},
		{Timestamp: 2000, ProjectID: pid, ConversationID: "conv-1", Role: "agent", Content: "hi"},
	}
	if err := InsertTurns(ctx, db, turns); err != nil {
		t.Fatalf("InsertTurns: %v", err)
	}

	// Add a rating for this conversation.
	if _, err := InsertRating(ctx, db, "conv-1", 5, "excellent", ""); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	detail, err := GetConversationDetail(ctx, db, "conv-1")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected non-nil conversation detail")
	}

	if detail.ID != "conv-1" {
		t.Errorf("ID = %q, want %q", detail.ID, "conv-1")
	}
	if detail.Agent != "claude" {
		t.Errorf("Agent = %q, want %q", detail.Agent, "claude")
	}
	if len(detail.Turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(detail.Turns))
	}
	if detail.Turns[0].Content != "hello" {
		t.Errorf("first turn content = %q, want %q", detail.Turns[0].Content, "hello")
	}
	if detail.Turns[1].Role != "agent" {
		t.Errorf("second turn role = %q, want %q", detail.Turns[1].Role, "agent")
	}
	if len(detail.Ratings) != 1 {
		t.Fatalf("got %d ratings, want 1", len(detail.Ratings))
	}
	if detail.Ratings[0].Rating != 5 {
		t.Errorf("rating = %d, want 5", detail.Ratings[0].Rating)
	}
}

func TestGetConversationDetailNotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	detail, err := GetConversationDetail(ctx, db, "nonexistent")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail != nil {
		t.Errorf("expected nil for nonexistent conversation, got %+v", detail)
	}
}

func TestGetConversationDetailEmptyTurnsAndRatings(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-empty", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	detail, err := GetConversationDetail(ctx, db, "conv-empty")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected non-nil conversation detail")
	}
	if detail.Turns == nil {
		t.Error("expected non-nil empty Turns slice")
	}
	if len(detail.Turns) != 0 {
		t.Errorf("expected 0 turns, got %d", len(detail.Turns))
	}
	if detail.Ratings == nil {
		t.Error("expected non-nil empty Ratings slice")
	}
	if len(detail.Ratings) != 0 {
		t.Errorf("expected 0 ratings, got %d", len(detail.Ratings))
	}
}

func TestParseTimeRFC3339(t *testing.T) {
	input := "2024-06-15T10:30:00.123456789Z"
	got := parseTime(input)

	want := time.Date(2024, 6, 15, 10, 30, 0, 123456789, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseTime(%q) = %v, want %v", input, got, want)
	}
}

func TestParseTimeSQLiteFormat(t *testing.T) {
	input := "2024-06-15 10:30:00"
	got := parseTime(input)

	want := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseTime(%q) = %v, want %v", input, got, want)
	}
}

func TestParseTimeInvalid(t *testing.T) {
	got := parseTime("not a time")
	if !got.IsZero() {
		t.Errorf("parseTime with invalid input = %v, want zero time", got)
	}
}
