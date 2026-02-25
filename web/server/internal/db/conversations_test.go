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
	if conversations[0].StartedAt != 0 || conversations[0].EndedAt != 0 {
		t.Errorf("expected default startedAt/endedAt to be 0, got %d/%d", conversations[0].StartedAt, conversations[0].EndedAt)
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
	if detail.StartedAt != 1000 {
		t.Errorf("StartedAt = %d, want %d", detail.StartedAt, 1000)
	}
	if detail.EndedAt != 2000 {
		t.Errorf("EndedAt = %d, want %d", detail.EndedAt, 2000)
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
		{Timestamp: 6750, ProjectID: pid, ConversationID: "conv-filter", Role: "User", Content: "<command-name source=\"shell\">/clear</command-name>"},
		{Timestamp: 7000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "/clear"},
		{Timestamp: 8000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "/new"},
		{Timestamp: 9000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "[Pasted text #1 from clipboard]"},
		{Timestamp: 10000, ProjectID: pid, ConversationID: "conv-filter", Role: "user", Content: "[Pasted text #42 some long description here]"},
		// Ingest-time filtering now also applies to non-user roles:
		{Timestamp: 11000, ProjectID: pid, ConversationID: "conv-filter", Role: "agent", Content: "<command-name>/clear</command-name>\n            <command-message>clear</command-message>\n            <command-args></command-args>"},
	}
	if err := InsertMessages(ctx, db, messages); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	detail, err := GetConversationDetail(ctx, db, "conv-filter")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}

	// Only "real prompt" and "response" should survive.
	if len(detail.Messages) != 2 {
		contents := make([]string, len(detail.Messages))
		for i, m := range detail.Messages {
			contents[i] = m.Content
		}
		t.Fatalf("got %d messages %v, want 2", len(detail.Messages), contents)
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

	// The /bb message timestamp and rating createdAt are within 120s.
	ratingCreatedAt := time.Now().UTC().UnixMilli()
	ratingTimestamp := ratingCreatedAt + 500 // 500ms after rating

	messages := []Message{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "conv-rate", Role: "user", Content: "do something"},
		{Timestamp: ratingTimestamp, ProjectID: pid, ConversationID: "conv-rate", Role: "user", Content: "/bb 5"},
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

	// The /bb message should be removed from messages.
	if len(detail.Messages) != 1 {
		t.Fatalf("got %d messages, want 1 (rating message should be removed)", len(detail.Messages))
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
	if *detail.Ratings[0].MatchedTimestamp != ratingTimestamp {
		t.Errorf("MatchedTimestamp = %d, want %d", *detail.Ratings[0].MatchedTimestamp, ratingTimestamp)
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

func TestGetConversationDetailByTempConversationID(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-actual", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "conv-actual", Role: "user", Content: "hello"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}
	if _, err := InsertRatingWithTemp(ctx, db, "conv-actual", "temp-123", 5, "great", ""); err != nil {
		t.Fatalf("InsertRatingWithTemp: %v", err)
	}

	detail, err := GetConversationDetail(ctx, db, "temp-123")
	if err != nil {
		t.Fatalf("GetConversationDetail by temp ID: %v", err)
	}
	if detail == nil {
		t.Fatal("expected non-nil conversation detail")
	}
	if detail.ID != "conv-actual" {
		t.Errorf("detail.ID = %q, want %q", detail.ID, "conv-actual")
	}
	if len(detail.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(detail.Messages))
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
	if detail.StartedAt != 0 || detail.EndedAt != 0 {
		t.Errorf("expected StartedAt/EndedAt of empty conversation to be 0/0, got %d/%d", detail.StartedAt, detail.EndedAt)
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

func TestUpdateConversationParent(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "child-conv", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Initially parent should be empty.
	detail, err := GetConversationDetail(ctx, db, "child-conv")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail.ParentConversationID != "" {
		t.Errorf("initial parent = %q, want empty", detail.ParentConversationID)
	}

	// Set parent.
	if err := UpdateConversationParent(ctx, db, "child-conv", "parent-conv"); err != nil {
		t.Fatalf("UpdateConversationParent: %v", err)
	}

	detail, err = GetConversationDetail(ctx, db, "child-conv")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail.ParentConversationID != "parent-conv" {
		t.Errorf("parent = %q, want %q", detail.ParentConversationID, "parent-conv")
	}

	// Idempotent: setting again should not overwrite.
	if err := UpdateConversationParent(ctx, db, "child-conv", "other-parent"); err != nil {
		t.Fatalf("UpdateConversationParent (idempotent): %v", err)
	}

	detail, err = GetConversationDetail(ctx, db, "child-conv")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail.ParentConversationID != "parent-conv" {
		t.Errorf("parent after idempotent call = %q, want %q (should not change)", detail.ParentConversationID, "parent-conv")
	}
}
