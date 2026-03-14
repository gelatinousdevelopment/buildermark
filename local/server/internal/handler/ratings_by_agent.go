package handler

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func (s *Server) handleGetRatingsByAgent(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "projectId is required")
		return
	}

	q := r.URL.Query()
	startMs, _ := strconv.ParseInt(q.Get("start"), 10, 64)
	endMs, _ := strconv.ParseInt(q.Get("end"), 10, 64)
	if startMs <= 0 || endMs <= 0 || endMs <= startMs {
		writeError(w, http.StatusBadRequest, "start and end (ms) are required and end must be after start")
		return
	}

	rows, err := db.GetRatingsByAgent(r.Context(), s.DB, projectID, startMs, endMs)
	if err != nil {
		log.Printf("error getting ratings by agent: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get ratings by agent")
		return
	}

	writeSuccess(w, http.StatusOK, rows)
}
