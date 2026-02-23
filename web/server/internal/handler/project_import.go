package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/davidcann/zrate/web/server/internal/agent"
	"github.com/davidcann/zrate/web/server/internal/db"
)

const (
	defaultDiscoveryDays = 30
	defaultImportDays    = "90"
)

type discoverImportableProjectsResponse struct {
	Projects []discoverImportableProject `json:"projects"`
	Since    string                      `json:"since"`
}

type discoverImportableProject struct {
	Path      string `json:"path"`
	Label     string `json:"label"`
	ProjectID string `json:"projectId,omitempty"`
	Tracked   bool   `json:"tracked"`
}

type importProjectsRequest struct {
	Paths       []string `json:"paths"`
	HistoryDays string   `json:"historyDays"`
}

type importProjectsResponse struct {
	Started bool `json:"started"`
}

// importStatusEvent is broadcast over WebSocket during an import job.
type importStatusEvent struct {
	State            string `json:"state"`   // "running", "complete", "error"
	Message          string `json:"message"` // human-readable status line
	ProjectsImported int    `json:"projectsImported"`
	EntriesProcessed int    `json:"entriesProcessed"`
	CommitsIngested  int    `json:"commitsIngested"`
}

func (s *Server) handleDiscoverImportableProjects(w http.ResponseWriter, r *http.Request) {
	if s.Agents == nil {
		writeSuccess(w, http.StatusOK, discoverImportableProjectsResponse{Projects: []discoverImportableProject{}})
		return
	}

	days := defaultDiscoveryDays
	if rawDays := strings.TrimSpace(r.URL.Query().Get("days")); rawDays != "" {
		parsed, err := strconv.Atoi(rawDays)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "days must be a positive integer")
			return
		}
		days = parsed
	}

	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	paths := make(map[string]struct{})
	for _, discoverer := range s.Agents.ProjectPathDiscoverers() {
		for _, path := range discoverer.DiscoverProjectPathsSince(r.Context(), since) {
			root, ok := agent.FindGitRoot(path)
			if !ok {
				continue
			}
			paths[root] = struct{}{}
		}
	}

	projectIndex, err := listProjectsByPath(r.Context(), s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	candidates := make([]discoverImportableProject, 0, len(paths))
	for path := range paths {
		candidate := discoverImportableProject{
			Path:  path,
			Label: db.RepoLabel(path),
		}
		if existing, ok := projectIndex[path]; ok {
			candidate.ProjectID = existing.ID
			candidate.Tracked = !existing.Ignored
			if strings.TrimSpace(existing.Label) != "" {
				candidate.Label = existing.Label
			}
		}
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Path < candidates[j].Path
	})

	writeSuccess(w, http.StatusOK, discoverImportableProjectsResponse{
		Projects: candidates,
		Since:    since.Format(time.RFC3339),
	})
}

func (s *Server) handleImportProjects(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req importProjectsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	since, includeAll, err := importSinceForHistoryDays(req.HistoryDays)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	roots, err := normalizeImportPaths(req.Paths)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Try to acquire the import lock; reject if another import is already running.
	if !s.importMu.TryLock() {
		writeError(w, http.StatusConflict, "an import is already in progress")
		return
	}

	// Return immediately — the import runs in the background.
	writeSuccess(w, http.StatusAccepted, importProjectsResponse{Started: true})

	// Run the import job asynchronously.
	go s.runImportJob(roots, since, includeAll)
}

