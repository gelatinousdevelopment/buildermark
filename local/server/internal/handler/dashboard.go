package handler

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:frontend
var frontendFS embed.FS

var frontendFileServer http.Handler

func init() {
	sub, _ := fs.Sub(frontendFS, "frontend")
	frontendFileServer = http.FileServer(http.FS(sub))
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Try to serve a static file (e.g. favicon.png, robots.txt).
	if r.URL.Path != "/" {
		path := "frontend" + r.URL.Path
		if info, err := fs.Stat(frontendFS, path); err == nil && !info.IsDir() {
			frontendFileServer.ServeHTTP(w, r)
			return
		}
	}

	// Fall back to the SPA shell for all unmatched routes;
	// SvelteKit's client-side router handles routing.
	html, err := frontendFS.ReadFile("frontend/200.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	rendered := string(html)
	if nonce, ok := cspNonceFromContext(r.Context()); ok {
		rendered = injectNonceIntoHTML(rendered, nonce)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(rendered))
}

// staticFrontendHandler returns the shared http.Handler that serves the
// embedded frontend static assets (JS, CSS, etc.).
func staticFrontendHandler() http.Handler {
	return frontendFileServer
}
