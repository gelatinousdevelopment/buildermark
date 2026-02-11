package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davidcann/zrate/web/server/internal/db"
)

func TestListProjects(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	for _, path := range []string{"/z/project", "/a/project"} {
		if _, err := db.EnsureProject(ctx, s.DB, path); err != nil {
			t.Fatalf("EnsureProject: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
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

	data, ok := env.Data.([]any)
	if !ok {
		t.Fatalf("data is not an array: %T", env.Data)
	}
	if len(data) != 2 {
		t.Errorf("got %d projects, want 2", len(data))
	}

	// Verify order by path.
	first := data[0].(map[string]any)
	if first["path"] != "/a/project" {
		t.Errorf("first project path = %v, want %q", first["path"], "/a/project")
	}
}

func TestListProjectsEmpty(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
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
		t.Errorf("got %d projects, want 0", len(data))
	}
}

func TestGetProject(t *testing.T) {
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
	if _, err := db.InsertRating(ctx, s.DB, "conv-1", 3, "ok"); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/projects/"+pid, nil)
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

	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	if data["path"] != "/test/project" {
		t.Errorf("path = %v, want %q", data["path"], "/test/project")
	}

	conversations, ok := data["conversations"].([]any)
	if !ok {
		t.Fatalf("conversations is not an array: %T", data["conversations"])
	}
	if len(conversations) != 1 {
		t.Fatalf("got %d conversations, want 1", len(conversations))
	}

	conv := conversations[0].(map[string]any)
	ratings, ok := conv["ratings"].([]any)
	if !ok {
		t.Fatalf("ratings is not an array: %T", conv["ratings"])
	}
	if len(ratings) != 1 {
		t.Errorf("got %d ratings, want 1", len(ratings))
	}
}

func TestGetProjectNotFound(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/v1/projects/nonexistent", nil)
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
