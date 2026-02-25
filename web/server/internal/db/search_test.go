package db

import (
	"context"
	"testing"
)

func TestSearchProjectMatches(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	p1, err := EnsureProject(ctx, database, "/tmp/repo-one")
	if err != nil {
		t.Fatalf("EnsureProject p1: %v", err)
	}
	p2, err := EnsureProject(ctx, database, "/tmp/repo-two")
	if err != nil {
		t.Fatalf("EnsureProject p2: %v", err)
	}

	if err := EnsureConversation(ctx, database, "conv-a", p1, "codex"); err != nil {
		t.Fatalf("EnsureConversation conv-a: %v", err)
	}
	if err := EnsureConversation(ctx, database, "conv-hidden", p1, "codex"); err != nil {
		t.Fatalf("EnsureConversation conv-hidden: %v", err)
	}
	if err := EnsureConversation(ctx, database, "conv-b", p2, "claude"); err != nil {
		t.Fatalf("EnsureConversation conv-b: %v", err)
	}
	if err := SetConversationHidden(ctx, database, "conv-hidden", true); err != nil {
		t.Fatalf("SetConversationHidden: %v", err)
	}

	if err := InsertMessages(ctx, database, []Message{
		{Timestamp: 1000, ProjectID: p1, ConversationID: "conv-a", Role: "user", Content: "please deploy this change", RawJSON: "{}"},
		{Timestamp: 1001, ProjectID: p1, ConversationID: "conv-hidden", Role: "user", Content: "deploy hidden", RawJSON: "{}"},
		{Timestamp: 1002, ProjectID: p2, ConversationID: "conv-b", Role: "user", Content: "search does not match", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	if err := UpsertCommit(ctx, database, Commit{ProjectID: p1, BranchName: "main", CommitHash: "abc123", Subject: "deploy pipeline", DiffContent: ""}); err != nil {
		t.Fatalf("UpsertCommit p1: %v", err)
	}
	if err := UpsertCommit(ctx, database, Commit{ProjectID: p2, BranchName: "main", CommitHash: "def456", Subject: "refactor parser", DiffContent: ""}); err != nil {
		t.Fatalf("UpsertCommit p2: %v", err)
	}

	results, err := SearchProjectMatches(ctx, database, "deploy", "")
	if err != nil {
		t.Fatalf("SearchProjectMatches: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	if results[0].Project.ID != p1 {
		t.Fatalf("project id = %s, want %s", results[0].Project.ID, p1)
	}
	if results[0].ConversationMatches != 1 {
		t.Fatalf("conversation matches = %d, want 1", results[0].ConversationMatches)
	}
	if results[0].CommitMatches != 1 {
		t.Fatalf("commit matches = %d, want 1", results[0].CommitMatches)
	}
}

func TestFilterCommitHashesBySearch(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, database, "/tmp/repo-filter")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := UpsertCommit(ctx, database, Commit{ProjectID: projectID, BranchName: "main", CommitHash: "ab1000", Subject: "docs", DiffContent: "+needle"}); err != nil {
		t.Fatalf("UpsertCommit one: %v", err)
	}
	if err := UpsertCommit(ctx, database, Commit{ProjectID: projectID, BranchName: "main", CommitHash: "cd2000", Subject: "feature", DiffContent: "+other"}); err != nil {
		t.Fatalf("UpsertCommit two: %v", err)
	}

	hashes := []string{"ab1000", "cd2000"}
	filtered, err := FilterCommitHashesBySearch(ctx, database, projectID, hashes, "needle")
	if err != nil {
		t.Fatalf("FilterCommitHashesBySearch needle: %v", err)
	}
	if len(filtered) != 1 || filtered[0] != "ab1000" {
		t.Fatalf("filtered needle = %v, want [ab1000]", filtered)
	}

	filtered, err = FilterCommitHashesBySearch(ctx, database, projectID, hashes, "ab")
	if err != nil {
		t.Fatalf("FilterCommitHashesBySearch short: %v", err)
	}
	if len(filtered) != 1 || filtered[0] != "ab1000" {
		t.Fatalf("filtered short = %v, want [ab1000]", filtered)
	}
}
