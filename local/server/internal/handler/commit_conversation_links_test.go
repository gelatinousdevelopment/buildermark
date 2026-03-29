package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func TestCommitConversationLinksPostAllowedInReadOnlyMode(t *testing.T) {
	s := setupTestServer(t)
	s.ReadOnly = true
	handler := s.Routes()
	ctx := context.Background()

	projectID, err := db.EnsureProject(ctx, s.DB, "/missing/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-1", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	if err := db.UpsertCommit(ctx, s.DB, db.Commit{
		ProjectID:  projectID,
		BranchName: "main",
		CommitHash: "hash-1",
		Subject:    "subject",
		AuthoredAt: 1700000000,
	}); err != nil {
		t.Fatalf("UpsertCommit: %v", err)
	}
	commit, err := db.GetCommitByProjectAndHash(ctx, s.DB, projectID, "hash-1")
	if err != nil {
		t.Fatalf("GetCommitByProjectAndHash: %v", err)
	}
	if commit == nil {
		t.Fatal("commit = nil, want stored commit")
	}
	if err := db.UpsertCommitConversationLinks(ctx, s.DB, commit.ID, []string{"conv-1"}); err != nil {
		t.Fatalf("UpsertCommitConversationLinks: %v", err)
	}

	body, err := json.Marshal(map[string]any{
		"commitHashes":    []string{"hash-1"},
		"conversationIds": []string{"conv-1"},
	})
	if err != nil {
		t.Fatalf("Marshal request body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/commit-conversation-links", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok=false, error=%v", env.Error)
	}

	data := env.Data.(map[string]any)
	commitToConversations := data["commitToConversations"].(map[string]any)
	if got := len(commitToConversations["hash-1"].([]any)); got != 1 {
		t.Fatalf("commitToConversations[hash-1] len = %d, want 1", got)
	}
	conversationToCommits := data["conversationToCommits"].(map[string]any)
	if got := len(conversationToCommits["conv-1"].([]any)); got != 1 {
		t.Fatalf("conversationToCommits[conv-1] len = %d, want 1", got)
	}
}

func TestCommitConversationLinksConversationOnlyFallsBackToSingleProjectGroup(t *testing.T) {
	s := setupTestServer(t)
	s.ReadOnly = true
	handler := s.Routes()
	ctx := context.Background()

	projectID, err := db.EnsureProject(ctx, s.DB, "/missing/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-1", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	if err := db.UpsertCommit(ctx, s.DB, db.Commit{
		ProjectID:  projectID,
		BranchName: "main",
		CommitHash: "hash-1",
		Subject:    "subject",
		AuthoredAt: 1700000000,
	}); err != nil {
		t.Fatalf("UpsertCommit: %v", err)
	}
	commit, err := db.GetCommitByProjectAndHash(ctx, s.DB, projectID, "hash-1")
	if err != nil {
		t.Fatalf("GetCommitByProjectAndHash: %v", err)
	}
	if commit == nil {
		t.Fatal("commit = nil, want stored commit")
	}
	if err := db.UpsertCommitConversationLinks(ctx, s.DB, commit.ID, []string{"conv-1"}); err != nil {
		t.Fatalf("UpsertCommitConversationLinks: %v", err)
	}

	body, err := json.Marshal(map[string]any{
		"commitHashes":    []string{},
		"conversationIds": []string{"conv-1"},
	})
	if err != nil {
		t.Fatalf("Marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/commit-conversation-links", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok=false, error=%v", env.Error)
	}

	data := env.Data.(map[string]any)
	conversationToCommits := data["conversationToCommits"].(map[string]any)
	if got := len(conversationToCommits["conv-1"].([]any)); got != 1 {
		t.Fatalf("conversationToCommits[conv-1] len = %d, want 1", got)
	}
	commitBranches := data["commitBranches"].(map[string]any)
	if got := commitBranches["hash-1"].(string); got != "main" {
		t.Fatalf("commitBranches[hash-1] = %q, want %q", got, "main")
	}
}
