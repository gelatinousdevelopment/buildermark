package handler

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gelatinousdevelopment/buildermark/web/server/internal/db"
)

type commitRefreshManager struct {
	mu      sync.Mutex
	running map[string]bool
}

func newCommitRefreshManager() *commitRefreshManager {
	return &commitRefreshManager{running: make(map[string]bool)}
}

func (m *commitRefreshManager) tryStart(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running[key] {
		return false
	}
	m.running[key] = true
	return true
}

func (m *commitRefreshManager) finish(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, key)
}

func (s *Server) refreshManager() *commitRefreshManager {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if s.refresher == nil {
		s.refresher = newCommitRefreshManager()
	}
	return s.refresher
}

func (s *Server) enqueueCommitRefresh(repoProject db.Project, group projectGroup, identity gitIdentity, branch string) (bool, string) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	key := repoProject.ID + ":" + branch
	mgr := s.refreshManager()
	if !mgr.tryStart(key) {
		return false, key
	}

	head := ""
	if c, err := latestCommitByIdentity(context.Background(), repoProject.Path, branch, identity); err == nil && c != nil {
		head = c.Hash
	}
	_ = db.UpsertCommitSyncState(context.Background(), s.DB, db.CommitSyncState{
		ProjectID:             repoProject.ID,
		BranchName:            branch,
		State:                 "queued",
		LatestKnownHeadHash:   head,
		LastProcessedHeadHash: "",
		EstimatedTotalCommits: 0,
	})

	go func() {
		defer mgr.finish(key)
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
	_ = db.UpsertCommitSyncState(ctx, s.DB, db.CommitSyncState{
		ProjectID:             repoProject.ID,
		BranchName:            branch,
		State:                 "running",
		LatestKnownHeadHash:   head,
		LastProcessedHeadHash: "",
		LastStartedAtMs:       startedAt,
	})

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
		writeError(w, 400, "project id is required")
		return
	}

	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	project, err := getProjectByID(r.Context(), s.DB, projectID)
	if err != nil {
		log.Printf("error loading project %s: %v", projectID, err)
		writeError(w, 500, "failed to load project")
		return
	}
	if project == nil {
		writeError(w, 404, "project not found")
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
		writeError(w, 500, "failed to list projects")
		return
	}
	group, ok := findProjectGroupByProjectID(groups, project.ID)
	if !ok {
		writeError(w, 404, "project group not found")
		return
	}
	repoProject, err := resolveRepoProject(r.Context(), group)
	if err != nil {
		writeError(w, 404, "repository for project not found")
		return
	}
	identity, err := resolveGitIdentity(r.Context(), repoProject.Path)
	if err != nil {
		writeError(w, 404, "git identity not configured for project")
		return
	}

	queued, jobID := s.enqueueCommitRefresh(*repoProject, group, identity, branch)
	writeSuccess(w, 200, refreshCommitsResponse{
		Queued: queued,
		JobID:  jobID,
		Branch: branch,
	})
}
