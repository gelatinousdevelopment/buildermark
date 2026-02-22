package db

import (
	"context"
	"testing"
	"time"
)

func TestInsertRating(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	r, err := InsertRating(ctx, db, "conv-1", 4, "good work", "")
	if err != nil {
		t.Fatalf("InsertRating: %v", err)
	}
	if r.ID == "" {
		t.Error("expected non-empty ID")
	}
	if r.ConversationID != "conv-1" {
		t.Errorf("ConversationID = %q, want %q", r.ConversationID, "conv-1")
	}
	if r.TempConversationID != "conv-1" {
		t.Errorf("TempConversationID = %q, want %q", r.TempConversationID, "conv-1")
	}
	if r.Rating != 4 {
		t.Errorf("Rating = %d, want 4", r.Rating)
	}
	if r.Note != "good work" {
		t.Errorf("Note = %q, want %q", r.Note, "good work")
	}
	if r.CreatedAt <= 0 {
		t.Error("expected CreatedAt > 0")
	}
}

func TestInsertRatingWithTemp(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	r, err := InsertRatingWithTemp(ctx, db, "conv-1", "temp-1", 4, "good work", "")
	if err != nil {
		t.Fatalf("InsertRatingWithTemp: %v", err)
	}
	if r.TempConversationID != "temp-1" {
		t.Errorf("TempConversationID = %q, want %q", r.TempConversationID, "temp-1")
	}

	resolved, found, err := ResolveConversationIDByTempID(ctx, db, "temp-1")
	if err != nil {
		t.Fatalf("ResolveConversationIDByTempID: %v", err)
	}
	if !found {
		t.Fatal("expected temp ID to resolve")
	}
	if resolved != "conv-1" {
		t.Errorf("resolved conversation ID = %q, want %q", resolved, "conv-1")
	}
}

func TestListRatings(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := InsertRating(ctx, db, "conv", i, "", ""); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	ratings, err := ListRatings(ctx, db, 3)
	if err != nil {
		t.Fatalf("ListRatings: %v", err)
	}
	if len(ratings) != 3 {
		t.Fatalf("got %d ratings, want 3", len(ratings))
	}
}

func TestListRatingsEmpty(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	ratings, err := ListRatings(ctx, db, 50)
	if err != nil {
		t.Fatalf("ListRatings: %v", err)
	}
	if ratings == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(ratings) != 0 {
		t.Errorf("got %d ratings, want 0", len(ratings))
	}
}

func TestUpdateConversationID(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	r, err := InsertRating(ctx, db, "old-conv", 3, "", "")
	if err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	if err := UpdateConversationID(ctx, db, r.ID, "new-conv"); err != nil {
		t.Fatalf("UpdateConversationID: %v", err)
	}

	ratings, err := ListRatings(ctx, db, 1)
	if err != nil {
		t.Fatalf("ListRatings: %v", err)
	}
	if len(ratings) != 1 {
		t.Fatalf("got %d ratings, want 1", len(ratings))
	}
	if ratings[0].ConversationID != "new-conv" {
		t.Errorf("ConversationID = %q, want %q", ratings[0].ConversationID, "new-conv")
	}
}

func TestReconcileOrphanedRating(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	// Insert a rating with a conversation_id that has no matching conversation row (orphaned).
	orphanedConvID := "orphaned-conv-id"
	r, err := InsertRating(ctx, database, orphanedConvID, 4, "great work", "")
	if err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	// The history timestamp should be close to the rating's created_at.
	historyTs := r.CreatedAt
	realSessionID := "real-session-id"

	if err := ReconcileOrphanedRating(ctx, database, 4, "great work", historyTs, realSessionID); err != nil {
		t.Fatalf("ReconcileOrphanedRating: %v", err)
	}

	// Verify the rating was updated.
	ratings, err := ListRatings(ctx, database, 1)
	if err != nil {
		t.Fatalf("ListRatings: %v", err)
	}
	if len(ratings) != 1 {
		t.Fatalf("got %d ratings, want 1", len(ratings))
	}
	if ratings[0].ConversationID != realSessionID {
		t.Errorf("ConversationID = %q, want %q", ratings[0].ConversationID, realSessionID)
	}
}

func TestReconcileOrphanedRating_NoMatchWrongRating(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	orphanedConvID := "orphaned-conv-id"
	r, err := InsertRating(ctx, database, orphanedConvID, 4, "note", "")
	if err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	// Try to reconcile with a different rating value — should not match.
	if err := ReconcileOrphanedRating(ctx, database, 3, "note", r.CreatedAt, "real-session"); err != nil {
		t.Fatalf("ReconcileOrphanedRating: %v", err)
	}

	ratings, _ := ListRatings(ctx, database, 1)
	if ratings[0].ConversationID != orphanedConvID {
		t.Errorf("rating should not have been reconciled, got ConversationID = %q", ratings[0].ConversationID)
	}
}

func TestReconcileOrphanedRating_NoMatchWrongNote(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	orphanedConvID := "orphaned-conv-id"
	r, err := InsertRating(ctx, database, orphanedConvID, 4, "note A", "")
	if err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	// Try to reconcile with a different note — should not match.
	if err := ReconcileOrphanedRating(ctx, database, 4, "note B", r.CreatedAt, "real-session"); err != nil {
		t.Fatalf("ReconcileOrphanedRating: %v", err)
	}

	ratings, _ := ListRatings(ctx, database, 1)
	if ratings[0].ConversationID != orphanedConvID {
		t.Errorf("rating should not have been reconciled, got ConversationID = %q", ratings[0].ConversationID)
	}
}

func TestReconcileOrphanedRating_NoMatchTimestampTooFar(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	orphanedConvID := "orphaned-conv-id"
	_, err := InsertRating(ctx, database, orphanedConvID, 4, "note", "")
	if err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	// Timestamp 2 minutes away — outside the 60-second window.
	farTs := time.Now().Add(2 * time.Minute).UnixMilli()

	if err := ReconcileOrphanedRating(ctx, database, 4, "note", farTs, "real-session"); err != nil {
		t.Fatalf("ReconcileOrphanedRating: %v", err)
	}

	ratings, _ := ListRatings(ctx, database, 1)
	if ratings[0].ConversationID != orphanedConvID {
		t.Errorf("rating should not have been reconciled, got ConversationID = %q", ratings[0].ConversationID)
	}
}

func TestReconcileOrphanedRating_SkipsNonOrphaned(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	// Create a project and conversation so the rating is NOT orphaned.
	projectID, err := EnsureProject(ctx, database, "/some/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	convID := "linked-conv-id"
	if err := EnsureConversation(ctx, database, convID, projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	r, err := InsertRating(ctx, database, convID, 4, "note", "")
	if err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	// Try to reconcile — should not touch this rating because it's linked to a real conversation.
	if err := ReconcileOrphanedRating(ctx, database, 4, "note", r.CreatedAt, "other-session"); err != nil {
		t.Fatalf("ReconcileOrphanedRating: %v", err)
	}

	ratings, _ := ListRatings(ctx, database, 1)
	if ratings[0].ConversationID != convID {
		t.Errorf("non-orphaned rating should not have been reconciled, got ConversationID = %q", ratings[0].ConversationID)
	}
}
