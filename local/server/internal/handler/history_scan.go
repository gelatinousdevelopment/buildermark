package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

type historyScanRequest struct {
	// Timeframe is a Go duration string (e.g. "720h" for 30 days, "168h" for 1 week).
	Timeframe string `json:"timeframe"`
	// Agent optionally limits the scan to a single agent. If empty, all watchers are scanned.
	Agent string `json:"agent"`
	// Sync runs the scan inline and blocks until complete instead of starting a background job.
	Sync bool `json:"sync"`
	// ProjectID limits the scan to a single known project path/worktree set.
	ProjectID string `json:"projectId"`
	// ReplaceDerivedDiffs deletes old synthetic diff messages for the scoped
	// project+agent before rescanning so regenerated diffs can replace them.
	ReplaceDerivedDiffs bool `json:"replaceDerivedDiffs"`
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
					count += pw.ScanPathsSince(ctx, since, paths, progress)
				} else {
					count += w.ScanSince(ctx, since, progress)
				}
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
	paths, err := s.historyScanPaths(r.Context(), req.ProjectID)
	if err != nil {
		if err == db.ErrNotFound {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to resolve project scan paths")
		return
	}

	// Try to acquire the import lock; reject if another import/scan is already running.
	if !s.importMu.TryLock() {
		writeError(w, http.StatusConflict, "an import is already in progress")
		return
	}
	scanCtx := r.Context()
	if req.ReplaceDerivedDiffs {
		scanCtx = db.WithReplaceDerivedDiffs(scanCtx)
	}
	if req.Sync {
		defer s.importMu.Unlock()
		count := s.scanWatchersSincePaths(scanCtx, since, req.Agent, paths, nil)
		writeSuccess(w, http.StatusOK, map[string]any{
			"started":          true,
			"completed":        true,
			"entriesProcessed": count,
		})
		return
	}

	// Return immediately — the scan runs in the background.
	writeSuccess(w, http.StatusAccepted, historyScanStartedResponse{Started: true})

	// Run the scan job asynchronously.
	go s.runHistoryScanJob(since, req, paths)
}

// runHistoryScanJob performs a history scan in the background, broadcasting
// progress over WebSocket. The caller must hold s.importMu.
func (s *Server) runHistoryScanJob(since time.Time, req historyScanRequest, paths []string) {
	defer s.importMu.Unlock()

	ctx := context.Background()
	if req.ReplaceDerivedDiffs {
		ctx = db.WithReplaceDerivedDiffs(ctx)
	}

	broadcast := func(state, message string) {
		s.ws.broadcastEvent("job_status", jobStatusEvent{
			JobType: "history_scan",
			State:   state,
			Message: message,
		})
	}

	broadcast("running", "Scanning conversation history...")

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

	// Pass nil paths so the scan is unfiltered — a manual re-import should
	// discover all conversations, not just those matching existing projects.
	count := s.scanWatchersSincePaths(ctx, since, req.Agent, paths, progress)

	// Recompute commit coverage for affected projects so newly imported
	// messages are matched against existing commits.
	broadcast("running", "Recomputing commit coverage...")
	s.recomputeCommitCoverageAfterHistoryScan(ctx, since, paths, broadcast)

	msg := fmt.Sprintf("Imported %d conversation entries", count)
	broadcast("complete", msg)
	s.sendNotification("history_scan_complete", "History scan complete", msg, "")
}

func (s *Server) historyScanPaths(ctx context.Context, projectID string) ([]string, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, nil
	}
	project, err := db.GetProjectDetail(ctx, s.DB, projectID)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, db.ErrNotFound
	}

	seen := make(map[string]struct{}, 1)
	paths := make([]string, 0, 4)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	add(project.Path)
	for _, path := range strings.Split(project.OldPaths, "\n") {
		add(path)
	}
	for _, path := range strings.Split(project.GitWorktreePaths, "\n") {
		add(path)
	}
	return paths, nil
}

// recomputeCommitCoverageAfterHistoryScan recomputes commit coverage for
// tracked projects so newly imported messages are matched against existing
// commits. If paths is non-nil, only projects whose path appears in the list
// are recomputed.
func (s *Server) recomputeCommitCoverageAfterHistoryScan(ctx context.Context, since time.Time, paths []string, broadcast func(string, string)) {
	groups, err := listAllProjectGroups(ctx, s.DB)
	if err != nil {
		log.Printf("history scan: failed to list projects for recompute: %v", err)
		return
	}

	pathSet := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		pathSet[p] = struct{}{}
	}

	afterMs := since.UnixMilli()
	recomputed := 0

	for _, group := range groups {
		repoProject, err := resolveRepoProject(ctx, group)
		if err != nil || repoProject == nil {
			continue
		}

		// If paths were specified, only recompute projects that match.
		if len(pathSet) > 0 {
			if !projectGroupMatchesPaths(group, pathSet) {
				continue
			}
		}

		branch := strings.TrimSpace(ensureProjectDefaultBranch(ctx, s.DB, repoProject))
		if branch == "" {
			branch = "main"
		}

		label := db.RepoLabel(repoProject.Path)
		broadcast("running", fmt.Sprintf("Recomputing commits for %s...", label))

		identity, _ := resolveGitIdentity(ctx, repoProject.Path)
		extraEmails := s.loadExtraLocalUserEmails()
		n, err := recomputeCommitCoverageForProjectWithChangedPatterns(
			ctx, s.DB, repoProject, group, branch, "", nil, &identity, extraEmails, nil, afterMs,
		)
		if err != nil {
			log.Printf("history scan: recompute failed for %s: %v", repoProject.Path, err)
			continue
		}
		recomputed += n
	}

	if recomputed > 0 {
		broadcast("running", fmt.Sprintf("Recomputed %d commits", recomputed))
	}
}

// projectGroupMatchesPaths returns true if any project in the group has a path
// (primary or worktree) that appears in the given path set.
func projectGroupMatchesPaths(group projectGroup, pathSet map[string]struct{}) bool {
	for _, p := range group.Projects {
		if _, ok := pathSet[p.Path]; ok {
			return true
		}
		for _, wt := range strings.Split(p.GitWorktreePaths, "\n") {
			wt = strings.TrimSpace(wt)
			if wt == "" {
				continue
			}
			if _, ok := pathSet[wt]; ok {
				return true
			}
		}
		for _, op := range strings.Split(p.OldPaths, "\n") {
			op = strings.TrimSpace(op)
			if op == "" {
				continue
			}
			if _, ok := pathSet[op]; ok {
				return true
			}
		}
	}
	return false
}
