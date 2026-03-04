package gitutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveWorktreeParent reads a .git file and, if it points to a worktree
// directory (pattern: <parent>/.git/worktrees/<name>), returns the parent
// repo root and true. For submodules or regular repos it returns ("", false).
func ResolveWorktreeParent(gitPath string) (string, bool) {
	info, err := os.Lstat(gitPath)
	if err != nil || info.IsDir() {
		return "", false
	}

	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false
	}

	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir:") {
		return "", false
	}

	gitdir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if gitdir == "" {
		return "", false
	}

	// Resolve relative paths against the directory containing .git.
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(filepath.Dir(gitPath), gitdir)
	}
	gitdir = filepath.Clean(gitdir)

	// Worktree pattern: <parent>/.git/worktrees/<name>
	const worktreeSep = string(filepath.Separator) + ".git" + string(filepath.Separator) + "worktrees" + string(filepath.Separator)
	if idx := strings.Index(gitdir, worktreeSep); idx >= 0 {
		return gitdir[:idx], true
	}

	// Submodules use /.git/modules/ — don't resolve those.
	return "", false
}
