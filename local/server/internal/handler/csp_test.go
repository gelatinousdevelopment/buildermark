package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsFrontendHTMLRequest(t *testing.T) {
	tests := []struct {
		name string
		req  *http.Request
		want bool
	}{
		{
			name: "frontend html request",
			req:  httptest.NewRequest(http.MethodGet, "/", nil),
			want: true,
		},
		{
			name: "api request",
			req:  httptest.NewRequest(http.MethodGet, "/api/v1/ratings", nil),
			want: false,
		},
		{
			name: "non-get request",
			req:  httptest.NewRequest(http.MethodPost, "/", nil),
			want: false,
		},
		{
			name: "asset accept header only",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/_app/immutable/entry/app.js", nil)
				r.Header.Set("Accept", "application/javascript")
				return r
			}(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFrontendHTMLRequest(tt.req); got != tt.want {
				t.Fatalf("isFrontendHTMLRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInjectNonceIntoHTML(t *testing.T) {
	const nonce = "abc123nonce"
	input := `<!doctype html>
<html>
<head><title>x</title></head>
<body>
<script>console.log('a')</script>
<script nonce="already">console.log('b')</script>
</body>
</html>`

	out := injectNonceIntoHTML(input, nonce)

	if !strings.Contains(out, `<meta property="csp-nonce" nonce="`+nonce+`">`) {
		t.Fatalf("missing csp nonce meta: %s", out)
	}
	if !strings.Contains(out, `<script nonce="`+nonce+`">console.log('a')</script>`) {
		t.Fatalf("missing injected script nonce: %s", out)
	}
	if !strings.Contains(out, `<script nonce="`+nonce+`">console.log('b')</script>`) {
		t.Fatalf("existing nonce should be replaced with runtime nonce: %s", out)
	}
}

func TestNoCSPOnAPIResponses(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ratings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Security-Policy"); got != "" {
		t.Fatalf("unexpected CSP header on API response: %q", got)
	}
}
