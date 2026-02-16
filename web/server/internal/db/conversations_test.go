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

	// Set up a project, conversation, messages, and a rating.
	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	messages := []Message{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "conv-1", Role: "user", Content: "hello"},
		{Timestamp: 2000, ProjectID: pid, ConversationID: "conv-1", Role: "agent", Model: "claude-3-7-sonnet", Content: "hi"},
	}
	if err := InsertMessages(ctx, db, messages); err != nil {
		t.Fatalf("InsertMessages: %v", err)
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
	if len(detail.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(detail.Messages))
	}
	if detail.Messages[0].Content != "hi" {
		t.Errorf("first turn content = %q, want %q (most recent first)", detail.Messages[0].Content, "hi")
	}
	if detail.Messages[1].Role != "user" {
		t.Errorf("second turn role = %q, want %q", detail.Messages[1].Role, "user")
	}
	if detail.Messages[0].Model != "claude-3-7-sonnet" {
		t.Errorf("first turn model = %q, want %q", detail.Messages[0].Model, "claude-3-7-sonnet")
	}
	if len(detail.Ratings) != 1 {
		t.Fatalf("got %d ratings, want 1", len(detail.Ratings))
	}
	if detail.Ratings[0].Rating != 5 {
		t.Errorf("rating = %d, want 5", detail.Ratings[0].Rating)
	}
}

func TestGetConversationDetailFiltersMessages(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-filter", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	messages := []Message{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "real prompt"},
		{Timestamp: 2000, ProjectID: pid, ConversationID: "conv-filter", Role: "agent", Content: "response"},
		// Should be filtered out:
		{Timestamp: 3000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: ""},
		{Timestamp: 4000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "  "},
		{Timestamp: 5000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "[user]"},
		{Timestamp: 6000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "<command-message>something</command-message>"},
		{Timestamp: 6500, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "<command-name>/clear</command-name>\n            <command-message>clear</command-message>\n            <command-args></command-args>"},
		{Timestamp: 7000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "/clear"},
		{Timestamp: 8000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "/new"},
		{Timestamp: 9000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "[Pasted text #1 from clipboard]"},
		{Timestamp: 10000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "[Pasted text #42 some long description here]"},
		// Should be preserved (agent/system logs are intentionally visible):
		{Timestamp: 11000, ProjectID: pid, ConversationID: "conv-filter", Role: "agent", Content: "<command-name>/clear</command-name>\n            <command-message>clear</command-message>\n            <command-args></command-args>"},
	}
	if err := InsertMessages(ctx, db, messages); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	detail, err := GetConversationDetail(ctx, db, "conv-filter")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}

	// "real prompt", "response", and the agent command/system log should survive.
	if len(detail.Messages) != 3 {
		contents := make([]string, len(detail.Messages))
		for i, m := range detail.Messages {
			contents[i] = m.Content
		}
		t.Fatalf("got %d messages %v, want 3", len(detail.Messages), contents)
	}
}

func TestGetConversationDetailRatingMatching(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-rate", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// The /zrate message timestamp and rating createdAt are within 120s.
	ratingCreatedAt := time.Now().UTC()
	zrateTimestamp := ratingCreatedAt.UnixMilli() + 500 // 500ms after rating

	messages := []Message{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "conv-rate", Role: "user", Content: "do something"},
		{Timestamp: zrateTimestamp, ProjectID: pid, ConversationID: "conv-rate", Role: "user", Content: "/zrate 5"},
	}
	if err := InsertMessages(ctx, db, messages); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	rating, err := InsertRating(ctx, db, "conv-rate", 5, "", "")
	if err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	detail, err := GetConversationDetail(ctx, db, "conv-rate")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}

	// The /zrate message should be removed from messages.
	if len(detail.Messages) != 1 {
		t.Fatalf("got %d messages, want 1 (zrate message should be removed)", len(detail.Messages))
	}
	if detail.Messages[0].Content != "do something" {
		t.Errorf("remaining message = %q, want %q", detail.Messages[0].Content, "do something")
	}

	// The rating should have a matched timestamp.
	if len(detail.Ratings) != 1 {
		t.Fatalf("got %d ratings, want 1", len(detail.Ratings))
	}
	_ = rating
	if detail.Ratings[0].MatchedTimestamp == nil {
		t.Fatal("expected MatchedTimestamp to be set")
	}
	if *detail.Ratings[0].MatchedTimestamp != zrateTimestamp {
		t.Errorf("MatchedTimestamp = %d, want %d", *detail.Ratings[0].MatchedTimestamp, zrateTimestamp)
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

func TestGetConversationDetailEmptyMessagesAndRatings(t *testing.T) {
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
	if detail.Messages == nil {
		t.Error("expected non-nil empty Messages slice")
	}
	if len(detail.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(detail.Messages))
	}
	if detail.Ratings == nil {
		t.Error("expected non-nil empty Ratings slice")
	}
	if len(detail.Ratings) != 0 {
		t.Errorf("expected 0 ratings, got %d", len(detail.Ratings))
	}
}

