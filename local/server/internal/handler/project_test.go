package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
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
	if data["oldPaths"] != "" {
		t.Errorf("oldPaths = %v, want empty", data["oldPaths"])
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

func TestGetProjectPaginationByFamilyIncludesParentAndChildren(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	for _, id := range []string{"parent-a", "child-a", "parent-b"} {
		if err := db.EnsureConversation(ctx, s.DB, id, pid, "codex"); err != nil {
			t.Fatalf("EnsureConversation %s: %v", id, err)
		}
	}
	if err := db.UpdateConversationParent(ctx, s.DB, "child-a", "parent-a"); err != nil {
		t.Fatalf("UpdateConversationParent: %v", err)
	}
	if err := db.InsertMessages(ctx, s.DB, []db.Message{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "parent-a", Role: "user", Content: "one", RawJSON: "{}"},
		{Timestamp: 4000, ProjectID: pid, ConversationID: "child-a", Role: "user", Content: "two", RawJSON: "{}"},
		{Timestamp: 2000, ProjectID: pid, ConversationID: "parent-b", Role: "user", Content: "three", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/projects/"+pid+"?page=1&pageSize=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data := env.Data.(map[string]any)
	pagination := data["conversationPagination"].(map[string]any)
	if got := int(pagination["total"].(float64)); got != 2 {
		t.Fatalf("total = %d, want 2 families", got)
	}
	conversations := data["conversations"].([]any)
	if len(conversations) != 2 {
		t.Fatalf("rows = %d, want 2 (parent+child family)", len(conversations))
	}
	first := conversations[0].(map[string]any)
	second := conversations[1].(map[string]any)
	if first["id"] != "parent-a" || second["id"] != "child-a" {
		t.Fatalf("conversation order = [%v, %v], want [parent-a, child-a]", first["id"], second["id"])
	}
}

func TestGetProjectPaginationByFamilyIncludesGrandchildren(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	for _, id := range []string{"root-a", "child-a", "grandchild-a", "root-b"} {
		if err := db.EnsureConversation(ctx, s.DB, id, pid, "codex"); err != nil {
			t.Fatalf("EnsureConversation %s: %v", id, err)
		}
	}
	if err := db.UpdateConversationParent(ctx, s.DB, "child-a", "root-a"); err != nil {
		t.Fatalf("UpdateConversationParent child: %v", err)
	}
	if err := db.UpdateConversationParent(ctx, s.DB, "grandchild-a", "child-a"); err != nil {
		t.Fatalf("UpdateConversationParent grandchild: %v", err)
	}
	if err := db.InsertMessages(ctx, s.DB, []db.Message{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "root-a", Role: "user", Content: "root", RawJSON: "{}"},
		{Timestamp: 2000, ProjectID: pid, ConversationID: "child-a", Role: "user", Content: "child", RawJSON: "{}"},
		{Timestamp: 3000, ProjectID: pid, ConversationID: "grandchild-a", Role: "user", Content: "grandchild", RawJSON: "{}"},
		{Timestamp: 1500, ProjectID: pid, ConversationID: "root-b", Role: "user", Content: "other", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/projects/"+pid+"?page=1&pageSize=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data := env.Data.(map[string]any)
	conversations := data["conversations"].([]any)
	if len(conversations) != 3 {
		t.Fatalf("rows = %d, want 3 (root+child+grandchild)", len(conversations))
	}
	if conversations[0].(map[string]any)["id"] != "root-a" {
		t.Fatalf("first row = %v, want root-a", conversations[0].(map[string]any)["id"])
	}
	if conversations[1].(map[string]any)["id"] != "child-a" {
		t.Fatalf("second row = %v, want child-a", conversations[1].(map[string]any)["id"])
	}
	if conversations[2].(map[string]any)["id"] != "grandchild-a" {
		t.Fatalf("third row = %v, want grandchild-a", conversations[2].(map[string]any)["id"])
	}
}

func TestGetProjectHiddenFilter(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	for _, id := range []string{"conv-visible", "conv-hidden"} {
		if err := db.EnsureConversation(ctx, s.DB, id, pid, "codex"); err != nil {
			t.Fatalf("EnsureConversation %s: %v", id, err)
		}
	}
	if err := db.SetConversationHidden(ctx, s.DB, "conv-hidden", true); err != nil {
		t.Fatalf("SetConversationHidden: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/projects/"+pid, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("visible status = %d, want %d", rec.Code, http.StatusOK)
	}
	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode visible response: %v", err)
	}
	data := env.Data.(map[string]any)
	conversations := data["conversations"].([]any)
	if len(conversations) != 1 {
		t.Fatalf("visible conversations = %d, want 1", len(conversations))
	}

	req = httptest.NewRequest("GET", "/api/v1/projects/"+pid+"?hidden=true", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("hidden status = %d, want %d", rec.Code, http.StatusOK)
	}
	env = jsonEnvelope{}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode hidden response: %v", err)
	}
	data = env.Data.(map[string]any)
	conversations = data["conversations"].([]any)
	if len(conversations) != 1 {
		t.Fatalf("hidden conversations = %d, want 1", len(conversations))
	}
	first := conversations[0].(map[string]any)
	if first["id"] != "conv-hidden" {
		t.Fatalf("hidden conversation id = %v, want %q", first["id"], "conv-hidden")
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

func TestSetProjectIgnoreDiffPathsTriggersCoverageRecomputeWhenChanged(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	projectID, commitHash := setupSingleCommitProjectAndIngest(t, s, handler)
	setCommitCoverageVersion(t, s, projectID, commitHash, 0)

	body, _ := json.Marshal(map[string]string{"ignoreDiffPaths": "app.txt"})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+projectID+"/ignore-diff-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	waitForCommitCoverageVersion(t, ctx, s, projectID, commitHash, currentCommitCoverageVersion, 3*time.Second)
}

func TestSetProjectIgnoreDiffPathsDoesNotRecomputeWhenUnchanged(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	projectID, commitHash := setupSingleCommitProjectAndIngest(t, s, handler)
	paths := "app.txt"
	if err := db.SetProjectIgnoreDiffPaths(ctx, s.DB, projectID, paths); err != nil {
		t.Fatalf("SetProjectIgnoreDiffPaths: %v", err)
	}
	setCommitCoverageVersion(t, s, projectID, commitHash, 0)

	body, _ := json.Marshal(map[string]string{"ignoreDiffPaths": paths})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+projectID+"/ignore-diff-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	time.Sleep(300 * time.Millisecond)
	if got := getCommitCoverageVersion(t, ctx, s, projectID, commitHash); got != 0 {
		t.Fatalf("coverage_version = %d, want 0", got)
	}
}

func TestSetProjectIgnoreDiffPathsChangedWithoutMatchingDiffSkipsRecompute(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	projectID, commitHash := setupSingleCommitProjectAndIngest(t, s, handler)
	setCommitCoverageVersion(t, s, projectID, commitHash, 0)

	body, _ := json.Marshal(map[string]string{"ignoreDiffPaths": "docs/**"})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+projectID+"/ignore-diff-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	time.Sleep(300 * time.Millisecond)
	if got := getCommitCoverageVersion(t, ctx, s, projectID, commitHash); got != 0 {
		t.Fatalf("coverage_version = %d, want 0", got)
	}
}

func TestSetProjectIgnoreDefaultDiffPathsTriggersCoverageRecomputeWhenChanged(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	projectID, commitHash := setupSingleCommitProjectAndIngestFile(t, s, handler, "go.sum")
	setCommitCoverageVersion(t, s, projectID, commitHash, 0)

	body, _ := json.Marshal(map[string]bool{"ignoreDefaultDiffPaths": false})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+projectID+"/ignore-default-diff-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	waitForCommitCoverageVersion(t, ctx, s, projectID, commitHash, currentCommitCoverageVersion, 3*time.Second)
}

func TestProjectGroupFingerprintChangesWhenIgnoreSettingsChange(t *testing.T) {
	base := projectGroup{
		GitID: "git-1",
		Projects: []db.Project{
			{ID: "b", IgnoreDiffPaths: "docs/**"},
			{ID: "a", IgnoreDefaultDiffPaths: true},
		},
	}
	reordered := projectGroup{
		GitID: "git-1",
		Projects: []db.Project{
			{ID: "a", IgnoreDefaultDiffPaths: true},
			{ID: "b", IgnoreDiffPaths: "docs/**"},
		},
	}
	changed := projectGroup{
		GitID: "git-1",
		Projects: []db.Project{
			{ID: "a", IgnoreDefaultDiffPaths: true},
			{ID: "b", IgnoreDiffPaths: "README.md"},
		},
	}

	if got, want := projectGroupFingerprint(base), projectGroupFingerprint(reordered); got != want {
		t.Fatalf("fingerprint should be stable across project order: %q != %q", got, want)
	}
	if projectGroupFingerprint(base) == projectGroupFingerprint(changed) {
		t.Fatal("fingerprint did not change after ignore settings changed")
	}
}

func TestGetProjectReadOnlyUsesStoredFieldsWhenRepoMissing(t *testing.T) {
	s := setupTestServer(t)
	s.ReadOnly = true
	handler := s.Routes()
	ctx := context.Background()

	projectPath := filepath.Join(t.TempDir(), "missing-project")
	projectID, err := db.EnsureProject(ctx, s.DB, projectPath)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectDefaultBranch(ctx, s.DB, projectID, "main"); err != nil {
		t.Fatalf("UpdateProjectDefaultBranch: %v", err)
	}
	if err := db.UpdateProjectLocalUser(ctx, s.DB, projectID, "Stored User", "stored@example.com"); err != nil {
		t.Fatalf("UpdateProjectLocalUser: %v", err)
	}
	if err := db.UpdateProjectRemote(ctx, s.DB, projectID, "git@github.com:example/repo.git"); err != nil {
		t.Fatalf("UpdateProjectRemote: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID, nil)
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
	if got := data["currentBranch"].(string); got != "main" {
		t.Fatalf("currentBranch = %q, want %q", got, "main")
	}
	if got := data["localEmail"].(string); got != "stored@example.com" {
		t.Fatalf("localEmail = %q, want %q", got, "stored@example.com")
	}
}

func TestCommitRefreshRerunsWithLatestIgnoreDiffPaths(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	readmePath := filepath.Join(repo, "README.md")
	mustWriteFile(t, readmePath, "start\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "add", "README.md")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-readme", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	agentTs := mustUnixMilli(t, "2026-01-01T00:59:00Z")
	agentDiff := "```diff\n" +
		"diff --git a/README.md b/README.md\n" +
		"--- a/README.md\n" +
		"+++ b/README.md\n" +
		"@@ -1 +1,2 @@\n" +
		" start\n" +
		"+from agent\n" +
		"```"
	if err := db.InsertMessages(ctx, s.DB, []db.Message{{
		Timestamp:      agentTs,
		ProjectID:      projectID,
		ConversationID: "conv-readme",
		Role:           "agent",
		Content:        agentDiff,
		RawJSON:        agent.DerivedDiffRawJSON,
	}}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	mustWriteFile(t, readmePath, "start\nfrom agent\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "add", "README.md")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "commit", "-m", "agent readme change")
	commitHash := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	body, _ := json.Marshal(map[string]any{"count": 1})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+projectID+"/ingest-commits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ingest status = %d, want %d", rec.Code, http.StatusOK)
	}

	if got := getCommitLinesFromAgent(t, ctx, s, projectID, commitHash); got <= 0 {
		t.Fatalf("initial lines_from_agent = %d, want > 0", got)
	}

	stageReached := make(chan struct{})
	releaseStage := make(chan struct{})
	var once sync.Once
	s.afterCoverageStage = func(pid, stage string) {
		if pid != projectID || stage != "refresh_ingest" {
			return
		}
		once.Do(func() {
			close(stageReached)
			<-releaseStage
		})
	}

	refreshReq := httptest.NewRequest("POST", "/api/v1/projects/"+projectID+"/refresh-commits?branch=main", nil)
	refreshRec := httptest.NewRecorder()
	handler.ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, want %d", refreshRec.Code, http.StatusOK)
	}

	select {
	case <-stageReached:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for refresh ingest stage")
	}

	ignoreBody, _ := json.Marshal(map[string]string{"ignoreDiffPaths": "README.md"})
	ignoreReq := httptest.NewRequest("POST", "/api/v1/projects/"+projectID+"/ignore-diff-paths", bytes.NewReader(ignoreBody))
	ignoreReq.Header.Set("Content-Type", "application/json")
	ignoreRec := httptest.NewRecorder()
	handler.ServeHTTP(ignoreRec, ignoreReq)
	if ignoreRec.Code != http.StatusOK {
		t.Fatalf("ignore-diff-paths status = %d, want %d", ignoreRec.Code, http.StatusOK)
	}

	waitForCommitLinesFromAgent(t, ctx, s, projectID, commitHash, 0, 3*time.Second)
	close(releaseStage)
	waitForCommitRefresh(t, s)
	waitForCommitLinesFromAgent(t, ctx, s, projectID, commitHash, 0, 3*time.Second)
}

func TestProjectCoverageRecomputeCoalescesPerProject(t *testing.T) {
	s := setupTestServer(t)

	if !s.tryStartProjectCoverageRecompute("p1") {
		t.Fatal("first tryStartProjectCoverageRecompute returned false, want true")
	}
	if s.tryStartProjectCoverageRecompute("p1") {
		t.Fatal("second tryStartProjectCoverageRecompute returned true, want false")
	}
	if !s.tryStartProjectCoverageRecompute("p2") {
		t.Fatal("different project should be allowed to start")
	}
	s.finishProjectCoverageRecompute("p1")
	if !s.tryStartProjectCoverageRecompute("p1") {
		t.Fatal("project should be allowed after finish")
	}
}

func TestCommitDetailCacheKeyIncludesIgnorePatterns(t *testing.T) {
	k1 := commitDetailCacheKey("p1", "abc", []string{"CHANGELOG.md"})
	k2 := commitDetailCacheKey("p1", "abc", []string{"README.md"})
	if k1 == k2 {
		t.Fatalf("cache keys should differ when ignore patterns differ: %q == %q", k1, k2)
	}
}

func TestSetProjectIgnoreDiffPathsClearsCommitDetailCacheForProject(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	projectID, err := db.EnsureProject(ctx, s.DB, "/tmp/cache-clear-project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	otherProjectID, err := db.EnsureProject(ctx, s.DB, "/tmp/cache-clear-other")
	if err != nil {
		t.Fatalf("EnsureProject other: %v", err)
	}

	s.commitDetailCache.set(commitDetailCacheKey(projectID, "h1", []string{"README.md"}), &commitDetailCacheEntry{fetchedAt: time.Now()})
	s.commitDetailCache.set(commitDetailCacheKey(otherProjectID, "h2", []string{"README.md"}), &commitDetailCacheEntry{fetchedAt: time.Now()})

	body, _ := json.Marshal(map[string]string{"ignoreDiffPaths": "CHANGELOG.md"})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+projectID+"/ignore-diff-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if entry, ok := s.commitDetailCache.get(commitDetailCacheKey(projectID, "h1", []string{"README.md"})); ok && entry != nil {
		t.Fatalf("cache entry for project %s should have been cleared", projectID)
	}
}

func TestSetProjectOldPaths(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	oldPaths := "/old/path/one\n/old/path/two"
	body, _ := json.Marshal(map[string]string{"oldPaths": oldPaths})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+pid+"/old-paths", bytes.NewReader(body))
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
	if detail.OldPaths != oldPaths {
		t.Errorf("oldPaths = %q, want %q", detail.OldPaths, oldPaths)
	}
}

func TestSetProjectOldPathsTriggersAutomaticHistoryScanWhenChanged(t *testing.T) {
	w := &mockWatcher{name: "claude"}
	s := setupTestServerWithWatcher(t, w)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"oldPaths": "/old/path"})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+pid+"/old-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, scanPathsCount, _, _ := w.snapshot()
		if scanPathsCount > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, scanPathsCount, lastSince, lastPaths := w.snapshot()
	if scanPathsCount != 1 {
		t.Fatalf("scanPathsCount = %d, want 1", scanPathsCount)
	}
	if lastSince.Unix() != 0 {
		t.Fatalf("lastSince = %s, want Unix epoch", lastSince.Format(time.RFC3339))
	}
	if len(lastPaths) != 1 || lastPaths[0] != "/old/path" {
		t.Fatalf("lastPaths = %#v, want [/old/path]", lastPaths)
	}
}

func TestSetProjectOldPathsDoesNotScanWhenUnchanged(t *testing.T) {
	w := &mockWatcher{name: "claude"}
	s := setupTestServerWithWatcher(t, w)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.SetProjectOldPaths(ctx, s.DB, pid, "/old/path"); err != nil {
		t.Fatalf("SetProjectOldPaths: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"oldPaths": "/old/path"})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+pid+"/old-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	time.Sleep(50 * time.Millisecond)
	_, scanPathsCount, _, _ := w.snapshot()
	if scanPathsCount != 0 {
		t.Fatalf("scanPathsCount = %d, want 0", scanPathsCount)
	}
}

func TestSetProjectOldPathsReassignsExistingConversationsFromOldPathProject(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	currentProjectID, err := db.EnsureProject(ctx, s.DB, "/Users/davidcann/github/card-generator")
	if err != nil {
		t.Fatalf("EnsureProject current: %v", err)
	}
	oldProjectID, err := db.EnsureProject(ctx, s.DB, "/Users/davidcann/Downloads/card-generator")
	if err != nil {
		t.Fatalf("EnsureProject old: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-old", oldProjectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation old: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"oldPaths": "/Users/davidcann/Downloads/card-generator"})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+currentProjectID+"/old-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	detail, err := db.GetConversationDetail(ctx, s.DB, "conv-old")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail.ProjectID != currentProjectID {
		t.Fatalf("conversation project_id = %q, want %q", detail.ProjectID, currentProjectID)
	}
}

func TestSetProjectOldPathsNotFound(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	body, _ := json.Marshal(map[string]string{"oldPaths": "/old/path"})
	req := httptest.NewRequest("POST", "/api/v1/projects/nonexistent/old-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
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

func TestDeleteProject(t *testing.T) {
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

	req := httptest.NewRequest("DELETE", "/api/v1/projects/"+pid, nil)
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

	// Verify project is gone.
	projects, err := db.ListProjects(ctx, s.DB, false)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects after delete, got %d", len(projects))
	}
}

func TestDeleteProjectNotFound(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("DELETE", "/api/v1/projects/nonexistent", nil)
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

func setupSingleCommitProjectAndIngest(t *testing.T, s *Server, handler http.Handler) (string, string) {
	return setupSingleCommitProjectAndIngestFile(t, s, handler, "app.txt")
}

func setupSingleCommitProjectAndIngestFile(t *testing.T, s *Server, handler http.Handler, relPath string) (string, string) {
	t.Helper()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	filePath := filepath.Join(repo, relPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	mustWriteFile(t, filePath, "hello\n")
	gitRun(t, repo, nil, "add", relPath)
	gitRun(t, repo, nil, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))
	head := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"count": 1})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+projectID+"/ingest-commits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ingest status = %d, want %d", rec.Code, http.StatusOK)
	}

	return projectID, head
}

func setCommitCoverageVersion(t *testing.T, s *Server, projectID, commitHash string, version int) {
	t.Helper()
	if _, err := s.DB.Exec(
		`UPDATE commits SET coverage_version = ? WHERE project_id = ? AND commit_hash = ?`,
		version, projectID, commitHash,
	); err != nil {
		t.Fatalf("set commit coverage version: %v", err)
	}
}

func getCommitLinesFromAgent(t *testing.T, ctx context.Context, s *Server, projectID, commitHash string) int {
	t.Helper()
	var got int
	if err := s.DB.QueryRowContext(
		ctx,
		`SELECT lines_from_agent FROM commits WHERE project_id = ? AND commit_hash = ?`,
		projectID,
		commitHash,
	).Scan(&got); err != nil {
		t.Fatalf("query lines_from_agent: %v", err)
	}
	return got
}

func waitForCommitLinesFromAgent(
	t *testing.T,
	ctx context.Context,
	s *Server,
	projectID, commitHash string,
	want int,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := getCommitLinesFromAgent(t, ctx, s, projectID, commitHash); got == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for lines_from_agent=%d", want)
}

func getCommitCoverageVersion(t *testing.T, ctx context.Context, s *Server, projectID, commitHash string) int {
	t.Helper()
	var got int
	if err := s.DB.QueryRowContext(
		ctx,
		`SELECT coverage_version FROM commits WHERE project_id = ? AND commit_hash = ?`,
		projectID,
		commitHash,
	).Scan(&got); err != nil {
		t.Fatalf("query coverage version: %v", err)
	}
	return got
}

func waitForCommitCoverageVersion(
	t *testing.T,
	ctx context.Context,
	s *Server,
	projectID, commitHash string,
	want int,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := getCommitCoverageVersion(t, ctx, s, projectID, commitHash); got == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	got := getCommitCoverageVersion(t, ctx, s, projectID, commitHash)
	t.Fatalf("coverage_version = %d, want %d (timeout %s)", got, want, timeout)
}
