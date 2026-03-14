package handler

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func (s *Server) handleGetProjectActivity(w http.ResponseWriter, r *http.Request) {
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

	tzOffset, _ := strconv.Atoi(q.Get("tzOffset"))
	timeZone := q.Get("timeZone")

	rows, err := db.GetDailyActivity(r.Context(), s.DB, projectID, startMs, endMs, timeZone, tzOffset)
	if err != nil {
		log.Printf("error getting daily activity: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get daily activity")
		return
	}

	writeSuccess(w, http.StatusOK, rows)
}
