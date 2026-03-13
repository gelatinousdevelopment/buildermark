package handler

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/gitmonitor"
)

func TestGitMonitorAutoIngestsDefaultBranchCommits(t *testing.T) {
	s := setupTestServer(t)
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")
	mustWriteFile(t, repo+"/app.txt", "hello\n")
	gitRun(t, repo, nil, "add", "app.txt")
	gitRun(t, repo, nil, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))
	initialHead := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.UpdateProjectDefaultBranch(ctx, s.DB, projectID, "main"); err != nil {
		t.Fatalf("UpdateProjectDefaultBranch: %v", err)
	}

	s.RepoMonitor = gitmonitor.New(context.Background(), gitmonitor.Options{
		DebounceInterval:  100 * time.Millisecond,
		ReconcileInterval: time.Hour,
		OnBranchChange:    s.HandleGitBranchChange,
	})
	s.ReconcileGitRepoMonitor(ctx)

	waitForGitMonitor(t, 5*time.Second, func() bool {
		commit, err := db.GetCommitByHash(ctx, s.DB, projectID, "main", initialHead)
		return err == nil && commit != nil
	})

	mustWriteFile(t, repo+"/app.txt", "hello\nworld\n")
	gitRun(t, repo, nil, "add", "app.txt")
	gitRun(t, repo, nil, "commit", "-m", "second")
	newHead := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	waitForGitMonitor(t, 5*time.Second, func() bool {
		commit, err := db.GetCommitByHash(ctx, s.DB, projectID, "main", newHead)
		return err == nil && commit != nil
	})
}

func TestGitMonitorAutoIngestsActiveBranchCommits(t *testing.T) {
	s := setupTestServer(t)
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")
	mustWriteFile(t, repo+"/app.txt", "hello\n")
	gitRun(t, repo, nil, "add", "app.txt")
	gitRun(t, repo, nil, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))
	gitRun(t, repo, nil, "checkout", "-b", "feature")
	initialHead := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.UpdateProjectDefaultBranch(ctx, s.DB, projectID, "main"); err != nil {
		t.Fatalf("UpdateProjectDefaultBranch: %v", err)
	}

	s.RepoMonitor = gitmonitor.New(context.Background(), gitmonitor.Options{
		DebounceInterval:  100 * time.Millisecond,
		ReconcileInterval: time.Hour,
		OnBranchChange:    s.HandleGitBranchChange,
	})
	s.ReconcileGitRepoMonitor(ctx)

	waitForGitMonitor(t, 5*time.Second, func() bool {
		mainCommit, mainErr := db.GetCommitByHash(ctx, s.DB, projectID, "main", initialHead)
		featureCommit, featureErr := db.GetCommitByHash(ctx, s.DB, projectID, "feature", initialHead)
		return (mainErr == nil && mainCommit != nil) || (featureErr == nil && featureCommit != nil)
	})

	mustWriteFile(t, repo+"/feature.txt", "feature\n")
	gitRun(t, repo, nil, "add", "feature.txt")
	gitRun(t, repo, nil, "commit", "-m", "feature work")
	featureHead := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	waitForGitMonitor(t, 5*time.Second, func() bool {
		commit, err := db.GetCommitByHash(ctx, s.DB, projectID, "feature", featureHead)
		return err == nil && commit != nil
	})
}

func TestHandleGitBranchChangeFastForwardMergeMovesMergedCommitsToDestinationBranch(t *testing.T) {
	s := setupTestServer(t)
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")
	mustWriteFile(t, filepath.Join(repo, "app.txt"), "hello\n")
	gitRun(t, repo, nil, "add", "app.txt")
	gitRun(t, repo, nil, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.UpdateProjectDefaultBranch(ctx, s.DB, projectID, "main"); err != nil {
		t.Fatalf("UpdateProjectDefaultBranch: %v", err)
	}

	initialHead := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))
	gitRun(t, repo, nil, "checkout", "-b", "feature")
	mustWriteFile(t, filepath.Join(repo, "feature.txt"), "one\n")
	gitRun(t, repo, nil, "add", "feature.txt")
	gitRun(t, repo, nil, "commit", "-m", "feature 1")
	featureOne := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	mustWriteFile(t, filepath.Join(repo, "feature.txt"), "one\ntwo\n")
	gitRun(t, repo, nil, "add", "feature.txt")
	gitRun(t, repo, nil, "commit", "-m", "feature 2")
	featureTwo := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	s.HandleGitBranchChange(ctx, gitmonitor.BranchChange{
		RepoID:           projectID,
		RepoPath:         repo,
		Branch:           "feature",
		PreviousHeadHash: initialHead,
		HeadHash:         featureTwo,
		Reason:           "test",
	})

	waitForGitMonitor(t, 5*time.Second, func() bool {
		first, firstErr := db.GetCommitByHash(ctx, s.DB, projectID, "feature", featureOne)
		second, secondErr := db.GetCommitByHash(ctx, s.DB, projectID, "feature", featureTwo)
		return firstErr == nil && secondErr == nil && first != nil && second != nil
	})

	gitRun(t, repo, nil, "checkout", "main")
	gitRun(t, repo, nil, "merge", "feature")

	s.HandleGitBranchChange(ctx, gitmonitor.BranchChange{
		RepoID:           projectID,
		RepoPath:         repo,
		Branch:           "main",
		PreviousHeadHash: initialHead,
		HeadHash:         featureTwo,
		Reason:           "test",
	})

	waitForGitMonitor(t, 5*time.Second, func() bool {
		first, firstErr := db.GetCommitByHash(ctx, s.DB, projectID, "main", featureOne)
		second, secondErr := db.GetCommitByHash(ctx, s.DB, projectID, "main", featureTwo)
		return firstErr == nil && secondErr == nil && first != nil && second != nil
	})
}

