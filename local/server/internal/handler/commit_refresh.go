package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// RefreshStaleProjects checks all projects for stale commit coverage and
// enqueues a refresh for each stale project/branch combination.
func (s *Server) RefreshStaleProjects(ctx context.Context) {
	groups, err := listAllProjectGroups(ctx, s.DB)
	if err != nil {
		log.Printf("startup refresh: failed to list project groups: %v", err)
		return
	}
	for _, group := range groups {
		repoProject, err := resolveRepoProject(ctx, group)
		if err != nil {
			continue
		}
		branches, err := db.ListDistinctBranches(ctx, s.DB, repoProject.ID)
		if err != nil {
			log.Printf("startup refresh: failed to list branches for project %s: %v", repoProject.ID, err)
			continue
		}
		for _, branch := range branches {
			stale, err := db.HasStaleCommitCoverageByBranch(ctx, s.DB, repoProject.ID, branch, currentCommitCoverageVersion)
			if err != nil {
				log.Printf("startup refresh: stale check failed for project %s branch %s: %v", repoProject.ID, branch, err)
				continue
			}
			if !stale {
				continue
			}
			if queued, _ := s.enqueueCommitRefresh(repoProject.ID, branch); queued {
				log.Printf("startup refresh: queued stale commit refresh for project %s branch %s", repoProject.ID, branch)
			}
		}
	}
}

func (s *Server) enqueueCommitRefresh(projectID, branch string) (bool, string) {
	return s.enqueueCommitRefreshWithDays(projectID, branch, 0, false)
}

func (s *Server) enqueueCommitRefreshWithDays(projectID, branch string, days int, manual bool) (bool, string) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	key := projectID + ":" + branch
	if !s.refreshJobs.tryStart(key) {
		return false, key
	}

	covCtx, err := s.loadProjectCoverageContext(context.Background(), projectID)
	if err != nil {
		s.refreshJobs.finish(key)
		log.Printf("warning: commit refresh enqueue context load failed for %s: %v", projectID, err)
		return false, key
	}

	head := ""
	if c, err := latestCommitByIdentity(context.Background(), covCtx.repoProject.Path, branch, covCtx.identity); err == nil && c != nil {
		head = c.Hash
	}
	if err := db.UpsertCommitSyncState(context.Background(), s.DB, db.CommitSyncState{
		ProjectID:             projectID,
		BranchName:            branch,
		State:                 "queued",
		LatestKnownHeadHash:   head,
		LastProcessedHeadHash: "",
		EstimatedTotalCommits: 0,
	}); err != nil {
		log.Printf("warning: commit sync state upsert (queued) failed: %v", err)
	}

	go func() {
		defer s.refreshJobs.finish(key)
		s.runCommitRefresh(projectID, branch, days, manual)
	}()

	return true, key
}

func (s *Server) broadcastRefreshStatus(state, message, projectID, branch string) {
	if s.ws != nil {
		s.ws.broadcastEvent("job_status", jobStatusEvent{
			JobType:   "commit_refresh",
			State:     state,
			Message:   message,
			ProjectID: projectID,
			Branch:    branch,
		})
	}
}

