package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	ProjectsImported int `json:"projectsImported"`
	EntriesProcessed int `json:"entriesProcessed"`
	CommitsIngested  int `json:"commitsIngested"`
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

	projectIDs := make([]string, 0, len(roots))
	for _, root := range roots {
		projectID, err := db.EnsureProject(r.Context(), s.DB, root)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to ensure project")
			return
		}
		if err := db.SetProjectIgnored(r.Context(), s.DB, projectID, false); err != nil && !errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, "failed to update project tracking")
			return
		}

		project, err := getProjectByID(r.Context(), s.DB, projectID)
		if err != nil || project == nil {
			writeError(w, http.StatusInternalServerError, "failed to load project")
			return
		}
		if strings.TrimSpace(project.GitID) == "" {
			gitID, gitErr := gitRootCommit(r.Context(), root)
			if gitErr == nil && gitID != "" {
				if err := db.UpdateProjectGitID(r.Context(), s.DB, projectID, gitID); err == nil {
					project.GitID = gitID
				}
			}
		}
		_ = ensureProjectDefaultBranch(r.Context(), s.DB, project)
		projectIDs = append(projectIDs, projectID)
	}

	entriesProcessed := 0
	if s.Agents != nil && len(s.Agents.Watchers()) > 0 {
		entriesProcessed = s.scanWatchersSincePaths(r.Context(), since, "", roots)
	}

	groups, err := listAllProjectGroups(r.Context(), s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	commitsIngested := 0
	for _, projectID := range projectIDs {
		group, ok := findProjectGroupByProjectID(groups, projectID)
		if !ok {
			continue
		}
		repoProject, err := resolveRepoProject(r.Context(), group)
		if err != nil || repoProject == nil {
			continue
		}
		branch := strings.TrimSpace(ensureProjectDefaultBranch(r.Context(), s.DB, repoProject))
		if branch == "" {
			branch = "main"
		}

		ingested, err := IngestCommitsForWindow(r.Context(), s.DB, repoProject, group, branch, since, includeAll)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to ingest commits for %s", repoProject.Path))
			return
		}
		commitsIngested += ingested
	}

	writeSuccess(w, http.StatusOK, importProjectsResponse{
		ProjectsImported: len(projectIDs),
		EntriesProcessed: entriesProcessed,
		CommitsIngested:  commitsIngested,
	})
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

