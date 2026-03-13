package handler

import (
	"context"
	"log"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/gitmonitor"
)

func (s *Server) ReconcileGitRepoMonitor(ctx context.Context) {
	if s == nil || s.RepoMonitor == nil {
		return
	}
	configs, err := s.listGitRepoMonitorConfigs(ctx)
	if err != nil {
		log.Printf("git monitor: reconcile configs failed: %v", err)
		return
	}
	s.RepoMonitor.Reconcile(configs)
}

func (s *Server) reconcileGitRepoMonitorAsync() {
	if s == nil || s.RepoMonitor == nil {
		return
	}
	go s.ReconcileGitRepoMonitor(context.Background())
}

func (s *Server) listGitRepoMonitorConfigs(ctx context.Context) ([]gitmonitor.RepoConfig, error) {
	projects, err := db.ListProjects(ctx, s.DB, false)
	if err != nil {
		return nil, err
	}
	groups := groupProjectsByGitID(projects)
	configs := make([]gitmonitor.RepoConfig, 0, len(groups))
	for _, group := range groups {
		repoProject, err := resolveRepoProject(ctx, group)
		if err != nil || repoProject == nil {
			continue
		}
		defaultBranch := strings.TrimSpace(ensureProjectDefaultBranch(ctx, s.DB, repoProject))
		if defaultBranch == "" {
			defaultBranch = "main"
		}
		configs = append(configs, gitmonitor.RepoConfig{
			RepoID:        repoProject.ID,
			RepoPath:      repoProject.Path,
			DefaultBranch: defaultBranch,
		})
	}
	return configs, nil
}

func (s *Server) HandleGitBranchChange(ctx context.Context, change gitmonitor.BranchChange) {
	if s == nil {
		return
	}
	projectID := strings.TrimSpace(change.RepoID)
	branch := strings.TrimSpace(change.Branch)
	headHash := strings.TrimSpace(change.HeadHash)
	if projectID == "" || branch == "" || headHash == "" {
		return
	}

	project, err := getProjectByID(ctx, s.DB, projectID)
	if err != nil || project == nil {
		return
	}

	groups, err := listAllProjectGroups(ctx, s.DB)
	if err != nil {
		return
	}
	group, ok := findProjectGroupByProjectID(groups, projectID)
	if !ok {
		return
	}
	repoProject, err := resolveRepoProject(ctx, group)
	if err != nil || repoProject == nil {
		return
	}

	baseHash := strings.TrimSpace(change.PreviousHeadHash)
	if baseHash == "" {
		syncState, syncErr := db.GetCommitSyncState(ctx, s.DB, repoProject.ID, branch)
		if syncErr == nil && syncState != nil {
			processedHead := strings.TrimSpace(syncState.LastProcessedHeadHash)
			if processedHead != "" && processedHead != headHash {
				baseHash = processedHead
			}
		}
	}

	if baseHash != "" && baseHash != headHash {
		hashes, hashErr := listCommitRangeHashes(ctx, repoProject.Path, baseHash, headHash)
		if hashErr == nil && len(hashes) > 0 {
			s.enqueueCommitIngestion(repoProject.ID, branch, hashes)
			return
		}
	}

	s.maybeIngestBranchHead(ctx, repoProject, group, branch, headHash)
}
