package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davidcann/zrate/web/server/internal/db"
)

func TestListConversations(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	// Seed data: project + conversations.
	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	for _, id := range []string{"conv-a", "conv-b"} {
		if err := db.EnsureConversation(ctx, s.DB, id, pid, "claude"); err != nil {
			t.Fatalf("EnsureConversation: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "/api/v1/conversations", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Errorf("ok = false, want true")
	}

	data, ok := env.Data.([]any)
	if !ok {
		t.Fatalf("data is not an array: %T", env.Data)
	}
	if len(data) != 2 {
		t.Errorf("got %d conversations, want 2", len(data))
	}
}

func TestListConversationsEmpty(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/v1/conversations", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	data, ok := env.Data.([]any)
	if !ok {
		t.Fatalf("data is not an array: %T", env.Data)
	}
	if len(data) != 0 {
		t.Errorf("got %d conversations, want 0", len(data))
	}
}

func TestGetConversation(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-1", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Add turns and a rating.
	turns := []db.Turn{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "conv-1", Role: "user", Content: "hello"},
		{Timestamp: 2000, ProjectID: pid, ConversationID: "conv-1", Role: "agent", Content: "hi"},
	}
	if err := db.InsertTurns(ctx, s.DB, turns); err != nil {
		t.Fatalf("InsertTurns: %v", err)
	}
	if _, err := db.InsertRating(ctx, s.DB, "conv-1", 4, "nice"); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/conversations/conv-1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Error("ok = false, want true")
	}

	// Verify the response has the expected structure.
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	if data["id"] != "conv-1" {
		t.Errorf("id = %v, want %q", data["id"], "conv-1")
	}

	turnsData, ok := data["turns"].([]any)
	if !ok {
		t.Fatalf("turns is not an array: %T", data["turns"])
	}
	if len(turnsData) != 2 {
		t.Errorf("got %d turns, want 2", len(turnsData))
	}

	ratingsData, ok := data["ratings"].([]any)
	if !ok {
		t.Fatalf("ratings is not an array: %T", data["ratings"])
	}
	if len(ratingsData) != 1 {
		t.Errorf("got %d ratings, want 1", len(ratingsData))
	}
}

func TestGetConversationNotFound(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/v1/conversations/nonexistent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if env.OK {
		t.Error("ok = true, want false")
	}
}
