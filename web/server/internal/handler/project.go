package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

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

func (s *Server) handleSetProjectLabel(w http.ResponseWriter, r *http.Request) {
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
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if body.Label == "" {
		writeError(w, http.StatusBadRequest, "label must not be empty")
		return
	}

	if err := db.SetProjectLabel(r.Context(), s.DB, id, body.Label); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		log.Printf("error setting project label: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update project")
		return
	}

	writeSuccess(w, http.StatusOK, nil)
}

type projectDetailResponse struct {
	*db.ProjectDetail
	RemoteURL string `json:"remoteUrl"`
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	page := 1
	pageSize := 0
	pageRaw := strings.TrimSpace(r.URL.Query().Get("page"))
	pageSizeRaw := strings.TrimSpace(r.URL.Query().Get("pageSize"))
	if pageRaw != "" || pageSizeRaw != "" {
		page = parsePositiveInt(pageRaw, 1)
		pageSize = parsePositiveInt(pageSizeRaw, 10)
	}

	project, err := db.GetProjectDetailPage(r.Context(), s.DB, id, page, pageSize)
	if err != nil {
		log.Printf("error getting project: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get project")
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	ensureProjectLocalUser(r.Context(), s.DB, project)

	writeSuccess(w, http.StatusOK, projectDetailResponse{
		ProjectDetail: project,
		RemoteURL:     remoteURL(project.Remote),
	})
}

func (s *Server) handleSetProjectIgnoreDiffPaths(w http.ResponseWriter, r *http.Request) {
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
		IgnoreDiffPaths string `json:"ignoreDiffPaths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := db.SetProjectIgnoreDiffPaths(r.Context(), s.DB, id, body.IgnoreDiffPaths); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		log.Printf("error setting project ignore_diff_paths: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update project")
		return
	}

	writeSuccess(w, http.StatusOK, nil)
}

func (s *Server) handleSetProjectIgnoreDefaultDiffPaths(w http.ResponseWriter, r *http.Request) {
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
		IgnoreDefaultDiffPaths bool `json:"ignoreDefaultDiffPaths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := db.SetProjectIgnoreDefaultDiffPaths(r.Context(), s.DB, id, body.IgnoreDefaultDiffPaths); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		log.Printf("error setting project ignore_default_diff_paths: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update project")
		return
	}

	writeSuccess(w, http.StatusOK, nil)
}
