package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/davidcann/zrate/web/server/internal/agent"
)

// Server holds dependencies shared across HTTP handlers.
type Server struct {
	DB     *sql.DB
	Agents *agent.Registry // may be nil if no agents are registered
}

// Routes returns an http.Handler with all routes and middleware wired up.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/rating", s.handleCreateRating)
	mux.HandleFunc("GET /api/v1/ratings", s.handleListRatings)
	mux.HandleFunc("GET /api/v1/projects", s.handleListProjects)
	mux.HandleFunc("GET /api/v1/projects/{id}", s.handleGetProject)
	mux.HandleFunc("POST /api/v1/projects/{id}/ignored", s.handleSetProjectIgnored)
	mux.HandleFunc("GET /api/v1/conversations", s.handleListConversations)
	mux.HandleFunc("GET /api/v1/conversations/{id}", s.handleGetConversation)
	mux.HandleFunc("POST /api/v1/history/scan", s.handleHistoryScan)
	mux.HandleFunc("GET /", s.handleDashboard)
	return corsMiddleware(mux)
}

type jsonEnvelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, jsonEnvelope{OK: false, Error: msg})
}

func writeSuccess(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, jsonEnvelope{OK: true, Data: data})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
