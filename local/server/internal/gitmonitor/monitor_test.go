package gitmonitor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
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

func TestReconcileSuppressesChangesAfterSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := t.TempDir()
	gitRun(t, repo, "init", "-b", "main")
	gitRun(t, repo, "config", "user.name", "Test User")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	mustWriteFile(t, filepath.Join(repo, "app.txt"), "hello\n")
	gitRun(t, repo, "add", "app.txt")
	gitRun(t, repo, "commit", "-m", "initial")

	var changes []BranchChange
	var mu sync.Mutex

	reconcileInterval := 100 * time.Millisecond

	mon := newRepoMonitor(ctx, RepoConfig{
		RepoID:        "proj-1",
		RepoPath:      repo,
		DefaultBranch: "main",
	}, 50*time.Millisecond, reconcileInterval, func(_ context.Context, change BranchChange) {
		mu.Lock()
		changes = append(changes, change)
		mu.Unlock()
	})

	// Simulate the run loop's reconcile behavior manually.
	// First, do the startup refresh to populate lastHeads.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()

	mon.refresh(watcher, "startup")
	// startup fires onChange for initial population — clear it
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	changes = nil
	mu.Unlock()

	// Make a commit so heads actually differ from lastHeads
	mustWriteFile(t, filepath.Join(repo, "app.txt"), "updated\n")
	gitRun(t, repo, "add", "app.txt")
	gitRun(t, repo, "commit", "-m", "second")

	// Simulate a reconcile that happens after a long sleep gap.
	// Set lastReconcile to long ago to simulate elapsed > 2*interval.
	lastReconcile := time.Now().Add(-5 * reconcileInterval)
	elapsed := time.Since(lastReconcile)
	if elapsed > 2*reconcileInterval {
		// Sleep path: should NOT fire onChange
		mon.refreshSilent(watcher)
	} else {
		t.Fatal("expected elapsed to exceed 2x reconcile interval")
	}

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	silentChanges := len(changes)
	mu.Unlock()

	if silentChanges != 0 {
		t.Fatalf("expected 0 changes after silent refresh, got %d: %+v", silentChanges, changes)
	}

	// Now do a normal reconcile — heads were updated silently, so no changes should fire
	mon.refresh(watcher, "reconcile")
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	normalChanges := len(changes)
	mu.Unlock()

	if normalChanges != 0 {
		t.Fatalf("expected 0 changes after normal reconcile (heads already synced), got %d", normalChanges)
	}

	// Make another commit — now a normal reconcile SHOULD fire onChange
	mustWriteFile(t, filepath.Join(repo, "app.txt"), "third\n")
	gitRun(t, repo, "add", "app.txt")
	gitRun(t, repo, "commit", "-m", "third")

	mon.refresh(watcher, "reconcile")
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	finalChanges := len(changes)
	mu.Unlock()

	if finalChanges != 1 {
		t.Fatalf("expected 1 change after real commit + reconcile, got %d", finalChanges)
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
