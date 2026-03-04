package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/gitutil"
)

func TestFindGitRootNormalRepo(t *testing.T) {
	// Create a temp directory with a .git directory (normal repo).
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := FindGitRoot(subdir)
	if !ok {
		t.Fatal("expected to find git root")
	}
	if got != root {
		t.Errorf("got %q, want %q", got, root)
	}
}

func TestFindGitRootWorktreeFile(t *testing.T) {
	// Simulate a worktree: parent repo has .git/worktrees/wt, and the
	// worktree directory has a .git file pointing to it.
	parent := t.TempDir()
	wtDir := filepath.Join(parent, ".git", "worktrees", "wt")
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Also create the parent .git dir marker.
	// (The .git directory already exists from MkdirAll above.)

	worktree := t.TempDir()
	gitFile := filepath.Join(worktree, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+wtDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := FindGitRoot(worktree)
	if !ok {
		t.Fatal("expected to find git root")
	}
	if got != parent {
		t.Errorf("got %q, want %q (parent repo root)", got, parent)
	}
}

func TestFindGitRootWorktreeRelativePath(t *testing.T) {
	// Create parent repo with .git/worktrees/wt.
	parent := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parent, ".git", "worktrees", "wt"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Worktree inside parent (e.g. .claude/worktrees/wt) with relative gitdir.
	worktree := filepath.Join(parent, ".claude", "worktrees", "wt")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	// Relative path from worktree dir to parent's .git/worktrees/wt.
	// From parent/.claude/worktrees/wt → parent/.git/worktrees/wt is ../../../.git/worktrees/wt
	relPath := filepath.Join("..", "..", "..", ".git", "worktrees", "wt")
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+relPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := FindGitRoot(worktree)
	if !ok {
		t.Fatal("expected to find git root")
	}
	if got != parent {
		t.Errorf("got %q, want %q (parent repo root)", got, parent)
	}
}

func TestFindGitRootSubmoduleNotResolved(t *testing.T) {
	// Simulate a submodule: .git file pointing to /.git/modules/sub.
	parent := t.TempDir()
	modDir := filepath.Join(parent, ".git", "modules", "sub")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}

	submodule := filepath.Join(parent, "sub")
	if err := os.MkdirAll(submodule, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(submodule, ".git"), []byte("gitdir: "+modDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := FindGitRoot(submodule)
	if !ok {
		t.Fatal("expected to find git root")
	}
	// Submodule .git file should NOT resolve to parent — should return the submodule dir.
	if got != submodule {
		t.Errorf("got %q, want %q (submodule dir itself)", got, submodule)
	}
}

func TestResolveGitWorktreeRoot(t *testing.T) {
	// Non-existent path.
	if _, ok := gitutil.ResolveWorktreeParent("/nonexistent/.git"); ok {
		t.Error("expected false for non-existent path")
	}

	// Regular .git directory.
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := gitutil.ResolveWorktreeParent(gitDir); ok {
		t.Error("expected false for .git directory")
	}

	// .git file without gitdir: prefix.
	gitFile := filepath.Join(t.TempDir(), ".git")
	if err := os.WriteFile(gitFile, []byte("not a gitdir line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := gitutil.ResolveWorktreeParent(gitFile); ok {
		t.Error("expected false for non-gitdir file")
	}
}

func TestListGitWorktrees(t *testing.T) {
	// Non-existent path should return nil.
	paths := ListGitWorktrees("/nonexistent/path")
	if len(paths) != 0 {
		t.Errorf("expected empty list, got %v", paths)
	}
}
