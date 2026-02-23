package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

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

func (s *Server) handleSetProjectOldPaths(w http.ResponseWriter, r *http.Request) {
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
		OldPaths string `json:"oldPaths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	prevOldPaths, err := db.GetProjectOldPaths(r.Context(), s.DB, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		log.Printf("error reading project old_paths: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update project")
		return
	}

	if err := db.SetProjectOldPaths(r.Context(), s.DB, id, body.OldPaths); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		log.Printf("error setting project old_paths: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update project")
		return
	}

	var movedConversations int64
	for _, oldPath := range splitLines(body.OldPaths) {
		moved, err := db.ReassignProjectDataByPath(r.Context(), s.DB, id, oldPath)
		if err != nil {
			log.Printf("warning: failed to reassign data for old path %q on project %s: %v", oldPath, id, err)
			continue
		}
		movedConversations += moved
	}

	if movedConversations > 0 {
		log.Printf("project old_paths changed for %s; reassigned %d existing conversations", id, movedConversations)
	}

	changed := body.OldPaths != prevOldPaths
	currentPaths := splitLines(body.OldPaths)
	if changed || movedConversations > 0 || len(currentPaths) > 0 {
		scanPaths := diffAddedPaths(prevOldPaths, body.OldPaths)
		go s.backfillProjectForOldPaths(id, scanPaths)
	}

	writeSuccess(w, http.StatusOK, nil)
}

func (s *Server) backfillProjectForOldPaths(projectID string, paths []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	if s.Agents != nil && len(s.Agents.Watchers()) > 0 && len(paths) > 0 {
		entriesProcessed := s.scanWatchersSincePaths(ctx, time.Unix(0, 0), "", paths, nil)
		log.Printf("project old_paths changed for %s; automatic path-filtered history scan processed %d entries across %d paths", projectID, entriesProcessed, len(paths))
	}

	if n, err := s.recomputeProjectCoverageAllBranches(ctx, projectID); err != nil {
		log.Printf("project old_paths changed for %s; coverage recompute failed: %v", projectID, err)
	} else if n > 0 {
		log.Printf("project old_paths changed for %s; recomputed coverage on %d branch(es)", projectID, n)
	}
}

func (s *Server) recomputeProjectCoverageAllBranches(ctx context.Context, projectID string) (int, error) {
	project, err := getProjectByID(ctx, s.DB, projectID)
	if err != nil {
		return 0, err
	}
	if project == nil {
		return 0, db.ErrNotFound
	}

	groups, err := listAllProjectGroups(ctx, s.DB)
	if err != nil {
		return 0, err
	}
	group, ok := findProjectGroupByProjectID(groups, project.ID)
	if !ok {
		return 0, nil
	}
	repoProject, err := resolveRepoProject(ctx, group)
	if err != nil {
		return 0, nil
	}

	branches := make(map[string]struct{})
	defaultBranch := strings.TrimSpace(ensureProjectDefaultBranch(ctx, s.DB, repoProject))
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	branches[defaultBranch] = struct{}{}
	if repoBranches, err := listRepoBranches(ctx, repoProject.Path, defaultBranch); err == nil {
		for _, b := range repoBranches {
			b = strings.TrimSpace(b)
			if b != "" {
				branches[b] = struct{}{}
			}
		}
	}
	if rows, err := s.DB.QueryContext(ctx, "SELECT DISTINCT branch_name FROM commits WHERE project_id = ? AND branch_name <> ''", repoProject.ID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var b string
			if err := rows.Scan(&b); err == nil {
				b = strings.TrimSpace(b)
				if b != "" {
					branches[b] = struct{}{}
				}
			}
		}
	}

	branchList := make([]string, 0, len(branches))
	for b := range branches {
		branchList = append(branchList, b)
	}
	sort.Strings(branchList)

	recomputed := 0
	for _, branch := range branchList {
		if _, err := recomputeCommitCoverageForProject(ctx, s.DB, repoProject, group, branch); err != nil {
			log.Printf("warning: recompute commit coverage failed for project=%s branch=%s: %v", projectID, branch, err)
			continue
		}
		recomputed++
	}
	return recomputed, nil
}

func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func diffAddedPaths(prev, next string) []string {
	prevSet := make(map[string]struct{})
	for _, p := range splitLines(prev) {
		prevSet[p] = struct{}{}
	}
	out := make([]string, 0, 4)
	for _, p := range splitLines(next) {
		if _, exists := prevSet[p]; exists {
			continue
		}
		out = append(out, p)
	}
	return out
}

type projectDetailResponse struct {
	*db.ProjectDetail
	RemoteURL     string `json:"remoteUrl"`
	CurrentBranch string `json:"currentBranch"`
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

	var filters db.ConversationFilters
	filters.Agent = strings.TrimSpace(r.URL.Query().Get("agent"))
	if ratingRaw := strings.TrimSpace(r.URL.Query().Get("rating")); ratingRaw != "" {
		if v, err := strconv.Atoi(ratingRaw); err == nil {
			filters.Rating = v
		}
	}

	project, err := db.GetProjectDetailPage(r.Context(), s.DB, id, page, pageSize, filters)
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

	currentBranch := detectCurrentBranch(r.Context(), project.Path)

	writeSuccess(w, http.StatusOK, projectDetailResponse{
		ProjectDetail: project,
		RemoteURL:     remoteURL(project.Remote),
		CurrentBranch: currentBranch,
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