func (s *Server) runCommitRefresh(projectID, branch string, days int, manual bool) {
	startedAt := time.Now().UnixMilli()
	timeout := 2 * time.Minute
	if days > 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	s.broadcastRefreshStatus("running", "Refreshing commits...", projectID, branch)

	covCtx, err := s.loadProjectCoverageContext(ctx, projectID)
	if err != nil {
		log.Printf("commit refresh context load failed for project=%s branch=%s: %v", projectID, branch, err)
		s.broadcastRefreshStatus("error", fmt.Sprintf("Refresh failed: %v", err), projectID, branch)
		return
	}

	head := ""
	if c, err := latestCommitByIdentity(ctx, covCtx.repoProject.Path, branch, covCtx.identity); err == nil && c != nil {
		head = c.Hash
	}
	if err := db.UpsertCommitSyncState(ctx, s.DB, db.CommitSyncState{
		ProjectID:             projectID,
		BranchName:            branch,
		State:                 "running",
		LatestKnownHeadHash:   head,
		LastProcessedHeadHash: "",
		LastStartedAtMs:       startedAt,
	}); err != nil {
		log.Printf("warning: commit sync state upsert (running) failed: %v", err)
	}

	if days > 0 {
		// Extended refresh: ingest all commits within the day window.
		since := time.Now().AddDate(0, 0, -days)
		includeAll := days >= 36500 // "all" sentinel

		// If the repo is a shallow clone, fetch more history so git log can see it.
		if shallow := shallowBoundaryHashes(ctx, covCtx.repoProject.Path); shallow != nil {
			var fetchArgs []string
			if includeAll {
				s.broadcastRefreshStatus("running", "Fetching full history...", projectID, branch)
				fetchArgs = []string{"-C", covCtx.repoProject.Path, "fetch", "--unshallow", "origin", branch}
			} else {
				sinceStr := since.Format("2006-01-02")
				s.broadcastRefreshStatus("running", fmt.Sprintf("Fetching history since %s...", sinceStr), projectID, branch)
				fetchArgs = []string{"-C", covCtx.repoProject.Path, "fetch", fmt.Sprintf("--shallow-since=%s", sinceStr), "origin", branch}
			}
			cmd := exec.CommandContext(ctx, "git", fetchArgs...)
			if output, fetchErr := cmd.CombinedOutput(); fetchErr != nil {
				log.Printf("warning: shallow fetch failed for %s (continuing with local history): %v (output: %s)", covCtx.repoProject.Path, fetchErr, string(output))
			}
		}

		dayLabel := "days"
		if days == 1 {
			dayLabel = "day"
		}
		s.broadcastRefreshStatus("running", fmt.Sprintf("Ingesting commits for last %d %s...", days, dayLabel), projectID, branch)
		var ingestedCommits []db.Commit
		_, err = s.runStableCoverageStage(
			ctx,
			projectID,
			"refresh_ingest",
			func() {
				s.broadcastRefreshStatus("running", "Project diff settings changed during refresh; re-running with latest settings...", projectID, branch)
			},
			func(stageCtx *projectCoverageContext) error {
				n, ingestErr := IngestCommitsForWindow(
					ctx,
					s.DB,
					stageCtx.repoProject,
					stageCtx.group,
					branch,
					since,
					includeAll,
					&stageCtx.identity,
					stageCtx.extraEmails,
					func(c []db.Commit) { ingestedCommits = append(ingestedCommits, c...) },
				)
				if ingestErr != nil {
					return ingestErr
				}
				s.broadcastRefreshStatus("running", fmt.Sprintf("Ingested %d commit(s). Checking for shallow commits...", n), projectID, branch)
				s.deepenNeedsParentCommits(ctx, stageCtx.repoProject, stageCtx.group, branch, &stageCtx.identity, stageCtx.extraEmails)
				return nil
			},
		)
		if err == nil && len(ingestedCommits) > 0 {
			localCommits := newLocalUserAuthorFilter(covCtx.identity, covCtx.extraEmails).FilterCommits(ingestedCommits)
			s.notifyIngestedCommits(localCommits, db.RepoLabel(covCtx.repoProject.Path))
		}
	} else {
		var ingestedCommits []db.Commit
		_, err = s.runStableCoverageStage(
			ctx,
			projectID,
			"refresh_ingest",
			func() {
				s.broadcastRefreshStatus("running", "Project diff settings changed during refresh; re-running with latest settings...", projectID, branch)
			},
			func(stageCtx *projectCoverageContext) error {
				return IngestDefaultCommits(ctx, s.DB, stageCtx.repoProject, stageCtx.group, stageCtx.identity, stageCtx.extraEmails, branch,
					func(c []db.Commit) { ingestedCommits = append(ingestedCommits, c...) },
				)
			},
		)
		if err == nil && len(ingestedCommits) > 0 {
			localCommits := newLocalUserAuthorFilter(covCtx.identity, covCtx.extraEmails).FilterCommits(ingestedCommits)
			s.notifyIngestedCommits(localCommits, db.RepoLabel(covCtx.repoProject.Path))
		}
	}

	if err == nil {
		// Only reset coverage_version during manual refresh (days > 0).
		// Auto-refresh (days == 0) should only recompute already-stale commits,
		// not force-reset all commits on the branch.
		if days > 0 {
			sinceSec := time.Now().AddDate(0, 0, -days).Unix()
			if reset, resetErr := db.ResetCoverageVersionByDateRange(ctx, s.DB, projectID, branch, sinceSec); resetErr != nil {
				log.Printf("warning: failed to reset coverage version: %v", resetErr)
			} else if reset > 0 {
				log.Printf("reset coverage_version for %d commits in refresh window", reset)
			}
		}

		s.broadcastRefreshStatus("running", "Checking commit coverage...", projectID, branch)
		_, err = s.runStableCoverageStage(
			ctx,
			projectID,
			"refresh_recompute",
			func() {
				s.broadcastRefreshStatus("running", "Project diff settings changed during coverage recompute; re-running with latest settings...", projectID, branch)
			},
			func(stageCtx *projectCoverageContext) error {
				branchHashes, hashErr := listBranchCommitHashes(ctx, stageCtx.repoProject.Path, branch)
				if hashErr != nil {
					return hashErr
				}
				staleCoverage, staleErr := db.HasStaleCommitCoverageByHashes(ctx, s.DB, stageCtx.repoProject.ID, branchHashes, currentCommitCoverageVersion)
				if staleErr != nil || !staleCoverage {
					return staleErr
				}
				s.broadcastRefreshStatus("running", "Recomputing commit coverage...", projectID, branch)
				_, recomputeErr := recomputeCommitCoverageForProjectWithChangedPatterns(
					ctx,
					s.DB,
					stageCtx.repoProject,
					stageCtx.group,
					branch,
					"",
					nil,
					&stageCtx.identity,
					stageCtx.extraEmails,
					func(message string, processed int) {
						s.broadcastRefreshStatus("running", message, projectID, branch)
					},
					0,
				)
				return recomputeErr
			},
		)
	}

	finishedAt := time.Now().UnixMilli()
	duration := finishedAt - startedAt
	estimatedTotal := 0
	finalCtx, loadErr := s.loadProjectCoverageContext(ctx, projectID)
	if loadErr == nil {
		covCtx = finalCtx
	}
	if count, countErr := countBranchCommits(ctx, covCtx.repoProject.Path, branch); countErr == nil {
		estimatedTotal = count
	}
	if c, latestErr := latestCommitByIdentity(ctx, covCtx.repoProject.Path, branch, covCtx.identity); latestErr == nil && c != nil {
		head = c.Hash
	}

	state := db.CommitSyncState{
		ProjectID:             projectID,
		BranchName:            branch,
		State:                 "idle",
		LatestKnownHeadHash:   head,
		LastProcessedHeadHash: head,
		EstimatedTotalCommits: estimatedTotal,
		LastStartedAtMs:       startedAt,
		LastFinishedAtMs:      finishedAt,
		LastDurationMs:        duration,
	}
	if err != nil {
		log.Printf("commit refresh failed for project=%s branch=%s: %v", projectID, branch, err)
		state.State = "failed"
		state.LastError = err.Error()
		s.broadcastRefreshStatus("error", fmt.Sprintf("Refresh failed: %v", err), projectID, branch)
	} else {
		state.LastError = ""
		s.broadcastRefreshStatus("complete", fmt.Sprintf("Refresh complete (%.1fs).", float64(duration)/1000), projectID, branch)
		if manual {
			label := db.RepoLabel(covCtx.repoProject.Path)
			s.sendNotification("commit_refresh_complete", "Commit refresh complete",
				fmt.Sprintf("Refresh complete for %s", label), "")
		}
	}
	if upsertErr := db.UpsertCommitSyncState(context.Background(), s.DB, state); upsertErr != nil {
		log.Printf("commit sync state upsert failed: %v", upsertErr)
	}
}

