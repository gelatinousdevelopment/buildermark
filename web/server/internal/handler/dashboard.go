package handler

import (
	"embed"
	"net/http"
)

//go:embed index.html
var dashboardHTML embed.FS

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	html, err := dashboardHTML.ReadFile("index.html")
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
