package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	recomputedBranches, _, err := s.recomputeProjectCoverageAllBranchesWithChangedPatterns(ctx, projectID, nil, nil)
	return recomputedBranches, err
}

func (s *Server) recomputeProjectCoverageAllBranchesWithChangedPatterns(
	ctx context.Context,
	projectID string,
	changedPatterns []string,
	progress func(string),
) (int, int, error) {
	project, err := getProjectByID(ctx, s.DB, projectID)
	if err != nil {
		return 0, 0, err
	}
	if project == nil {
		return 0, 0, db.ErrNotFound
	}

	groups, err := listAllProjectGroups(ctx, s.DB)
	if err != nil {
		return 0, 0, err
	}
	group, ok := findProjectGroupByProjectID(groups, project.ID)
	if !ok {
		return 0, 0, nil
	}
	repoProject, err := resolveRepoProject(ctx, group)
	if err != nil {
		return 0, 0, nil
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

	recomputedBranches := 0
	recomputedCommits := 0
	for _, branch := range branchList {
		if progress != nil {
			progress(fmt.Sprintf("Recomputing branch %s...", branch))
		}
		n, err := recomputeCommitCoverageForProjectWithChangedPatterns(ctx, s.DB, repoProject, group, branch, changedPatterns)
		if err != nil {
			log.Printf("warning: recompute commit coverage failed for project=%s branch=%s: %v", projectID, branch, err)
			continue
		}
		if n == 0 {
			if progress != nil {
				progress(fmt.Sprintf("Branch %s has no matching commits", branch))
			}
			continue
		}
		recomputedBranches++
		recomputedCommits += n
		if progress != nil {
			progress(fmt.Sprintf("Branch %s recomputed %d commit(s)", branch, n))
		}
	}
	return recomputedBranches, recomputedCommits, nil
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

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	if err := db.DeleteProject(r.Context(), s.DB, id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		log.Printf("error deleting project: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete project")
		return
	}

	writeSuccess(w, http.StatusOK, nil)
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

	project, err := getProjectByID(r.Context(), s.DB, id)
	if err != nil {
		log.Printf("error loading project for ignore_diff_paths update: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update project")
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	changed := project.IgnoreDiffPaths != body.IgnoreDiffPaths
	changedPatterns, changedPatternErr := s.changedEffectiveIgnorePatternsForProject(
		r.Context(),
		id,
		project.IgnoreDiffPaths,
		project.IgnoreDefaultDiffPaths,
		body.IgnoreDiffPaths,
		project.IgnoreDefaultDiffPaths,
	)
	if changedPatternErr != nil {
		log.Printf("warning: could not compute changed effective ignore patterns for project %s: %v", id, changedPatternErr)
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

	if changed {
		s.enqueueProjectCoverageRecompute(id, "ignore_diff_paths_changed", changedPatterns)
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

	project, err := getProjectByID(r.Context(), s.DB, id)
	if err != nil {
		log.Printf("error loading project for ignore_default_diff_paths update: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update project")
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	changed := project.IgnoreDefaultDiffPaths != body.IgnoreDefaultDiffPaths
	changedPatterns, changedPatternErr := s.changedEffectiveIgnorePatternsForProject(
		r.Context(),
		id,
		project.IgnoreDiffPaths,
		project.IgnoreDefaultDiffPaths,
		project.IgnoreDiffPaths,
		body.IgnoreDefaultDiffPaths,
	)
	if changedPatternErr != nil {
		log.Printf("warning: could not compute changed effective ignore patterns for project %s: %v", id, changedPatternErr)
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

	if changed {
		s.enqueueProjectCoverageRecompute(id, "ignore_default_diff_paths_changed", changedPatterns)
	}

	writeSuccess(w, http.StatusOK, nil)
}

func (s *Server) tryStartProjectCoverageRecompute(projectID string) bool {
	s.coverageRecomputeMu.Lock()
	defer s.coverageRecomputeMu.Unlock()
	if s.coverageRecomputeRunning == nil {
		s.coverageRecomputeRunning = make(map[string]bool)
	}
	if s.coverageRecomputeRunning[projectID] {
		return false
	}
	s.coverageRecomputeRunning[projectID] = true
	return true
}

func (s *Server) finishProjectCoverageRecompute(projectID string) {
	s.coverageRecomputeMu.Lock()
	defer s.coverageRecomputeMu.Unlock()
	if s.coverageRecomputeRunning == nil {
		return
	}
	delete(s.coverageRecomputeRunning, projectID)
}

func (s *Server) enqueueProjectCoverageRecompute(projectID, reason string, changedPatterns []string) {
	if len(changedPatterns) == 0 {
		log.Printf("project %s settings changed (%s); no effective ignore-pattern changes, skipping recompute", projectID, reason)
		return
	}
	if !s.tryStartProjectCoverageRecompute(projectID) {
		log.Printf("project %s settings changed (%s); coverage recompute already in progress", projectID, reason)
		return
	}

	go func() {
		defer s.finishProjectCoverageRecompute(projectID)

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()

		status := func(state, message string, recomputedCommits int) {
			if s.ws == nil {
				return
			}
			s.ws.broadcastEvent("import_status", importStatusEvent{
				State:            state,
				Message:          message,
				ProjectsImported: 0,
				EntriesProcessed: 0,
				CommitsIngested:  recomputedCommits,
			})
		}

		preview := changedPatterns[0]
		if len(changedPatterns) > 1 {
			preview = fmt.Sprintf("%s (+%d more)", changedPatterns[0], len(changedPatterns)-1)
		}
		status("running", fmt.Sprintf("Recomputing diffs for project %s: %s", projectID, preview), 0)
		lastProgress := time.Time{}
		progress := func(message string) {
			now := time.Now()
			if now.Sub(lastProgress) < 50*time.Millisecond {
				return
			}
			lastProgress = now
			status("running", message, 0)
		}

		recomputedBranches, recomputedCommits, err := s.recomputeProjectCoverageAllBranchesWithChangedPatterns(ctx, projectID, changedPatterns, progress)
		if err != nil {
			status("error", fmt.Sprintf("Diff recompute failed for project %s", projectID), recomputedCommits)
			log.Printf("project %s settings changed (%s); coverage recompute failed: %v", projectID, reason, err)
		} else {
			status("complete", fmt.Sprintf("Recomputed %d commit(s) across %d branch(es) for project %s", recomputedCommits, recomputedBranches, projectID), recomputedCommits)
			log.Printf("project %s settings changed (%s); recomputed coverage on %d branch(es), %d commits", projectID, reason, recomputedBranches, recomputedCommits)
		}
	}()
}

func (s *Server) changedEffectiveIgnorePatternsForProject(
	ctx context.Context,
	projectID string,
	oldIgnoreDiffPaths string,
	oldIgnoreDefault bool,
	newIgnoreDiffPaths string,
	newIgnoreDefault bool,
) ([]string, error) {
	groups, err := listAllProjectGroups(ctx, s.DB)
	if err != nil {
		return nil, err
	}
	group, ok := findProjectGroupByProjectID(groups, projectID)
	if !ok {
		oldSet := make(map[string]struct{})
		newSet := make(map[string]struct{})
		for _, p := range splitIgnoreDiffPatterns(oldIgnoreDiffPaths) {
			oldSet[p] = struct{}{}
		}
		if oldIgnoreDefault {
			for _, p := range DefaultIgnoreDiffPaths {
				oldSet[p] = struct{}{}
			}
		}
		for _, p := range splitIgnoreDiffPatterns(newIgnoreDiffPaths) {
			newSet[p] = struct{}{}
		}
		if newIgnoreDefault {
			for _, p := range DefaultIgnoreDiffPaths {
				newSet[p] = struct{}{}
			}
		}
		return symmetricPatternDiff(oldSet, newSet), nil
	}

	oldGroup := cloneProjectGroupWithOverride(group, projectID, oldIgnoreDiffPaths, oldIgnoreDefault)
	newGroup := cloneProjectGroupWithOverride(group, projectID, newIgnoreDiffPaths, newIgnoreDefault)

	oldSet := make(map[string]struct{})
	newSet := make(map[string]struct{})
	for _, p := range groupIgnoreDiffPatterns(oldGroup) {
		oldSet[p] = struct{}{}
	}
	for _, p := range groupIgnoreDiffPatterns(newGroup) {
		newSet[p] = struct{}{}
	}
	return symmetricPatternDiff(oldSet, newSet), nil
}

func cloneProjectGroupWithOverride(group projectGroup, projectID, ignoreDiffPaths string, ignoreDefault bool) projectGroup {
	cloned := projectGroup{
		GitID:    group.GitID,
		Projects: make([]db.Project, len(group.Projects)),
	}
	copy(cloned.Projects, group.Projects)
	for i := range cloned.Projects {
		if cloned.Projects[i].ID != projectID {
			continue
		}
		cloned.Projects[i].IgnoreDiffPaths = ignoreDiffPaths
		cloned.Projects[i].IgnoreDefaultDiffPaths = ignoreDefault
		break
	}
	return cloned
}

func symmetricPatternDiff(a, b map[string]struct{}) []string {
	out := make([]string, 0, len(a)+len(b))
	for p := range a {
		if _, ok := b[p]; ok {
			continue
		}
		out = append(out, p)
	}
	for p := range b {
		if _, ok := a[p]; ok {
			continue
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
