package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// mockWatcher implements agent.Watcher for testing.
type mockWatcher struct {
	mu             sync.Mutex
	name           string
	scanCount      int
	scanPathsCount int
	lastSince      time.Time
	lastPaths      []string
}

func (m *mockWatcher) Name() string            { return m.name }
func (m *mockWatcher) Run(ctx context.Context) {}
func (m *mockWatcher) ScanSince(ctx context.Context, since time.Time, progress agent.ScanProgressFunc) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scanCount++
	m.lastSince = since
	return 10
}
func (m *mockWatcher) ScanPathsSince(ctx context.Context, since time.Time, paths []string, progress agent.ScanProgressFunc) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scanPathsCount++
	m.lastSince = since
	m.lastPaths = append([]string(nil), paths...)
	return 10
}

func (m *mockWatcher) snapshot() (scanCount, scanPathsCount int, lastSince time.Time, lastPaths []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.scanCount, m.scanPathsCount, m.lastSince, append([]string(nil), m.lastPaths...)
}

func setupTestServerWithWatcher(t *testing.T, watchers ...*mockWatcher) *Server {
	t.Helper()
	s := setupTestServer(t)
	reg := agent.NewRegistry()
	for _, w := range watchers {
		reg.Register(w)
	}
	s.Agents = reg
	return s
}

// addTestProject inserts a project into the test DB so that history scan has paths to scan.
func addTestProject(t *testing.T, s *Server, path string) {
	t.Helper()
	_, err := db.EnsureProject(context.Background(), s.DB, path)
	if err != nil {
		t.Fatalf("ensure test project %q: %v", path, err)
	}
}

// waitForImportUnlock waits until the server's importMu is available, indicating
// the background scan job has completed. It does this by acquiring and releasing the lock.
func waitForImportUnlock(s *Server) {
	s.importMu.Lock()
	s.importMu.Unlock()
}

func TestHistoryScan(t *testing.T) {
	tests := []struct {
		name       string
		body       any
		wantStatus int
		wantOK     bool
	}{
		{
			name:       "default timeframe",
			body:       map[string]any{},
			wantStatus: http.StatusAccepted,
			wantOK:     true,
		},
		{
			name:       "custom timeframe",
			body:       map[string]any{"timeframe": "720h"},
			wantStatus: http.StatusAccepted,
			wantOK:     true,
		},
		{
			name:       "specific agent",
			body:       map[string]any{"agent": "claude"},
			wantStatus: http.StatusAccepted,
			wantOK:     true,
		},
		{
			name:       "invalid timeframe",
			body:       map[string]any{"timeframe": "not-a-duration"},
			wantStatus: http.StatusBadRequest,
			wantOK:     false,
		},
		{
			name:       "wrong content-type",
			body:       nil, // will use text/plain
			wantStatus: http.StatusUnsupportedMediaType,
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &mockWatcher{name: "claude"}
			s := setupTestServerWithWatcher(t, w)
			addTestProject(t, s, "/tmp/test-project")
			handler := s.Routes()

			contentType := "application/json"
			var bodyBytes []byte
			if tt.body == nil {
				contentType = "text/plain"
				bodyBytes = []byte("{}")
			} else {
				bodyBytes, _ = json.Marshal(tt.body)
			}

			req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", contentType)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			var env jsonEnvelope
			if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if env.OK != tt.wantOK {
				t.Errorf("ok = %v, want %v", env.OK, tt.wantOK)
			}

			// Wait for any background job to complete before the next test.
			if tt.wantOK {
				waitForImportUnlock(s)
			}
		})
	}
}

