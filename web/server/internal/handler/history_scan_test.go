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

	"github.com/davidcann/zrate/web/server/internal/agent"
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
			wantStatus: http.StatusOK,
			wantOK:     true,
		},
		{
			name:       "custom timeframe",
			body:       map[string]any{"timeframe": "720h"},
			wantStatus: http.StatusOK,
			wantOK:     true,
		},
		{
			name:       "specific agent",
			body:       map[string]any{"agent": "claude"},
			wantStatus: http.StatusOK,
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

func TestHistoryScanResponseFields(t *testing.T) {
	w := &mockWatcher{name: "claude"}
	s := setupTestServerWithWatcher(t, w)
	handler := s.Routes()

	body, _ := json.Marshal(map[string]any{"timeframe": "168h"})
	req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			EntriesProcessed int    `json:"entriesProcessed"`
			Since            string `json:"since"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if env.Data.EntriesProcessed != 10 {
		t.Errorf("entriesProcessed = %d, want 10", env.Data.EntriesProcessed)
	}
	if env.Data.Since == "" {
		t.Error("since should not be empty")
	}
	if w.scanCount != 1 {
		t.Errorf("watcher scanCount = %d, want 1", w.scanCount)
	}
}

func TestHistoryScanMultipleWatchers(t *testing.T) {
	w1 := &mockWatcher{name: "claude"}
	w2 := &mockWatcher{name: "codex"}
	s := setupTestServerWithWatcher(t, w1, w2)
	handler := s.Routes()

	// No agent filter — should scan all.
	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if w1.scanCount != 1 {
		t.Errorf("w1 scanCount = %d, want 1", w1.scanCount)
	}
	if w2.scanCount != 1 {
		t.Errorf("w2 scanCount = %d, want 1", w2.scanCount)
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			EntriesProcessed int `json:"entriesProcessed"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if env.Data.EntriesProcessed != 20 {
		t.Errorf("entriesProcessed = %d, want 20 (10 per watcher)", env.Data.EntriesProcessed)
	}
}

func TestHistoryScanSpecificAgent(t *testing.T) {
	w1 := &mockWatcher{name: "claude"}
	w2 := &mockWatcher{name: "codex"}
	s := setupTestServerWithWatcher(t, w1, w2)
	handler := s.Routes()

	body, _ := json.Marshal(map[string]any{"agent": "codex"})
	req := httptest.NewRequest("POST", "/api/v1/history/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if w1.scanCount != 0 {
		t.Errorf("w1 scanCount = %d, want 0 (should not be scanned)", w1.scanCount)
	}
	if w2.scanCount != 1 {
		t.Errorf("w2 scanCount = %d, want 1", w2.scanCount)
	}
}
