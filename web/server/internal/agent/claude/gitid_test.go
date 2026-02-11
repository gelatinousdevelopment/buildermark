package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveGitID(t *testing.T) {
	dir := t.TempDir()

	// Initialize a git repo and make a commit.
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	gitID := resolveGitID(dir)
	if gitID == "" {
		t.Fatal("expected non-empty git ID")
	}
	if len(gitID) != 40 {
		t.Errorf("expected 40-char hash, got %d chars: %q", len(gitID), gitID)
	}
}

func TestResolveGitIDNonGitDir(t *testing.T) {
	dir := t.TempDir()

	gitID := resolveGitID(dir)
	if gitID != "" {
		t.Errorf("expected empty git ID for non-git dir, got %q", gitID)
	}
}

func TestResolveGitIDNonexistentPath(t *testing.T) {
	gitID := resolveGitID(filepath.Join(t.TempDir(), "nonexistent"))
	if gitID != "" {
		t.Errorf("expected empty git ID for nonexistent path, got %q", gitID)
	}
}
