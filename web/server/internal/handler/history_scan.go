package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/davidcann/zrate/web/server/internal/agent"
)

type historyScanRequest struct {
	// Timeframe is a Go duration string (e.g. "720h" for 30 days, "168h" for 1 week).
	Timeframe string `json:"timeframe"`
	// Agent optionally limits the scan to a single agent. If empty, all watchers are scanned.
	Agent string `json:"agent"`
}

type historyScanResponse struct {
	EntriesProcessed int    `json:"entriesProcessed"`
	Since            string `json:"since"`
}

func (s *Server) scanWatchersSince(ctx context.Context, since time.Time, agentName string) int {
	return s.scanWatchersSincePaths(ctx, since, agentName, nil)
}

func (s *Server) scanWatchersSincePaths(ctx context.Context, since time.Time, agentName string, paths []string) int {
	var count int
	if agentName != "" {
		for _, w := range s.Agents.Watchers() {
			if w.Name() == agentName {
				if pw, ok := w.(agent.PathFilteredWatcher); ok && len(paths) > 0 {
					count = pw.ScanPathsSince(ctx, since, paths)
				} else {
					count = w.ScanSince(ctx, since)
				}
				break
			}
		}
		return count
	}
	for _, w := range s.Agents.Watchers() {
		if pw, ok := w.(agent.PathFilteredWatcher); ok && len(paths) > 0 {
			count += pw.ScanPathsSince(ctx, since, paths)
		} else {
			count += w.ScanSince(ctx, since)
		}
	}
	return count
}

func (s *Server) handleHistoryScan(w http.ResponseWriter, r *http.Request) {
	if s.Agents == nil || len(s.Agents.Watchers()) == 0 {
		writeError(w, http.StatusServiceUnavailable, "history watcher is not available")
		return
	}

	if !requireJSON(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req historyScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Timeframe == "" {
		req.Timeframe = agent.DefaultScanWindow.String()
	}

	dur, err := time.ParseDuration(req.Timeframe)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid timeframe: use Go duration format (e.g. \"720h\" for 30 days)")
		return
	}

	since := time.Now().Add(-dur)
	count := s.scanWatchersSince(r.Context(), since, req.Agent)

	writeSuccess(w, http.StatusOK, historyScanResponse{
		EntriesProcessed: count,
		Since:            since.Format(time.RFC3339),
	})
}
