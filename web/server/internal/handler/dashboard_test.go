package handler

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var cspNoncePattern = regexp.MustCompile(`script-src 'self' 'nonce-([^']+)'`)

func TestDashboard(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html prefix", ct)
	}

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
	matches := cspNoncePattern.FindStringSubmatch(csp)
	if len(matches) != 2 || matches[1] == "" {
		t.Fatalf("CSP missing nonce-based script-src: %q", csp)
	}
	nonce := matches[1]

	body := rec.Body.String()
	if !strings.Contains(body, `property="csp-nonce"`) {
		t.Fatalf("dashboard body missing csp nonce meta tag")
	}
	if !strings.Contains(body, `nonce="`+nonce+`"`) {
		t.Fatalf("dashboard body missing expected nonce value")
	}
}

func TestDashboardNotFound(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
