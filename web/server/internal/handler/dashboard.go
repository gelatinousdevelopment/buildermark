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
	html, _ := dashboardHTML.ReadFile("index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(html)
}