// runImportJob performs the full import in the background, broadcasting
// progress over WebSocket. The caller must hold s.importMu.
func (s *Server) runImportJob(roots []string, since time.Time, includeAll bool) {
	defer s.importMu.Unlock()

	ctx := context.Background()

	broadcast := func(state, message string, projects, entries, commits int) {
		s.ws.broadcastEvent("import_status", importStatusEvent{
			State:            state,
			Message:          message,
			ProjectsImported: projects,
			EntriesProcessed: entries,
			CommitsIngested:  commits,
		})
	}

	broadcast("running", fmt.Sprintf("Setting up %d project(s)...", len(roots)), 0, 0, 0)

	projectIDs := make([]string, 0, len(roots))
	for _, root := range roots {
		label := db.RepoLabel(root)
		broadcast("running", fmt.Sprintf("Ensuring project %s...", label), len(projectIDs), 0, 0)

		projectID, err := db.EnsureProject(ctx, s.DB, root)
		if err != nil {
			broadcast("error", fmt.Sprintf("Failed to ensure project %s: %v", label, err), len(projectIDs), 0, 0)
			return
		}
		if err := db.SetProjectIgnored(ctx, s.DB, projectID, false); err != nil && !errors.Is(err, db.ErrNotFound) {
			broadcast("error", fmt.Sprintf("Failed to update project tracking for %s: %v", label, err), len(projectIDs), 0, 0)
			return
		}

		project, err := getProjectByID(ctx, s.DB, projectID)
		if err != nil || project == nil {
			broadcast("error", fmt.Sprintf("Failed to load project %s", label), len(projectIDs), 0, 0)
			return
		}
		if strings.TrimSpace(project.GitID) == "" {
			gitID, gitErr := gitRootCommit(ctx, root)
			if gitErr == nil && gitID != "" {
				if err := db.UpdateProjectGitID(ctx, s.DB, projectID, gitID); err == nil {
					project.GitID = gitID
				}
			}
		}
		_ = ensureProjectDefaultBranch(ctx, s.DB, project)
		projectIDs = append(projectIDs, projectID)
	}

	entriesProcessed := 0
	if s.Agents != nil && len(s.Agents.Watchers()) > 0 {
		broadcast("running", "Scanning conversation history...", len(projectIDs), 0, 0)
		entriesProcessed = s.scanWatchersSincePaths(ctx, since, "", roots)
		broadcast("running", fmt.Sprintf("Found %d conversation entries", entriesProcessed), len(projectIDs), entriesProcessed, 0)
	}

	groups, err := listAllProjectGroups(ctx, s.DB)
	if err != nil {
		broadcast("error", "Failed to list projects", len(projectIDs), entriesProcessed, 0)
		return
	}

	commitsIngested := 0
	for i, projectID := range projectIDs {
		group, ok := findProjectGroupByProjectID(groups, projectID)
		if !ok {
			continue
		}
		repoProject, err := resolveRepoProject(ctx, group)
		if err != nil || repoProject == nil {
			continue
		}
		branch := strings.TrimSpace(ensureProjectDefaultBranch(ctx, s.DB, repoProject))
		if branch == "" {
			branch = "main"
		}

		label := db.RepoLabel(repoProject.Path)
		broadcast("running", fmt.Sprintf("Ingesting commits for %s (%d/%d)...", label, i+1, len(projectIDs)), len(projectIDs), entriesProcessed, commitsIngested)

		ingested, err := IngestCommitsForWindow(ctx, s.DB, repoProject, group, branch, since, includeAll)
		if err != nil {
			log.Printf("error ingesting commits for %s: %v", repoProject.Path, err)
			broadcast("error", fmt.Sprintf("Failed to ingest commits for %s", label), len(projectIDs), entriesProcessed, commitsIngested)
			return
		}
		commitsIngested += ingested
	}

	broadcast("complete", fmt.Sprintf("Imported %d project(s), %d entries, %d commits", len(projectIDs), entriesProcessed, commitsIngested), len(projectIDs), entriesProcessed, commitsIngested)
}

func normalizeImportPaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("paths is required")
	}
	seen := make(map[string]struct{})
	roots := make([]string, 0, len(paths))
	for _, path := range paths {
		root, ok := agent.FindGitRoot(path)
		if !ok {
			continue
		}
		if _, exists := seen[root]; exists {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no valid git repository paths were provided")
	}
	sort.Strings(roots)
	return roots, nil
}

func importSinceForHistoryDays(historyDays string) (time.Time, bool, error) {
	historyDays = strings.ToLower(strings.TrimSpace(historyDays))
	if historyDays == "" {
		historyDays = defaultImportDays
	}
	if historyDays == "all" {
		return time.Time{}, true, nil
	}
	allowed := map[string]bool{
		"7":   true,
		"14":  true,
		"30":  true,
		"60":  true,
		"90":  true,
		"180": true,
		"365": true,
	}
	if !allowed[historyDays] {
		return time.Time{}, false, fmt.Errorf("historyDays must be one of: 7, 14, 30, 60, 90, 180, 365, all")
	}
	days, _ := strconv.Atoi(historyDays)
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour), false, nil
}

func listProjectsByPath(ctx context.Context, database *sql.DB) (map[string]db.Project, error) {
	active, err := db.ListProjects(ctx, database, false)
	if err != nil {
		return nil, err
	}
	ignored, err := db.ListProjects(ctx, database, true)
	if err != nil {
		return nil, err
	}
	all := append(active, ignored...)
	byPath := make(map[string]db.Project, len(all))
	for _, project := range all {
		path := strings.TrimSpace(project.Path)
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		if path == "." {
			continue
		}
		byPath[path] = project
	}
	return byPath, nil
}