func TestHandleGitBranchChangeNoFFMergeMovesMergedCommitsToDestinationBranch(t *testing.T) {
	s := setupTestServer(t)
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")
	mustWriteFile(t, filepath.Join(repo, "main.txt"), "main\n")
	gitRun(t, repo, nil, "add", "main.txt")
	gitRun(t, repo, nil, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.UpdateProjectDefaultBranch(ctx, s.DB, projectID, "main"); err != nil {
		t.Fatalf("UpdateProjectDefaultBranch: %v", err)
	}

	initialHead := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))
	gitRun(t, repo, nil, "checkout", "-b", "feature")
	mustWriteFile(t, filepath.Join(repo, "feature.txt"), "one\n")
	gitRun(t, repo, nil, "add", "feature.txt")
	gitRun(t, repo, nil, "commit", "-m", "feature 1")
	featureOne := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	mustWriteFile(t, filepath.Join(repo, "feature.txt"), "one\ntwo\n")
	gitRun(t, repo, nil, "add", "feature.txt")
	gitRun(t, repo, nil, "commit", "-m", "feature 2")
	featureTwo := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	s.HandleGitBranchChange(ctx, gitmonitor.BranchChange{
		RepoID:           projectID,
		RepoPath:         repo,
		Branch:           "feature",
		PreviousHeadHash: initialHead,
		HeadHash:         featureTwo,
		Reason:           "test",
	})

	waitForGitMonitor(t, 5*time.Second, func() bool {
		first, firstErr := db.GetCommitByHash(ctx, s.DB, projectID, "feature", featureOne)
		second, secondErr := db.GetCommitByHash(ctx, s.DB, projectID, "feature", featureTwo)
		return firstErr == nil && secondErr == nil && first != nil && second != nil
	})

	gitRun(t, repo, nil, "checkout", "main")
	mustWriteFile(t, filepath.Join(repo, "main.txt"), "main\nnext\n")
	gitRun(t, repo, nil, "add", "main.txt")
	gitRun(t, repo, nil, "commit", "-m", "main 2")
	mainTwo := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	gitRun(t, repo, nil, "merge", "--no-ff", "feature", "-m", "merge feature")
	mergeHead := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	s.HandleGitBranchChange(ctx, gitmonitor.BranchChange{
		RepoID:           projectID,
		RepoPath:         repo,
		Branch:           "main",
		PreviousHeadHash: initialHead,
		HeadHash:         mergeHead,
		Reason:           "test",
	})

	waitForGitMonitor(t, 5*time.Second, func() bool {
		first, firstErr := db.GetCommitByHash(ctx, s.DB, projectID, "main", featureOne)
		second, secondErr := db.GetCommitByHash(ctx, s.DB, projectID, "main", featureTwo)
		mainCommit, mainErr := db.GetCommitByHash(ctx, s.DB, projectID, "main", mainTwo)
		mergeCommit, mergeErr := db.GetCommitByHash(ctx, s.DB, projectID, "main", mergeHead)
		return firstErr == nil && secondErr == nil && mainErr == nil && mergeErr == nil && first != nil && second != nil && mainCommit != nil && mergeCommit != nil
	})
}

func TestHandleGitBranchChangeUsesCommitSyncStateWhenPreviousHeadMissing(t *testing.T) {
	s := setupTestServer(t)
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")
	mustWriteFile(t, filepath.Join(repo, "app.txt"), "hello\n")
	gitRun(t, repo, nil, "add", "app.txt")
	gitRun(t, repo, nil, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))
	initialHead := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.UpdateProjectDefaultBranch(ctx, s.DB, projectID, "main"); err != nil {
		t.Fatalf("UpdateProjectDefaultBranch: %v", err)
	}

	gitRun(t, repo, nil, "checkout", "-b", "feature")
	mustWriteFile(t, filepath.Join(repo, "feature.txt"), "one\n")
	gitRun(t, repo, nil, "add", "feature.txt")
	gitRun(t, repo, nil, "commit", "-m", "feature 1")
	featureOne := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	gitRun(t, repo, nil, "checkout", "main")
	gitRun(t, repo, nil, "merge", "feature")
	mainHead := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	if err := db.UpsertCommitSyncState(ctx, s.DB, db.CommitSyncState{
		ProjectID:             projectID,
		BranchName:            "main",
		State:                 "idle",
		LatestKnownHeadHash:   mainHead,
		LastProcessedHeadHash: initialHead,
	}); err != nil {
		t.Fatalf("UpsertCommitSyncState: %v", err)
	}

	s.HandleGitBranchChange(ctx, gitmonitor.BranchChange{
		RepoID:   projectID,
		RepoPath: repo,
		Branch:   "main",
		HeadHash: mainHead,
		Reason:   "startup",
	})

	waitForGitMonitor(t, 5*time.Second, func() bool {
		commit, err := db.GetCommitByHash(ctx, s.DB, projectID, "main", featureOne)
		return err == nil && commit != nil
	})
}

func waitForGitMonitor(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
