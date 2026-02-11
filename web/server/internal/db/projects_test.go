package db

import (
	"context"
	"testing"
)

func TestListProjects(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create projects in non-alphabetical order.
	for _, path := range []string{"/z/project", "/a/project", "/m/project"} {
		if _, err := EnsureProject(ctx, db, path); err != nil {
			t.Fatalf("EnsureProject %s: %v", path, err)
		}
	}

	projects, err := ListProjects(ctx, db)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 3 {
		t.Fatalf("got %d projects, want 3", len(projects))
	}

	// Should be ordered by path.
	if projects[0].Path != "/a/project" {
		t.Errorf("first project path = %q, want %q", projects[0].Path, "/a/project")
	}
	if projects[1].Path != "/m/project" {
		t.Errorf("second project path = %q, want %q", projects[1].Path, "/m/project")
	}
	if projects[2].Path != "/z/project" {
		t.Errorf("third project path = %q, want %q", projects[2].Path, "/z/project")
	}
}

func TestListProjectsEmpty(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projects, err := ListProjects(ctx, db)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if projects == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want 0", len(projects))
	}
}

func TestGetProjectDetail(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	// Add two conversations.
	if err := EnsureConversation(ctx, db, "conv-1", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation conv-1: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-2", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation conv-2: %v", err)
	}

	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected non-nil project detail")
	}
	if detail.Path != "/test/project" {
		t.Errorf("Path = %q, want %q", detail.Path, "/test/project")
	}
	if len(detail.Conversations) != 2 {
		t.Fatalf("got %d conversations, want 2", len(detail.Conversations))
	}
}

func TestGetProjectDetailNotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	detail, err := GetProjectDetail(ctx, db, "nonexistent")
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail != nil {
		t.Errorf("expected nil for nonexistent project, got %+v", detail)
	}
}

func TestGetProjectDetailWithRatings(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Add ratings for this conversation.
	if _, err := InsertRating(ctx, db, "conv-1", 4, "good"); err != nil {
		t.Fatalf("InsertRating 1: %v", err)
	}
	if _, err := InsertRating(ctx, db, "conv-1", 5, "great"); err != nil {
		t.Fatalf("InsertRating 2: %v", err)
	}

	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected non-nil project detail")
	}

	if len(detail.Conversations) != 1 {
		t.Fatalf("got %d conversations, want 1", len(detail.Conversations))
	}

	conv := detail.Conversations[0]
	if len(conv.Ratings) != 2 {
		t.Fatalf("got %d ratings, want 2", len(conv.Ratings))
	}

	// Ratings should be ordered by created_at DESC (newest first).
	if conv.Ratings[0].Note != "great" {
		t.Errorf("first rating note = %q, want %q", conv.Ratings[0].Note, "great")
	}
}

func TestGetProjectDetailNoConversations(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/empty/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected non-nil project detail")
	}
	if detail.Conversations == nil {
		t.Error("expected non-nil empty Conversations slice")
	}
	if len(detail.Conversations) != 0 {
		t.Errorf("expected 0 conversations, got %d", len(detail.Conversations))
	}
}
