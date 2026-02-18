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

func TestGetProjectSupportsConversationPagination(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	for _, id := range []string{"conv-1", "conv-2", "conv-3"} {
		if err := db.EnsureConversation(ctx, s.DB, id, pid, "codex"); err != nil {
			t.Fatalf("EnsureConversation %s: %v", id, err)
		}
	}
	if err := db.InsertMessages(ctx, s.DB, []db.Message{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "conv-1", Role: "user", Content: "one", RawJSON: "{}"},
		{Timestamp: 3000, ProjectID: pid, ConversationID: "conv-3", Role: "user", Content: "three", RawJSON: "{}"},
		{Timestamp: 2000, ProjectID: pid, ConversationID: "conv-2", Role: "user", Content: "two", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/projects/"+pid+"?page=1&pageSize=2", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}

	pagination, ok := data["conversationPagination"].(map[string]any)
	if !ok {
		t.Fatalf("conversationPagination is not an object: %T", data["conversationPagination"])
	}
	if got := int(pagination["page"].(float64)); got != 1 {
		t.Fatalf("page = %d, want 1", got)
	}
	if got := int(pagination["pageSize"].(float64)); got != 2 {
		t.Fatalf("pageSize = %d, want 2", got)
	}
	if got := int(pagination["total"].(float64)); got != 3 {
		t.Fatalf("total = %d, want 3", got)
	}

	conversations, ok := data["conversations"].([]any)
	if !ok {
		t.Fatalf("conversations is not an array: %T", data["conversations"])
	}
	if len(conversations) != 2 {
		t.Fatalf("got %d conversations, want 2", len(conversations))
	}
	first := conversations[0].(map[string]any)
	second := conversations[1].(map[string]any)
	if first["id"] != "conv-3" {
		t.Fatalf("first conversation id = %v, want %q", first["id"], "conv-3")
	}
	if second["id"] != "conv-2" {
		t.Fatalf("second conversation id = %v, want %q", second["id"], "conv-2")
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

func TestSetProjectIgnoredNotFound(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	body, _ := json.Marshal(map[string]bool{"ignored": true})
	req := httptest.NewRequest("POST", "/api/v1/projects/nonexistent/ignored", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.OK {
		t.Error("ok = true, want false for not found project")
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

func TestSetProjectLabel(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"label": "My Project"})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+pid+"/label", bytes.NewReader(body))
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

	// Verify the label was set.
	detail, err := db.GetProjectDetail(ctx, s.DB, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail.Label != "My Project" {
		t.Errorf("label = %q, want %q", detail.Label, "My Project")
	}
}

func TestSetProjectLabelNotFound(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	body, _ := json.Marshal(map[string]string{"label": "Test"})
	req := httptest.NewRequest("POST", "/api/v1/projects/nonexistent/label", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestSetProjectLabelEmpty(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"label": ""})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+pid+"/label", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListProjectsHasLabel(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	if _, err := db.EnsureProject(ctx, s.DB, "/home/user/myproject"); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}

	data := env.Data.([]any)
	first := data[0].(map[string]any)
	if first["label"] != "myproject" {
		t.Errorf("label = %v, want %q", first["label"], "myproject")
	}
}

func TestGetProjectHasLabel(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/home/user/myproject")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/projects/"+pid, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}

	data := env.Data.(map[string]any)
	if data["label"] != "myproject" {
		t.Errorf("label = %v, want %q", data["label"], "myproject")
	}
}

func TestSetProjectIgnoreDiffPaths(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	paths := "TODO.md\nAGENTS.md\n**/*.generated.go"
	body, _ := json.Marshal(map[string]string{"ignoreDiffPaths": paths})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+pid+"/ignore-diff-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	detail, err := db.GetProjectDetail(ctx, s.DB, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail.IgnoreDiffPaths != paths {
		t.Errorf("ignoreDiffPaths = %q, want %q", detail.IgnoreDiffPaths, paths)
	}
}

func TestSetProjectIgnoreDiffPathsNotFound(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	body, _ := json.Marshal(map[string]string{"ignoreDiffPaths": "TODO.md"})
	req := httptest.NewRequest("POST", "/api/v1/projects/nonexistent/ignore-diff-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
