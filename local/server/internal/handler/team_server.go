package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func (s *Server) handleListTeamServers(w http.ResponseWriter, r *http.Request) {
	servers, err := db.ListTeamServers(r.Context(), s.DB)
	if err != nil {
		log.Printf("error listing team servers: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list team servers")
		return
	}
	writeSuccess(w, http.StatusOK, servers)
}

func (s *Server) handleCreateTeamServer(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var body struct {
		Label  string `json:"label"`
		URL    string `json:"url"`
		APIKey string `json:"apiKey"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Label == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}
	if body.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	server, err := db.CreateTeamServer(r.Context(), s.DB, body.Label, body.URL, body.APIKey)
	if err != nil {
		log.Printf("error creating team server: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create team server")
		return
	}
	writeSuccess(w, http.StatusCreated, server)
}

func (s *Server) handleUpdateTeamServer(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "team server id is required")
		return
	}
	var body struct {
		Label  string `json:"label"`
		URL    string `json:"url"`
		APIKey string `json:"apiKey"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Label == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}
	if body.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	err := db.UpdateTeamServer(r.Context(), s.DB, id, body.Label, body.URL, body.APIKey)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "team server not found")
			return
		}
		log.Printf("error updating team server: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update team server")
		return
	}
	writeSuccess(w, http.StatusOK, nil)
}

func (s *Server) handleDeleteTeamServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "team server id is required")
		return
	}
	err := db.DeleteTeamServer(r.Context(), s.DB, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "team server not found")
			return
		}
		log.Printf("error deleting team server: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete team server")
		return
	}
	writeSuccess(w, http.StatusOK, nil)
}

func (s *Server) handleSetProjectTeamServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TeamServerID string `json:"teamServerId"`
	}
	id := decodeProjectSetterBody(w, r, &body)
	if id == "" {
		return
	}
	if handleProjectSetterError(w, db.SetProjectTeamServer(r.Context(), s.DB, id, body.TeamServerID), "team_server_id") {
		return
	}
	writeSuccess(w, http.StatusOK, nil)
}
