package db

import (
	"context"
	"testing"
)

func TestUpsertCommit_ConflictOnProjectAndHash(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, database, "/tmp/zrate-test-repo")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	commitHash := "bfe0c3d156588b5b804666d4ebe07e03c1e78f96"

	if err := UpsertCommit(ctx, database, Commit{
		ProjectID:   projectID,
		BranchName:  "main",
		CommitHash:  commitHash,
		Subject:     "first subject",
		AuthorName:  "Test User",
		AuthorEmail: "test@example.com",
		AuthoredAt:  1700000000,
		DiffContent: "diff --git a/a b/a",
	}); err != nil {
		t.Fatalf("first UpsertCommit: %v", err)
	}

	if err := UpsertCommit(ctx, database, Commit{
		ProjectID:   projectID,
		BranchName:  "feature",
		CommitHash:  commitHash,
		Subject:     "updated subject",
		AuthorName:  "Test User",
		AuthorEmail: "test@example.com",
		AuthoredAt:  1700000001,
		DiffContent: "diff --git a/a b/a\n+line",
	}); err != nil {
		t.Fatalf("second UpsertCommit: %v", err)
	}

	var count int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM commits WHERE project_id = ? AND commit_hash = ?", projectID, commitHash).Scan(&count); err != nil {
		t.Fatalf("count commits: %v", err)
	}
	if count != 1 {
		t.Fatalf("commit row count = %d, want 1", count)
	}

	var branchName, subject string
	if err := database.QueryRowContext(ctx, "SELECT branch_name, subject FROM commits WHERE project_id = ? AND commit_hash = ?", projectID, commitHash).Scan(&branchName, &subject); err != nil {
		t.Fatalf("query updated commit: %v", err)
	}
	if branchName != "feature" {
		t.Fatalf("branch_name = %q, want %q", branchName, "feature")
	}
	if subject != "updated subject" {
		t.Fatalf("subject = %q, want %q", subject, "updated subject")
	}
}

func TestHasStaleCommitCoverage(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, database, "/tmp/zrate-test-repo-stale")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	if err := UpsertCommit(ctx, database, Commit{
		ProjectID:       projectID,
		BranchName:      "main",
		CommitHash:      "hash-fresh",
		Subject:         "fresh",
		AuthorName:      "Test User",
		AuthorEmail:     "test@example.com",
		AuthoredAt:      1700000000,
		DiffContent:     "diff --git a/a b/a",
		CoverageVersion: 1,
	}); err != nil {
		t.Fatalf("upsert fresh commit: %v", err)
	}

	stale, err := HasStaleCommitCoverage(ctx, database, projectID, "main", 1)
	if err != nil {
		t.Fatalf("HasStaleCommitCoverage fresh: %v", err)
	}
	if stale {
		t.Fatalf("stale = true, want false")
	}

	if err := UpsertCommit(ctx, database, Commit{
		ProjectID:       projectID,
		BranchName:      "main",
		CommitHash:      "hash-stale",
		Subject:         "stale",
		AuthorName:      "Test User",
		AuthorEmail:     "test@example.com",
		AuthoredAt:      1700000001,
		DiffContent:     "diff --git a/b b/b",
		CoverageVersion: 0,
	}); err != nil {
		t.Fatalf("upsert stale commit: %v", err)
	}

	stale, err = HasStaleCommitCoverage(ctx, database, projectID, "main", 1)
	if err != nil {
		t.Fatalf("HasStaleCommitCoverage stale: %v", err)
	}
	if !stale {
		t.Fatalf("stale = false, want true")
	}
}