// deepenNeedsParentCommits finds all commits with needs_parent=1 for a project,
// attempts to deepen the clone for each, and re-ingests any that are resolved.
func (s *Server) deepenNeedsParentCommits(
	ctx context.Context,
	repoProject *db.Project,
	group projectGroup,
	branch string,
	identity *gitIdentity,
	extraEmails []string,
) {
	hashes, err := db.ListNeedsParentCommitHashes(ctx, s.DB, repoProject.ID)
	if err != nil || len(hashes) == 0 {
		return
	}

	if s.ws != nil {
		s.ws.broadcastEvent("job_status", jobStatusEvent{
			JobType:   "commit_deepen",
			State:     "running",
			Message:   fmt.Sprintf("Deepening %d shallow commit(s)...", len(hashes)),
			ProjectID: repoProject.ID,
			Branch:    branch,
		})
	}

	// Try deepening incrementally until no more progress is made.
	maxAttempts := 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Check which hashes are still at the shallow boundary.
		shallow := shallowBoundaryHashes(ctx, repoProject.Path)
		if shallow == nil {
			break // not a shallow repo anymore
		}

		var stillShallow []string
		for _, h := range hashes {
			if shallow[h] {
				stillShallow = append(stillShallow, h)
			}
		}
		if len(stillShallow) == 0 {
			break
		}

		// Deepen by 2 for each attempt.
		cmd := exec.CommandContext(ctx, "git", "-C", repoProject.Path, "fetch", "--deepen=2", "origin", branch)
		output, fetchErr := cmd.CombinedOutput()
		if fetchErr != nil {
			log.Printf("git fetch --deepen=2 failed for %s: %v (output: %s)", repoProject.Path, fetchErr, string(output))
			break
		}

		// Check if any became resolved.
		newShallow := shallowBoundaryHashes(ctx, repoProject.Path)
		resolved := false
		for _, h := range stillShallow {
			if newShallow == nil || !newShallow[h] {
				resolved = true
				break
			}
		}
		if !resolved {
			break // no progress made
		}

		hashes = stillShallow // narrow to still-shallow for next iteration
	}

	// Re-ingest all formerly needs_parent commits that are now resolved.
	allNeeds, _ := db.ListNeedsParentCommitHashes(ctx, s.DB, repoProject.ID)
	if len(allNeeds) == 0 {
		return
	}
	shallow := shallowBoundaryHashes(ctx, repoProject.Path)
	var resolved []string
	for _, h := range allNeeds {
		if shallow == nil || !shallow[h] {
			resolved = append(resolved, h)
		}
	}
	if len(resolved) > 0 {
		if s.ws != nil {
			s.ws.broadcastEvent("job_status", jobStatusEvent{
				JobType:   "commit_deepen",
				State:     "running",
				Message:   fmt.Sprintf("Re-ingesting %d resolved commit(s)...", len(resolved)),
				ProjectID: repoProject.ID,
				Branch:    branch,
			})
		}
		if _, err := ingestMissingCommits(ctx, s.DB, repoProject, group, branch, resolved, identity, extraEmails, nil); err != nil {
			log.Printf("warning: re-ingesting resolved shallow commits: %v", err)
		}
	}

	if s.ws != nil {
		s.ws.broadcastEvent("job_status", jobStatusEvent{
			JobType:   "commit_deepen",
			State:     "complete",
			Message:   fmt.Sprintf("Deepened %d of %d shallow commit(s)", len(resolved), len(allNeeds)),
			ProjectID: repoProject.ID,
			Branch:    branch,
		})
	}
}

