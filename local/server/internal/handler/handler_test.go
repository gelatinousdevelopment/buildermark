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

func TestCORSHeaders(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/v1/ratings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
		t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type" {
		t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, "Content-Type")
	}
}

func TestCORSPreflight(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("OPTIONS", "/api/v1/rating", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	// Should still have CORS headers.
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}

	// Body should be empty for preflight.
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body for OPTIONS, got %d bytes", rec.Body.Len())
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
