package handler

import (
	"log"
	"net/http"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func (s *Server) handleSearchProjects(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeSuccess(w, http.StatusOK, []db.ProjectSearchMatch{})
		return
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("projectId"))

	results, err := db.SearchProjectMatches(r.Context(), s.DB, query, projectID)
	if err != nil {
		log.Printf("error searching projects: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to search projects")
		return
	}

	writeSuccess(w, http.StatusOK, results)
}