type refreshCommitsResponse struct {
	Queued bool   `json:"queued"`
	JobID  string `json:"jobId"`
	Branch string `json:"branch"`
}

func (s *Server) handleRefreshProjectCommits(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	project, err := getProjectByID(r.Context(), s.DB, projectID)
	if err != nil {
		log.Printf("error loading project %s: %v", projectID, err)
		writeError(w, http.StatusInternalServerError, "failed to load project")
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if branch == "" {
		branch = strings.TrimSpace(project.DefaultBranch)
		if branch == "" {
			branch = "main"
		}
	}

	groups, err := listAllProjectGroups(r.Context(), s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}
	group, ok := findProjectGroupByProjectID(groups, project.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "project group not found")
		return
	}
	repoProject, err := resolveRepoProject(r.Context(), group)
	if err != nil {
		writeError(w, http.StatusNotFound, "repository for project not found")
		return
	}
	if _, err := resolveGitIdentity(r.Context(), repoProject.Path); err != nil {
		writeError(w, http.StatusNotFound, "git identity not configured for project")
		return
	}

	days := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		if v, parseErr := strconv.Atoi(raw); parseErr == nil && v > 0 {
			days = v
		}
	}

	queued, jobID := s.enqueueCommitRefreshWithDays(project.ID, branch, days, true)
	writeSuccess(w, http.StatusOK, refreshCommitsResponse{
		Queued: queued,
		JobID:  jobID,
		Branch: branch,
	})
}
