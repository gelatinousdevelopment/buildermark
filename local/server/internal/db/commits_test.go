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

func TestGetCachedConversationCommitLinks(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, database, "/tmp/buildermark-test-conv-links")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	if err := EnsureConversation(ctx, database, "conv-1", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation conv-1: %v", err)
	}
	if err := EnsureConversation(ctx, database, "conv-2", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation conv-2: %v", err)
	}

	for _, c := range []Commit{
		{ProjectID: projectID, BranchName: "main", CommitHash: "hash-a", Subject: "a", UserName: "Alice", UserEmail: "alice@example.com", AuthoredAt: 1700000000},
		{ProjectID: projectID, BranchName: "feature/demo", CommitHash: "hash-b", Subject: "b", UserName: "Alice", UserEmail: "alice@example.com", AuthoredAt: 1700000001},
	} {
		if err := UpsertCommit(ctx, database, c); err != nil {
			t.Fatalf("UpsertCommit %s: %v", c.CommitHash, err)
		}
	}

	var commitAID, commitBID string
	if err := database.QueryRowContext(ctx, "SELECT id FROM commits WHERE project_id = ? AND commit_hash = ?", projectID, "hash-a").Scan(&commitAID); err != nil {
		t.Fatalf("query commitA ID: %v", err)
	}
	if err := database.QueryRowContext(ctx, "SELECT id FROM commits WHERE project_id = ? AND commit_hash = ?", projectID, "hash-b").Scan(&commitBID); err != nil {
		t.Fatalf("query commitB ID: %v", err)
	}

	if err := UpsertCommitConversationLinks(ctx, database, commitAID, []string{"conv-1"}); err != nil {
		t.Fatalf("UpsertCommitConversationLinks commitA: %v", err)
	}
	if err := UpsertCommitConversationLinks(ctx, database, commitBID, []string{"conv-1", "conv-2"}); err != nil {
		t.Fatalf("UpsertCommitConversationLinks commitB: %v", err)
	}

	conversationToCommits, commitBranches, commitSubjects, err := GetCachedConversationCommitLinks(ctx, database, []string{projectID}, []string{"conv-1", "conv-2"})
	if err != nil {
		t.Fatalf("GetCachedConversationCommitLinks: %v", err)
	}

	conv1 := conversationToCommits["conv-1"]
	if len(conv1) != 2 || conv1[0] != "hash-b" || conv1[1] != "hash-a" {
		t.Fatalf("conversationToCommits[conv-1] = %#v, want [hash-b hash-a]", conv1)
	}
	conv2 := conversationToCommits["conv-2"]
	if len(conv2) != 1 || conv2[0] != "hash-b" {
		t.Fatalf("conversationToCommits[conv-2] = %#v, want [hash-b]", conv2)
	}
	if got := commitBranches["hash-a"]; got != "main" {
		t.Fatalf("commitBranches[hash-a] = %q, want %q", got, "main")
	}
	if got := commitBranches["hash-b"]; got != "feature/demo" {
		t.Fatalf("commitBranches[hash-b] = %q, want %q", got, "feature/demo")
	}
	if got := commitSubjects["hash-a"]; got != "a" {
		t.Fatalf("commitSubjects[hash-a] = %q, want %q", got, "a")
	}
	if got := commitSubjects["hash-b"]; got != "b" {
		t.Fatalf("commitSubjects[hash-b] = %q, want %q", got, "b")
	}
}