func TestHistoryScanNoAgents(t *testing.T) {
	s := setupTestServer(t) // no agents registered
	handler := s.Routes()

	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHistoryScanStartedResponse(t *testing.T) {
	w := &mockWatcher{name: "claude"}
	s := setupTestServerWithWatcher(t, w)
	addTestProject(t, s, "/tmp/test-project")
	handler := s.Routes()

	body, _ := json.Marshal(map[string]any{"timeframe": "168h"})
	req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Started bool `json:"started"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !env.Data.Started {
		t.Error("started should be true")
	}

	// Wait for the background scan to finish and verify watcher was called via ScanPathsSince.
	waitForImportUnlock(s)
	_, scanPathsCount, _, _ := w.snapshot()
	if scanPathsCount != 1 {
		t.Errorf("watcher scanPathsCount = %d, want 1", scanPathsCount)
	}
}

func TestHistoryScanMultipleWatchers(t *testing.T) {
	w1 := &mockWatcher{name: "claude"}
	w2 := &mockWatcher{name: "codex"}
	s := setupTestServerWithWatcher(t, w1, w2)
	addTestProject(t, s, "/tmp/test-project")
	handler := s.Routes()

	// No agent filter — should scan all.
	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	// Wait for the background scan to complete.
	waitForImportUnlock(s)

	_, pathsCount1, _, _ := w1.snapshot()
	_, pathsCount2, _, _ := w2.snapshot()
	if pathsCount1 != 1 {
		t.Errorf("w1 scanPathsCount = %d, want 1", pathsCount1)
	}
	if pathsCount2 != 1 {
		t.Errorf("w2 scanPathsCount = %d, want 1", pathsCount2)
	}
}

func TestHistoryScanSpecificAgent(t *testing.T) {
	w1 := &mockWatcher{name: "claude"}
	w2 := &mockWatcher{name: "codex"}
	s := setupTestServerWithWatcher(t, w1, w2)
	addTestProject(t, s, "/tmp/test-project")
	handler := s.Routes()

	body, _ := json.Marshal(map[string]any{"agent": "codex"})
	req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	// Wait for the background scan to complete.
	waitForImportUnlock(s)

	_, pathsCount1, _, _ := w1.snapshot()
	_, pathsCount2, _, _ := w2.snapshot()
	if pathsCount1 != 0 {
		t.Errorf("w1 scanPathsCount = %d, want 0 (should not be scanned)", pathsCount1)
	}
	if pathsCount2 != 1 {
		t.Errorf("w2 scanPathsCount = %d, want 1", pathsCount2)
	}
}

func TestHistoryScanNoProjects(t *testing.T) {
	w := &mockWatcher{name: "claude"}
	s := setupTestServerWithWatcher(t, w)
	// No projects added — scan should complete immediately with 0 entries.
	handler := s.Routes()

	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	waitForImportUnlock(s)

	scanCount, scanPathsCount, _, _ := w.snapshot()
	if scanCount != 0 {
		t.Errorf("watcher scanCount = %d, want 0 (no projects to scan)", scanCount)
	}
	if scanPathsCount != 0 {
		t.Errorf("watcher scanPathsCount = %d, want 0 (no projects to scan)", scanPathsCount)
	}
}

func TestHistoryScanConflict(t *testing.T) {
	w := &mockWatcher{name: "claude"}
	s := setupTestServerWithWatcher(t, w)
	handler := s.Routes()

	// Acquire the import lock to simulate an already-running import.
	s.importMu.Lock()

	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	s.importMu.Unlock()
}

func TestHistoryScanRetainsExistingConversationsAndMessages(t *testing.T) {
	w := &mockWatcher{name: "claude"}
	s := setupTestServerWithWatcher(t, w)
	handler := s.Routes()
	ctx := context.Background()

	projectID, err := db.EnsureProject(ctx, s.DB, "/tmp/test-project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-a", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation conv-a: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-b", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation conv-b: %v", err)
	}

	nowMs := time.Now().UnixMilli()
	oldTs := nowMs - int64((14*24*time.Hour)/time.Millisecond)
	recentTs := nowMs - int64((2*24*time.Hour)/time.Millisecond)
	if err := db.InsertMessages(ctx, s.DB, []db.Message{
		{Timestamp: oldTs, ProjectID: projectID, ConversationID: "conv-a", Role: "user", Content: "old", RawJSON: "{}"},
		{Timestamp: recentTs, ProjectID: projectID, ConversationID: "conv-b", Role: "user", Content: "recent", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var convBefore int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM conversations WHERE project_id = ?", projectID).Scan(&convBefore); err != nil {
		t.Fatalf("count conversations before: %v", err)
	}
	var msgBefore int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE project_id = ?", projectID).Scan(&msgBefore); err != nil {
		t.Fatalf("count messages before: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"timeframe": "168h"})
	req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	waitForImportUnlock(s)

	var convAfter int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM conversations WHERE project_id = ?", projectID).Scan(&convAfter); err != nil {
		t.Fatalf("count conversations after: %v", err)
	}
	if convAfter != convBefore {
		t.Fatalf("conversation count after = %d, want %d", convAfter, convBefore)
	}

	var msgAfter int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE project_id = ?", projectID).Scan(&msgAfter); err != nil {
		t.Fatalf("count messages after: %v", err)
	}
	if msgAfter != msgBefore {
		t.Fatalf("message count after = %d, want %d", msgAfter, msgBefore)
	}
}

func TestHistoryScanDoesNotDeleteOtherTables(t *testing.T) {
	w := &mockWatcher{name: "claude"}
	s := setupTestServerWithWatcher(t, w)
	handler := s.Routes()
	ctx := context.Background()

	projectID, err := db.EnsureProject(ctx, s.DB, "/tmp/test-project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-recent", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	recentTs := time.Now().Add(-2 * 24 * time.Hour).UnixMilli()
	if err := db.InsertMessages(ctx, s.DB, []db.Message{
		{Timestamp: recentTs, ProjectID: projectID, ConversationID: "conv-recent", Role: "user", Content: "recent", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}
	if _, err := db.InsertRating(ctx, s.DB, "conv-recent", 4, "keep", ""); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}
	if err := db.UpsertCommit(ctx, s.DB, db.Commit{
		ProjectID:   projectID,
		BranchName:  "main",
		CommitHash:  "abc123",
		Subject:     "subject",
		DiffContent: "diff --git a/a b/a",
	}); err != nil {
		t.Fatalf("UpsertCommit: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"timeframe": "168h"})
	req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	waitForImportUnlock(s)

	var ratingCount int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM ratings").Scan(&ratingCount); err != nil {
		t.Fatalf("count ratings: %v", err)
	}
	if ratingCount != 1 {
		t.Fatalf("ratings count = %d, want 1", ratingCount)
	}
	var commitCount int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM commits").Scan(&commitCount); err != nil {
		t.Fatalf("count commits: %v", err)
	}
	if commitCount != 1 {
		t.Fatalf("commits count = %d, want 1", commitCount)
	}
	var projectCount int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM projects").Scan(&projectCount); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if projectCount != 1 {
		t.Fatalf("projects count = %d, want 1", projectCount)
	}
}

func TestHistoryScanIsIdempotentForExistingMessages(t *testing.T) {
	w := &mockWatcher{name: "claude"}
	s := setupTestServerWithWatcher(t, w)
	ctx := context.Background()
	addTestProject(t, s, "/tmp/test-project")
	projectID, err := db.EnsureProject(ctx, s.DB, "/tmp/test-project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-idem", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	ts := time.Now().Add(-2 * time.Hour).UnixMilli()
	if err := db.InsertMessages(ctx, s.DB, []db.Message{
		{Timestamp: ts, ProjectID: projectID, ConversationID: "conv-idem", Role: "user", Content: "hello", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}
	handler := s.Routes()

	var before int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE project_id = ?", projectID).Scan(&before); err != nil {
		t.Fatalf("count messages before: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"timeframe": "168h"})
	req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	waitForImportUnlock(s)

	// Run a second scan to confirm no deletions/dup explosions for existing rows.
	req2 := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusAccepted {
		t.Fatalf("second status = %d, want %d", rec2.Code, http.StatusAccepted)
	}
	waitForImportUnlock(s)

	var after int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE project_id = ?", projectID).Scan(&after); err != nil {
		t.Fatalf("count messages after: %v", err)
	}
	if after != before {
		t.Fatalf("message count after = %d, want %d", after, before)
	}
}