func TestListUntitledConversations(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	// Create two conversations: one with title, one without.
	if err := EnsureConversation(ctx, db, "conv-titled", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	if err := UpdateConversationTitle(ctx, db, "conv-titled", "Has a title"); err != nil {
		t.Fatalf("UpdateConversationTitle: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-untitled", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	untitled, err := ListUntitledConversations(ctx, db, "claude")
	if err != nil {
		t.Fatalf("ListUntitledConversations: %v", err)
	}
	if len(untitled) != 1 {
		t.Fatalf("got %d untitled, want 1", len(untitled))
	}
	if untitled[0].ID != "conv-untitled" {
		t.Errorf("untitled ID = %q, want %q", untitled[0].ID, "conv-untitled")
	}
	if untitled[0].ProjectPath != "/test/project" {
		t.Errorf("untitled ProjectPath = %q, want %q", untitled[0].ProjectPath, "/test/project")
	}
}

func TestUpdateConversationTitle(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-title", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Title should default to empty.
	detail, err := GetConversationDetail(ctx, db, "conv-title")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail.Title != "" {
		t.Errorf("initial title = %q, want empty", detail.Title)
	}

	// Update title.
	if err := UpdateConversationTitle(ctx, db, "conv-title", "Fix the login bug"); err != nil {
		t.Fatalf("UpdateConversationTitle: %v", err)
	}

	detail, err = GetConversationDetail(ctx, db, "conv-title")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail.Title != "Fix the login bug" {
		t.Errorf("title = %q, want %q", detail.Title, "Fix the login bug")
	}

	// Also check ListConversations returns title.
	convs, err := ListConversations(ctx, db, 100)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convs) != 1 || convs[0].Title != "Fix the login bug" {
		t.Errorf("ListConversations title = %q, want %q", convs[0].Title, "Fix the login bug")
	}
}

func TestUpdateConversationProject(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pidA, err := EnsureProject(ctx, db, "/test/project-a")
	if err != nil {
		t.Fatalf("EnsureProject A: %v", err)
	}
	pidB, err := EnsureProject(ctx, db, "/test/project-b")
	if err != nil {
		t.Fatalf("EnsureProject B: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-project", pidA, "gemini"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	if err := UpdateConversationProject(ctx, db, "conv-project", pidB); err != nil {
		t.Fatalf("UpdateConversationProject: %v", err)
	}

	detail, err := GetConversationDetail(ctx, db, "conv-project")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail.ProjectID != pidB {
		t.Errorf("project_id = %q, want %q", detail.ProjectID, pidB)
	}
}

func TestParseTimeRFC3339(t *testing.T) {
	input := "2024-06-15T10:30:00.123456789Z"
	got, err := parseTime(input)
	if err != nil {
		t.Fatalf("parseTime(%q): %v", input, err)
	}

	want := time.Date(2024, 6, 15, 10, 30, 0, 123456789, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseTime(%q) = %v, want %v", input, got, want)
	}
}

func TestParseTimeSQLiteFormat(t *testing.T) {
	input := "2024-06-15 10:30:00"
	got, err := parseTime(input)
	if err != nil {
		t.Fatalf("parseTime(%q): %v", input, err)
	}

	want := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseTime(%q) = %v, want %v", input, got, want)
	}
}

func TestParseTimeInvalid(t *testing.T) {
	got, err := parseTime("not a time")
	if err == nil {
		t.Fatal("expected parseTime to return an error")
	}
	if !got.IsZero() {
		t.Errorf("parseTime with invalid input = %v, want zero time with error", got)
	}
}
