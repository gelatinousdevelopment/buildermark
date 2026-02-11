package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/davidcann/zrate/web/server/internal/db"
)

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	ignored := r.URL.Query().Get("ignored") == "true"
	projects, err := db.ListProjects(r.Context(), s.DB, ignored)
	if err != nil {
		log.Printf("error listing projects: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}
	writeSuccess(w, http.StatusOK, projects)
}

func (s *Server) handleSetProjectIgnored(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	var body struct {
		Ignored bool `json:"ignored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := db.SetProjectIgnored(r.Context(), s.DB, id, body.Ignored); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		log.Printf("error setting project ignored: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update project")
		return
	}

	writeSuccess(w, http.StatusOK, nil)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	project, err := db.GetProjectDetail(r.Context(), s.DB, id)
	if err != nil {
		log.Printf("error getting project: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get project")
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	writeSuccess(w, http.StatusOK, project)
}
