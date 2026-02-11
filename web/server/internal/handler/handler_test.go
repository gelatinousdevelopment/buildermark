package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, OPTIONS" {
		t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, "GET, POST, OPTIONS")
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
