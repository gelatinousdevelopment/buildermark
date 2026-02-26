package handler

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func (s *Server) enqueueCommitRefresh(repoProject db.Project, group projectGroup, identity gitIdentity, branch string) (bool, string) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	key := repoProject.ID + ":" + branch
	if !s.refreshJobs.tryStart(key) {
		return false, key
	}

	head := ""
	if c, err := latestCommitByIdentity(context.Background(), repoProject.Path, branch, identity); err == nil && c != nil {
		head = c.Hash
	}
	if err := db.UpsertCommitSyncState(context.Background(), s.DB, db.CommitSyncState{
		ProjectID:             repoProject.ID,
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
		s.runCommitRefresh(repoProject, group, identity, branch)
	}()

	return true, key
}

func (s *Server) runCommitRefresh(repoProject db.Project, group projectGroup, identity gitIdentity, branch string) {
	startedAt := time.Now().UnixMilli()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	head := ""
	if c, err := latestCommitByIdentity(ctx, repoProject.Path, branch, identity); err == nil && c != nil {
		head = c.Hash
	}
	if err := db.UpsertCommitSyncState(ctx, s.DB, db.CommitSyncState{
		ProjectID:             repoProject.ID,
		BranchName:            branch,
		State:                 "running",
		LatestKnownHeadHash:   head,
		LastProcessedHeadHash: "",
		LastStartedAtMs:       startedAt,
	}); err != nil {
		log.Printf("warning: commit sync state upsert (running) failed: %v", err)
	}

	err := IngestDefaultCommits(ctx, s.DB, &repoProject, group, identity, branch)
	if err == nil {
		branchHashes, hashErr := listBranchCommitHashes(ctx, repoProject.Path, branch)
		if hashErr != nil {
			err = hashErr
		} else {
			staleCoverage, staleErr := db.HasStaleCommitCoverageByHashes(ctx, s.DB, repoProject.ID, branchHashes, currentCommitCoverageVersion)
			if staleErr != nil {
				err = staleErr
			} else if staleCoverage {
				_, err = recomputeCommitCoverageForProject(ctx, s.DB, &repoProject, group, branch)
			}
		}
	}

	finishedAt := time.Now().UnixMilli()
	duration := finishedAt - startedAt
	estimatedTotal := 0
	if count, countErr := countBranchCommits(ctx, repoProject.Path, branch); countErr == nil {
		estimatedTotal = count
	}
	if c, latestErr := latestCommitByIdentity(ctx, repoProject.Path, branch, identity); latestErr == nil && c != nil {
		head = c.Hash
	}

	state := db.CommitSyncState{
		ProjectID:             repoProject.ID,
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
		log.Printf("commit refresh failed for project=%s branch=%s: %v", repoProject.ID, branch, err)
		state.State = "failed"
		state.LastError = err.Error()
	} else {
		state.LastError = ""
	}
	if upsertErr := db.UpsertCommitSyncState(context.Background(), s.DB, state); upsertErr != nil {
		log.Printf("commit sync state upsert failed: %v", upsertErr)
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
	identity, err := resolveGitIdentity(r.Context(), repoProject.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "git identity not configured for project")
		return
	}

	queued, jobID := s.enqueueCommitRefresh(*repoProject, group, identity, branch)
	writeSuccess(w, http.StatusOK, refreshCommitsResponse{
		Queued: queued,
		JobID:  jobID,
		Branch: branch,
	})
}
