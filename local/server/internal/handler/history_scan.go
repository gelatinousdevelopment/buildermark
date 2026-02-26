package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
)

type historyScanRequest struct {
	// Timeframe is a Go duration string (e.g. "720h" for 30 days, "168h" for 1 week).
	Timeframe string `json:"timeframe"`
	// Agent optionally limits the scan to a single agent. If empty, all watchers are scanned.
	Agent string `json:"agent"`
}

type historyScanStartedResponse struct {
	Started bool `json:"started"`
}

func (s *Server) scanWatchersSince(ctx context.Context, since time.Time, agentName string) int {
	return s.scanWatchersSincePaths(ctx, since, agentName, nil, nil)
}

func (s *Server) scanWatchersSincePaths(ctx context.Context, since time.Time, agentName string, paths []string, progress agent.ScanProgressFunc) int {
	var count int
	if agentName != "" {
		for _, w := range s.Agents.Watchers() {
			if w.Name() == agentName {
				if pw, ok := w.(agent.PathFilteredWatcher); ok && len(paths) > 0 {
					count = pw.ScanPathsSince(ctx, since, paths, progress)
				} else {
					count = w.ScanSince(ctx, since, progress)
				}
				break
			}
		}
		return count
	}
	for _, w := range s.Agents.Watchers() {
		if pw, ok := w.(agent.PathFilteredWatcher); ok && len(paths) > 0 {
			count += pw.ScanPathsSince(ctx, since, paths, progress)
		} else {
			count += w.ScanSince(ctx, since, progress)
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

	// Try to acquire the import lock; reject if another import/scan is already running.
	if !s.importMu.TryLock() {
		writeError(w, http.StatusConflict, "an import is already in progress")
		return
	}

	// Return immediately — the scan runs in the background.
	writeSuccess(w, http.StatusAccepted, historyScanStartedResponse{Started: true})

	// Run the scan job asynchronously.
	go s.runHistoryScanJob(since, req.Agent)
}

// runHistoryScanJob performs a history scan in the background, broadcasting
// progress over WebSocket. The caller must hold s.importMu.
func (s *Server) runHistoryScanJob(since time.Time, agentName string) {
	defer s.importMu.Unlock()

	ctx := context.Background()

	broadcast := func(state, message string) {
		s.ws.broadcastEvent("job_status", jobStatusEvent{
			JobType: "history_scan",
			State:   state,
			Message: message,
		})
	}

	broadcast("running", "Scanning conversation history...")

	// Only scan conversations belonging to existing projects.
	projectsByPath, err := listProjectsByPath(ctx, s.DB)
	if err != nil {
		broadcast("error", "Failed to list projects")
		return
	}
	if len(projectsByPath) == 0 {
		broadcast("complete", "Imported 0 conversation entries")
		return
	}

	// Collect all known paths: current paths plus old_paths aliases.
	paths := make([]string, 0, len(projectsByPath))
	for p, proj := range projectsByPath {
		paths = append(paths, p)
		for _, op := range strings.Split(proj.OldPaths, "\n") {
			op = strings.TrimSpace(op)
			if op != "" {
				paths = append(paths, op)
			}
		}
	}

	// Rate-limited progress: report file names no faster than every 50ms.
	var lastProgress time.Time
	progress := func(filename string) {
		now := time.Now()
		if now.Sub(lastProgress) < 50*time.Millisecond {
			return
		}
		lastProgress = now
		broadcast("running", fmt.Sprintf("Scanning %s", filepath.Base(filename)))
	}

	count := s.scanWatchersSincePaths(ctx, since, agentName, paths, progress)

	broadcast("complete", fmt.Sprintf("Imported %d conversation entries", count))
}
