package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestCORSNoOriginAllowed(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	// Requests with no Origin header (native/desktop clients) should pass
	// through without CORS headers.
	req := httptest.NewRequest("GET", "/api/v1/ratings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty (no Origin sent)", got)
	}
}

func TestCORSExtensionOriginAllowed(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	for _, origin := range []string{
		"chrome-extension://abcdef1234567890",
		"moz-extension://abcdef-1234-5678",
		"safari-web-extension://abcdef1234567890",
	} {
		req := httptest.NewRequest("GET", "/api/v1/ratings", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("[%s] status = %d, want %d", origin, rec.Code, http.StatusOK)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("[%s] Access-Control-Allow-Origin = %q, want %q", origin, got, origin)
		}
		if got := rec.Header().Get("Access-Control-Allow-Private-Network"); got != "true" {
			t.Errorf("[%s] Access-Control-Allow-Private-Network = %q, want %q", origin, got, "true")
		}
	}
}

func TestCORSPreflightExtensionOrigin(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("OPTIONS", "/api/v1/rating", nil)
	req.Header.Set("Origin", "chrome-extension://abcdef1234567890")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "chrome-extension://abcdef1234567890" {
		t.Errorf("Access-Control-Allow-Origin = %q, want extension origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Errorf("Access-Control-Allow-Private-Network = %q, want %q", got, "true")
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body for OPTIONS, got %d bytes", rec.Body.Len())
	}
}

func TestCORSBlocksWebOrigin(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	// GET from a web origin: should proceed but without CORS headers so the
	// browser rejects the response.
	req := httptest.NewRequest("GET", "/api/v1/ratings", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for blocked origin", got)
	}
}

func TestCORSPreflightBlocksWebOrigin(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("OPTIONS", "/api/v1/rating", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCORSExtraOriginsAllowed(t *testing.T) {
	s := setupTestServer(t)
	configDir := t.TempDir()
	s.ConfigDir = configDir

	cfg := localConfigFile{ExtraCORSOrigins: []string{"http://localhost:5173"}}
	if err := saveLocalConfigFile(configDir, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	handler := s.Routes()

	// GET from the configured extra origin should receive CORS headers.
	req := httptest.NewRequest("GET", "/api/v1/ratings", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:5173")
	}

	// An origin NOT in the list should still be blocked.
	req2 := httptest.NewRequest("GET", "/api/v1/ratings", nil)
	req2.Header.Set("Origin", "http://localhost:9999")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if got := rec2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for unlisted origin", got)
	}
}

func TestCORSRequiresJSONContentType(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	// POST from extension origin without application/json should be rejected.
	req := httptest.NewRequest("POST", "/api/v1/rating", strings.NewReader("{}"))
	req.Header.Set("Origin", "chrome-extension://abc123")
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnsupportedMediaType)
	}
}

func TestCORSAllowsDeleteWithoutContentType(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	// DELETE is a non-simple CORS method that always triggers a preflight,
	// so we don't require Content-Type: application/json on it.
	req := httptest.NewRequest("DELETE", "/api/v1/ratings/fake-id", nil)
	req.Header.Set("Origin", "chrome-extension://abc123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnsupportedMediaType {
		t.Errorf("DELETE without Content-Type should not return 415")
	}
}

func TestWriteErrorFormat(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	// Request a non-existent conversation to trigger a 404 error response.
	req := httptest.NewRequest("GET", "/api/v1/conversations/nope", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if env.OK {
		t.Error("ok should be false for error responses")
	}
	if env.Error == "" {
		t.Error("error message should not be empty")
	}
	if env.Data != nil {
		t.Errorf("data should be nil for error responses, got %v", env.Data)
	}
}

func TestReadOnlyBlocksMutations(t *testing.T) {
	s := setupTestServer(t)
	s.ReadOnly = true
	handler := s.Routes()

	req := httptest.NewRequest("POST", "/api/v1/rating", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if env.OK {
		t.Fatal("ok should be false for read-only error")
	}
	if env.Error != "server is in read-only mode" {
		t.Fatalf("error = %q, want %q", env.Error, "server is in read-only mode")
	}
}

func TestReadOnlyAllowsGetRequests(t *testing.T) {
	s := setupTestServer(t)
	s.ReadOnly = true
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/v1/ratings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
func TestWriteSuccessFormat(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/v1/ratings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Error("ok should be true for success responses")
	}
	if env.Error != "" {
		t.Errorf("error should be empty for success responses, got %q", env.Error)
	}
	if env.Data == nil {
		t.Error("data should not be nil for success responses")
	}

	// Content-Type should be JSON.
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestWebSocketReceivesRunningJobsOnReconnect(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	dialURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"

	firstConn, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		t.Fatalf("dial first websocket: %v", err)
	}

	s.ws.broadcastEvent("job_status", jobStatusEvent{
		JobType: "history_scan",
		State:   "running",
		Message: "still scanning",
	})

	if err := firstConn.Close(); err != nil {
		t.Fatalf("close first websocket: %v", err)
	}

	secondConn, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		t.Fatalf("dial second websocket: %v", err)
	}
	t.Cleanup(func() { _ = secondConn.Close() })

	if err := secondConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, payload, err := secondConn.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket message: %v", err)
	}

	var msg wsMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("unmarshal websocket envelope: %v", err)
	}
	if msg.Type != "job_status" {
		t.Fatalf("message type = %q, want %q", msg.Type, "job_status")
	}

	var status jobStatusEvent
	if err := json.Unmarshal(msg.Data, &status); err != nil {
		t.Fatalf("unmarshal websocket data: %v", err)
	}
	if status.JobType != "history_scan" {
		t.Fatalf("job type = %q, want %q", status.JobType, "history_scan")
	}
	if status.State != "running" {
		t.Fatalf("state = %q, want %q", status.State, "running")
	}
}
