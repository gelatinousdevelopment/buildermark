package db

import (
	"context"
	"testing"
)

func TestUpsertCommit_ConflictOnProjectAndHash(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, database, "/tmp/buildermark-test-repo")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	commitHash := "bfe0c3d156588b5b804666d4ebe07e03c1e78f96"

	if err := UpsertCommit(ctx, database, Commit{
		ProjectID:   projectID,
		BranchName:  "main",
		CommitHash:  commitHash,
		Subject:     "first subject",
		UserName:    "Test User",
		UserEmail:   "test@example.com",
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
		UserName:    "Test User",
		UserEmail:   "test@example.com",
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

func TestListDistinctUsers(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, database, "/tmp/buildermark-test-users")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	// Insert commits from two different users.
	for _, c := range []Commit{
		{ProjectID: projectID, BranchName: "main", CommitHash: "aaa", Subject: "a", UserName: "Alice", UserEmail: "alice@example.com", AuthoredAt: 1700000000},
		{ProjectID: projectID, BranchName: "main", CommitHash: "bbb", Subject: "b", UserName: "Bob", UserEmail: "bob@example.com", AuthoredAt: 1700000001},
		{ProjectID: projectID, BranchName: "main", CommitHash: "ccc", Subject: "c", UserName: "Alice", UserEmail: "alice@example.com", AuthoredAt: 1700000002},
	} {
		if err := UpsertCommit(ctx, database, c); err != nil {
			t.Fatalf("UpsertCommit %s: %v", c.CommitHash, err)
		}
	}

	users, err := ListDistinctUsers(ctx, database, projectID, "main")
	if err != nil {
		t.Fatalf("ListDistinctUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("got %d users, want 2", len(users))
	}
	// Ordered by user_name: Alice, Bob.
	if users[0].Name != "Alice" || users[0].Email != "alice@example.com" {
		t.Errorf("users[0] = %+v, want Alice", users[0])
	}
	if users[1].Name != "Bob" || users[1].Email != "bob@example.com" {
		t.Errorf("users[1] = %+v, want Bob", users[1])
	}
}

func TestListAndCountCommitsByProjectAndUser(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, database, "/tmp/buildermark-test-user-filter")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	for _, c := range []Commit{
		{ProjectID: projectID, BranchName: "main", CommitHash: "aaa", Subject: "a", UserName: "Alice", UserEmail: "alice@example.com", AuthoredAt: 1700000000},
		{ProjectID: projectID, BranchName: "main", CommitHash: "bbb", Subject: "b", UserName: "Bob", UserEmail: "bob@example.com", AuthoredAt: 1700000001},
		{ProjectID: projectID, BranchName: "main", CommitHash: "ccc", Subject: "c", UserName: "Alice", UserEmail: "alice@example.com", AuthoredAt: 1700000002},
	} {
		if err := UpsertCommit(ctx, database, c); err != nil {
			t.Fatalf("UpsertCommit %s: %v", c.CommitHash, err)
		}
	}

	// Filter by Alice.
	commits, err := ListCommitsByProjectAndUser(ctx, database, projectID, "main", "alice@example.com", 20, 0)
	if err != nil {
		t.Fatalf("ListCommitsByProjectAndUser alice: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits for alice, want 2", len(commits))
	}

	count, err := CountCommitsByProjectAndUser(ctx, database, projectID, "main", "alice@example.com")
	if err != nil {
		t.Fatalf("CountCommitsByProjectAndUser alice: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	// Empty user returns all (backward compat).
	allCommits, err := ListCommitsByProjectAndUser(ctx, database, projectID, "main", "", 20, 0)
	if err != nil {
		t.Fatalf("ListCommitsByProjectAndUser empty: %v", err)
	}
	if len(allCommits) != 3 {
		t.Fatalf("got %d commits for empty user, want 3", len(allCommits))
	}

	allCount, err := CountCommitsByProjectAndUser(ctx, database, projectID, "main", "")
	if err != nil {
		t.Fatalf("CountCommitsByProjectAndUser empty: %v", err)
	}
	if allCount != 3 {
		t.Fatalf("allCount = %d, want 3", allCount)
	}
}

func TestHasStaleCommitCoverage(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, database, "/tmp/buildermark-test-repo-stale")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	if err := UpsertCommit(ctx, database, Commit{
		ProjectID:       projectID,
		BranchName:      "main",
		CommitHash:      "hash-fresh",
		Subject:         "fresh",
		UserName:        "Test User",
		UserEmail:       "test@example.com",
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
		UserName:        "Test User",
		UserEmail:       "test@example.com",
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
