package db

import (
	"context"
	"testing"
)

func TestInsertRating(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	r, err := InsertRating(ctx, db, "conv-1", 4, "good work")
	if err != nil {
		t.Fatalf("InsertRating: %v", err)
	}
	if r.ID == "" {
		t.Error("expected non-empty ID")
	}
	if r.ConversationID != "conv-1" {
		t.Errorf("ConversationID = %q, want %q", r.ConversationID, "conv-1")
	}
	if r.Rating != 4 {
		t.Errorf("Rating = %d, want 4", r.Rating)
	}
	if r.Note != "good work" {
		t.Errorf("Note = %q, want %q", r.Note, "good work")
	}
	if r.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestListRatings(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := InsertRating(ctx, db, "conv", i, ""); err != nil {
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

	r, err := InsertRating(ctx, db, "old-conv", 3, "")
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
