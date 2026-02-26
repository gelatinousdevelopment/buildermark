package agent

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FindGitRoot walks up from the given path to find the nearest directory
// containing a .git entry. Returns the git root and true if found.
// The start path does not need to exist on disk; the walk will proceed
// through ancestor directories regardless.
func FindGitRoot(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	path = filepath.Clean(path)
	if path == "." {
		return "", false
	}
	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}
	for {
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// GitRootCache caches git root lookups to avoid repeated filesystem walks.
type GitRootCache struct {
	mu    sync.Mutex
	cache map[string]string
}

// NewGitRootCache returns an empty GitRootCache.
func NewGitRootCache() *GitRootCache {
	return &GitRootCache{cache: make(map[string]string)}
}

// Resolve returns the git root for the given path, falling back to the
// original path if no git root is found. Results are cached.
func (c *GitRootCache) Resolve(path string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if resolved, ok := c.cache[path]; ok {
		return resolved
	}
	resolved := path
	if root, ok := FindGitRoot(path); ok {
		resolved = root
	}
	c.cache[path] = resolved
	return resolved
}
