package handler

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/davidcann/zrate/web/server/internal/db"
)

func detectCurrentBranch(ctx context.Context, repoPath string) string {
	out, err := runGit(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(out)
	if name == "HEAD" {
		return ""
	}
	return name
}

func detectDefaultBranch(ctx context.Context, repoPath string) (string, error) {
	if out, err := runGit(ctx, repoPath, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		name := strings.TrimSpace(out)
		if idx := strings.Index(name, "/"); idx >= 0 && idx < len(name)-1 {
			return strings.TrimSpace(name[idx+1:]), nil
		}
	}
	if out, err := runGit(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		name := strings.TrimSpace(out)
		if name != "" && name != "HEAD" {
			return name, nil
		}
	}
	for _, fallback := range []string{"main", "master"} {
		if _, err := runGit(ctx, repoPath, "show-ref", "--verify", "--quiet", "refs/heads/"+fallback); err == nil {
			return fallback, nil
		}
	}
	return "", fmt.Errorf("could not resolve default branch")
}

func listRepoBranches(ctx context.Context, repoPath, defaultBranch string) ([]string, error) {
	out, err := runGit(ctx, repoPath, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	branches := make([]string, 0, 8)
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		branches = append(branches, name)
	}
	add(defaultBranch)
	for _, line := range strings.Split(out, "\n") {
		add(line)
	}
	return branches, nil
}

func ensureProjectDefaultBranch(ctx context.Context, database *sql.DB, project *db.Project) string {
	if project == nil {
		return ""
	}
	defaultBranch, err := detectDefaultBranch(ctx, project.Path)
	if err != nil {
		return strings.TrimSpace(project.DefaultBranch)
	}
	if defaultBranch != "" && defaultBranch != project.DefaultBranch {
		if err := db.UpdateProjectDefaultBranch(ctx, database, project.ID, defaultBranch); err == nil {
			project.DefaultBranch = defaultBranch
		}
	}
	return defaultBranch
}
