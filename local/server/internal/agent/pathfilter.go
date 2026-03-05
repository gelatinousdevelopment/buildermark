package agent

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// PathFilter matches project paths against a set of tracked paths.
// A nil PathFilter matches everything.
type PathFilter map[string]struct{}

// NewPathFilter creates a PathFilter from a list of paths. Returns nil if no
// valid paths are provided (which means "match everything").
func NewPathFilter(paths []string) PathFilter {
	out := make(PathFilter)
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		if p == "." {
			continue
		}
		out[p] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Match returns true if the given project path is tracked by this filter.
// A nil filter matches everything.
func (f PathFilter) Match(projectPath string) bool {
	if f == nil {
		return true
	}
	if len(f) == 0 {
		return false
	}
	projectPath = strings.TrimSpace(filepath.Clean(projectPath))
	if projectPath == "" {
		return false
	}
	if _, ok := f[projectPath]; ok {
		return true
	}
	for p := range f {
		if strings.HasPrefix(projectPath, p+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// ExtraPathsFunc is an optional callback that returns additional paths to
// include in the filter for a given project (e.g. worktree paths for Claude).
type ExtraPathsFunc func(p db.Project) []string

// TrackedProjectFilter builds a PathFilter from all tracked projects in the
// database. The optional extraPaths callback can add agent-specific paths
// (e.g. worktree paths for Claude).
func TrackedProjectFilter(ctx context.Context, database *sql.DB, extraPaths ExtraPathsFunc) PathFilter {
	projects, err := db.ListProjects(ctx, database, false)
	if err != nil {
		return make(PathFilter)
	}
	var paths []string
	for _, p := range projects {
		paths = append(paths, p.Path)
		if extraPaths != nil {
			paths = append(paths, extraPaths(p)...)
		}
		for _, oldPath := range strings.Split(p.OldPaths, "\n") {
			paths = append(paths, oldPath)
		}
	}
	result := NewPathFilter(paths)
	if result == nil {
		return make(PathFilter)
	}
	return result
}
