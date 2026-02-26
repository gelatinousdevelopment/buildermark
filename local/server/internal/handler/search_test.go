package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gelatinousdevelopment/buildermark/web/server/internal/db"
)

func TestSearchProjects(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	projectID, err := db.EnsureProject(ctx, s.DB, "/tmp/search-project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-1", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	if err := db.InsertMessages(ctx, s.DB, []db.Message{{
		Timestamp:      1000,
		ProjectID:      projectID,
		ConversationID: "conv-1",
		Role:           "user",
		Content:        "searchable user prompt",
		RawJSON:        "{}",
	}}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}
	if err := db.UpsertCommit(ctx, s.DB, db.Commit{ProjectID: projectID, BranchName: "main", CommitHash: "111aaa", Subject: "searchable commit", DiffContent: ""}); err != nil {
		t.Fatalf("UpsertCommit: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/search/projects?q=searchable", nil)
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
		t.Fatalf("expected ok=true, got false with error %q", env.Error)
	}

	data, ok := env.Data.([]any)
	if !ok {
		t.Fatalf("data is not array: %T", env.Data)
	}
	if len(data) != 1 {
		t.Fatalf("result count = %d, want 1", len(data))
	}
	row := data[0].(map[string]any)
	project := row["project"].(map[string]any)
	if project["id"] != projectID {
		t.Fatalf("project id = %v, want %s", project["id"], projectID)
	}
	if int(row["conversationMatches"].(float64)) != 1 {
		t.Fatalf("conversationMatches = %v, want 1", row["conversationMatches"])
	}
	if int(row["commitMatches"].(float64)) != 1 {
		t.Fatalf("commitMatches = %v, want 1", row["commitMatches"])
	}
}
