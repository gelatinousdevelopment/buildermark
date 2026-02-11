package handler

import (
	"bytes"
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
	if _, err := db.InsertRating(ctx, s.DB, "conv-1", 3, "ok", ""); err != nil {
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

func TestListProjectsIgnoredFilter(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid1, err := db.EnsureProject(ctx, s.DB, "/active/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	_, err = db.EnsureProject(ctx, s.DB, "/ignored/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	// Ignore the second project directly via DB.
	if err := db.SetProjectIgnored(ctx, s.DB, pid1, false); err != nil {
		t.Fatalf("SetProjectIgnored: %v", err)
	}

	// Default (no param) returns non-ignored projects.
	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data := env.Data.([]any)
	if len(data) != 2 {
		t.Errorf("got %d non-ignored projects, want 2", len(data))
	}

	// Now ignore pid1.
	if err := db.SetProjectIgnored(ctx, s.DB, pid1, true); err != nil {
		t.Fatalf("SetProjectIgnored: %v", err)
	}

	// ?ignored=true returns only ignored projects.
	req = httptest.NewRequest("GET", "/api/v1/projects?ignored=true", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	env = jsonEnvelope{}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data = env.Data.([]any)
	if len(data) != 1 {
		t.Fatalf("got %d ignored projects, want 1", len(data))
	}
	first := data[0].(map[string]any)
	if first["path"] != "/active/project" {
		t.Errorf("ignored project path = %v, want %q", first["path"], "/active/project")
	}
}

func TestSetProjectIgnored(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	// Set ignored = true via POST.
	body, _ := json.Marshal(map[string]bool{"ignored": true})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+pid+"/ignored", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.OK {
		t.Error("ok = false, want true")
	}

	// Verify it's now ignored via list.
	ignored, err := db.ListProjects(ctx, s.DB, true)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(ignored) != 1 {
		t.Fatalf("got %d ignored projects, want 1", len(ignored))
	}

	// Set ignored = false via POST.
	body, _ = json.Marshal(map[string]bool{"ignored": false})
	req = httptest.NewRequest("POST", "/api/v1/projects/"+pid+"/ignored", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	active, err := db.ListProjects(ctx, s.DB, false)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("got %d active projects, want 1", len(active))
	}
}

func TestSetProjectIgnoredInvalidBody(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/v1/projects/"+pid+"/ignored", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
