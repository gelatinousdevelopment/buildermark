package main

import (
	"database/sql"
	"embed"
	"net/http"
)

//go:embed index.html
var content embed.FS

func handleDashboard(db *sql.DB) http.HandlerFunc {
	html, _ := content.ReadFile("index.html")

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(html)
	}
}
