package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/davidcann/zrate/web/server/internal/history"
)

type historyScanRequest struct {
	// Timeframe is a Go duration string (e.g. "720h" for 30 days, "168h" for 1 week).
	Timeframe string `json:"timeframe"`
}

type historyScanResponse struct {
	EntriesProcessed int    `json:"entriesProcessed"`
	Since            string `json:"since"`
}

func (s *Server) handleHistoryScan(w http.ResponseWriter, r *http.Request) {
	if s.Watcher == nil {
		writeError(w, http.StatusServiceUnavailable, "history watcher is not available")
		return
	}

	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req historyScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Timeframe == "" {
		req.Timeframe = history.DefaultScanWindow.String()
	}

	dur, err := time.ParseDuration(req.Timeframe)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid timeframe: use Go duration format (e.g. \"720h\" for 30 days)")
		return
	}

	since := time.Now().Add(-dur)
	count := s.Watcher.ScanSince(r.Context(), since)

	writeSuccess(w, http.StatusOK, historyScanResponse{
		EntriesProcessed: count,
		Since:            since.Format(time.RFC3339),
	})
}
