package gitmonitor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverRepoStateIncludesDefaultAndWorktreeActiveBranches(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()

	gitRun(t, repo, "init", "-b", "main")
	gitRun(t, repo, "config", "user.name", "Test User")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	mustWriteFile(t, filepath.Join(repo, "app.txt"), "hello\n")
	gitRun(t, repo, "add", "app.txt")
	gitRun(t, repo, "commit", "-m", "initial")

	worktreePath := filepath.Join(t.TempDir(), "feature-worktree")
	gitRun(t, repo, "worktree", "add", "-b", "feature", worktreePath)

	state, err := discoverRepoState(ctx, RepoConfig{
		RepoID:        "proj-1",
		RepoPath:      repo,
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("discoverRepoState: %v", err)
	}

	if _, ok := state.branchHeads["main"]; !ok {
		t.Fatalf("expected main branch to be monitored, got %#v", state.branchHeads)
	}
	if _, ok := state.branchHeads["feature"]; !ok {
		t.Fatalf("expected feature worktree branch to be monitored, got %#v", state.branchHeads)
	}

	commonDir := strings.TrimSpace(gitRun(t, repo, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(repo, commonDir)
	}

	featureGitDir := strings.TrimSpace(gitRun(t, worktreePath, "rev-parse", "--git-dir"))
	if !filepath.IsAbs(featureGitDir) {
		featureGitDir = filepath.Join(worktreePath, featureGitDir)
	}
	wantPaths := []string{
		filepath.Join(featureGitDir, "logs", "HEAD"),
		filepath.Join(commonDir, "logs", "refs", "heads", "main"),
		filepath.Join(commonDir, "logs", "refs", "heads", "feature"),
	}
	for _, wantPath := range wantPaths {
		found := false
		for _, path := range state.watchPaths {
			if filepath.Clean(path) == filepath.Clean(wantPath) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected watch paths to include %q, got %#v", wantPath, state.watchPaths)
		}
	}
}

func TestDiscoverRepoStateDetachedHeadFallsBackToDefaultBranch(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()

	gitRun(t, repo, "init", "-b", "main")
	gitRun(t, repo, "config", "user.name", "Test User")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	mustWriteFile(t, filepath.Join(repo, "app.txt"), "hello\n")
	gitRun(t, repo, "add", "app.txt")
	gitRun(t, repo, "commit", "-m", "initial")
	head := strings.TrimSpace(gitRun(t, repo, "rev-parse", "HEAD"))
	gitRun(t, repo, "checkout", head)

	state, err := discoverRepoState(ctx, RepoConfig{
		RepoID:        "proj-1",
		RepoPath:      repo,
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("discoverRepoState: %v", err)
	}

	if len(state.branchHeads) != 1 {
		t.Fatalf("branchHeads len = %d, want 1 (%#v)", len(state.branchHeads), state.branchHeads)
	}
	if _, ok := state.branchHeads["main"]; !ok {
		t.Fatalf("expected main branch to be monitored, got %#v", state.branchHeads)
	}
}

func gitRun(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
