package handler

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:frontend
var frontendFS embed.FS

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Serve the SPA shell for all unmatched routes;
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

// staticFrontendHandler returns an http.Handler that serves the embedded
// frontend static assets (JS, CSS, etc.) under /_app/.
func staticFrontendHandler() http.Handler {
	sub, _ := fs.Sub(frontendFS, "frontend")
	return http.FileServer(http.FS(sub))
}
