package db

import (
	"context"
	"errors"
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

	projects, err := ListProjects(ctx, db, false)
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

	projects, err := ListProjects(ctx, db, false)
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

func TestListProjectsFiltersByIgnored(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid1, err := EnsureProject(ctx, db, "/active/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	pid2, err := EnsureProject(ctx, db, "/ignored/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	// Ignore the second project.
	if err := SetProjectIgnored(ctx, db, pid2, true); err != nil {
		t.Fatalf("SetProjectIgnored: %v", err)
	}

	// Non-ignored should return only the active project.
	active, err := ListProjects(ctx, db, false)
	if err != nil {
		t.Fatalf("ListProjects(false): %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("got %d non-ignored projects, want 1", len(active))
	}
	if active[0].ID != pid1 {
		t.Errorf("active project id = %q, want %q", active[0].ID, pid1)
	}

	// Ignored should return only the ignored project.
	ignored, err := ListProjects(ctx, db, true)
	if err != nil {
		t.Fatalf("ListProjects(true): %v", err)
	}
	if len(ignored) != 1 {
		t.Fatalf("got %d ignored projects, want 1", len(ignored))
	}
	if ignored[0].ID != pid2 {
		t.Errorf("ignored project id = %q, want %q", ignored[0].ID, pid2)
	}
	if !ignored[0].Ignored {
		t.Error("expected Ignored = true")
	}
}

func TestSetProjectIgnored(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	// Ignore the project.
	if err := SetProjectIgnored(ctx, db, pid, true); err != nil {
		t.Fatalf("SetProjectIgnored(true): %v", err)
	}

	// Verify it's ignored.
	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if !detail.Ignored {
		t.Error("expected Ignored = true after setting")
	}

	// Un-ignore the project.
	if err := SetProjectIgnored(ctx, db, pid, false); err != nil {
		t.Fatalf("SetProjectIgnored(false): %v", err)
	}

	detail, err = GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail.Ignored {
		t.Error("expected Ignored = false after unsetting")
	}
}

func TestSetProjectIgnoredNotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	err := SetProjectIgnored(ctx, db, "nonexistent", true)
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
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
	if _, err := InsertRating(ctx, db, "conv-1", 4, "good", ""); err != nil {
		t.Fatalf("InsertRating 1: %v", err)
	}
	if _, err := InsertRating(ctx, db, "conv-1", 5, "great", ""); err != nil {
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

func TestListProjectsReturnsLabel(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	if _, err := EnsureProject(ctx, db, "/home/user/myproject"); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	projects, err := ListProjects(ctx, db, false)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	if projects[0].Label != "myproject" {
		t.Errorf("label = %q, want %q", projects[0].Label, "myproject")
	}
}

func TestSetProjectLabel(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	if err := SetProjectLabel(ctx, db, pid, "My Project"); err != nil {
		t.Fatalf("SetProjectLabel: %v", err)
	}

	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail.Label != "My Project" {
		t.Errorf("label = %q, want %q", detail.Label, "My Project")
	}
}

func TestSetProjectLabelNotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	err := SetProjectLabel(ctx, db, "nonexistent", "label")
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateProjectGitID(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	if err := UpdateProjectGitID(ctx, db, pid, "abc123"); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}

	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail.GitID != "abc123" {
		t.Errorf("gitId = %q, want %q", detail.GitID, "abc123")
	}
}

func TestListProjectsWithoutGitID(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid1, err := EnsureProject(ctx, db, "/project/a")
	if err != nil {
		t.Fatalf("EnsureProject a: %v", err)
	}
	if _, err := EnsureProject(ctx, db, "/project/b"); err != nil {
		t.Fatalf("EnsureProject b: %v", err)
	}

	// Set git_id on the first project only.
	if err := UpdateProjectGitID(ctx, db, pid1, "abc123"); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}

	projects, err := ListProjectsWithoutGitID(ctx, db)
	if err != nil {
		t.Fatalf("ListProjectsWithoutGitID: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	if projects[0].Path != "/project/b" {
		t.Errorf("path = %q, want %q", projects[0].Path, "/project/b")
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
