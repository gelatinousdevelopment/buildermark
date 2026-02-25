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
	"testing"
	"time"

	"github.com/gelatinousdevelopment/buildermark/web/server/internal/db"
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

	commitDetailCacheMu.Lock()
	commitDetailCache = map[string]*commitDetailCacheEntry{
		commitDetailCacheKey(projectID, "h1", []string{"README.md"}):      {fetchedAt: time.Now()},
		commitDetailCacheKey(otherProjectID, "h2", []string{"README.md"}): {fetchedAt: time.Now()},
	}
	commitDetailCacheMu.Unlock()

	body, _ := json.Marshal(map[string]string{"ignoreDiffPaths": "CHANGELOG.md"})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+projectID+"/ignore-diff-paths", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	commitDetailCacheMu.RLock()
	defer commitDetailCacheMu.RUnlock()
	for key := range commitDetailCache {
		if strings.HasPrefix(key, projectID+":") {
			t.Fatalf("cache key %q should have been cleared for project %s", key, projectID)
		}
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
