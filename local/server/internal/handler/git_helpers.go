package handler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func runGit(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func resolveGitIdentity(ctx context.Context, path string) (gitIdentity, error) {
	name, _ := runGit(ctx, path, "config", "--get", "user.name")
	email, _ := runGit(ctx, path, "config", "--get", "user.email")
	id := gitIdentity{Name: strings.TrimSpace(name), Email: strings.TrimSpace(email)}
	if id.Name == "" && id.Email == "" {
		return gitIdentity{}, fmt.Errorf("missing git identity for %q", path)
	}
	return id, nil
}

func commitMatchesIdentity(c gitCommit, identity gitIdentity) bool {
	if identity.Email != "" {
		return strings.EqualFold(strings.TrimSpace(c.UserEmail), identity.Email)
	}
	if identity.Name != "" {
		return strings.TrimSpace(c.UserName) == identity.Name
	}
	return false
}

func dbCommitMatchesIdentity(c db.Commit, identity gitIdentity) bool {
	if identity.Email != "" {
		return strings.EqualFold(strings.TrimSpace(c.UserEmail), identity.Email)
	}
	if identity.Name != "" {
		return strings.TrimSpace(c.UserName) == identity.Name
	}
	return false
}

func commitMatchesExpandedIdentity(authorEmail string, identity gitIdentity, extraEmails []string) bool {
	email := strings.TrimSpace(authorEmail)
	if identity.Email != "" && strings.EqualFold(email, identity.Email) {
		return true
	}
	for _, extra := range extraEmails {
		if strings.EqualFold(email, extra) {
			return true
		}
	}
	return false
}

func listCommitsByIdentity(ctx context.Context, path, branch string, identity gitIdentity) ([]gitCommit, error) {
	out, err := runGit(ctx, path,
		"log", branch,
		"--pretty=format:%H%x1f%an%x1f%ae%x1f%ct%x1f%s%x1e",
		"--reverse",
		fmt.Sprintf("--max-count=%d", maxCommitsPerProject),
	)
	if err != nil {
		return nil, err
	}

	records := strings.Split(out, "\x1e")
	commits := make([]gitCommit, 0, len(records))
	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		parts := strings.Split(rec, "\x1f")
		if len(parts) < 5 {
			continue
		}
		ts, err := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
		if err != nil {
			continue
		}
		c := gitCommit{
			Hash:          strings.TrimSpace(parts[0]),
			UserName:      strings.TrimSpace(parts[1]),
			UserEmail:     strings.TrimSpace(parts[2]),
			TimestampUnix: ts,
			Subject:       strings.TrimSpace(parts[4]),
		}
		if commitMatchesIdentity(c, identity) {
			commits = append(commits, c)
		}
	}
	return commits, nil
}

func latestCommitByIdentity(ctx context.Context, path, branch string, identity gitIdentity) (*gitCommit, error) {
	commits, err := listCommitsByIdentity(ctx, path, branch, identity)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, nil
	}
	return &commits[len(commits)-1], nil
}

// listBranchCommitHashes returns all commit hashes on a branch, newest first.
func listBranchCommitHashes(ctx context.Context, repoPath, branch string) ([]string, error) {
	out, err := runGit(ctx, repoPath, "log", branch, "--format=%H")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	hashes := make([]string, 0, len(lines))
	for _, line := range lines {
		h := strings.TrimSpace(line)
		if h != "" {
			hashes = append(hashes, h)
		}
	}
	return hashes, nil
}

// countBranchCommits returns the total number of commits on a branch.
func countBranchCommits(ctx context.Context, repoPath, branch string) (int, error) {
	out, err := runGit(ctx, repoPath, "rev-list", "--count", branch)
	if err != nil {
		return 0, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("parse rev-list count: %w", err)
	}
	return count, nil
}

// listBranchCommits returns commits on a branch WITHOUT identity filtering.
// Returned in oldest-first order. maxCount=0 means no limit.
func listBranchCommits(ctx context.Context, repoPath, branch string, maxCount int) ([]gitCommit, error) {
	args := []string{"log", branch, "--pretty=format:%H%x1f%an%x1f%ae%x1f%ct%x1f%s%x1e", "--reverse"}
	if maxCount > 0 {
		args = append(args, fmt.Sprintf("--max-count=%d", maxCount))
	}
	out, err := runGit(ctx, repoPath, args...)
	if err != nil {
		return nil, err
	}

	records := strings.Split(out, "\x1e")
	commits := make([]gitCommit, 0, len(records))
	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		parts := strings.Split(rec, "\x1f")
		if len(parts) < 5 {
			continue
		}
		ts, err := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
		if err != nil {
			continue
		}
		commits = append(commits, gitCommit{
			Hash:          strings.TrimSpace(parts[0]),
			UserName:      strings.TrimSpace(parts[1]),
			UserEmail:     strings.TrimSpace(parts[2]),
			TimestampUnix: ts,
			Subject:       strings.TrimSpace(parts[4]),
		})
	}
	return commits, nil
}

// getCommitMetadata fetches metadata for a single commit hash from git.
func getCommitMetadata(ctx context.Context, repoPath, hash string) (*gitCommit, error) {
	out, err := runGit(ctx, repoPath, "show", "-s", "--format=%H%x1f%an%x1f%ae%x1f%ct%x1f%s", hash)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimSpace(out), "\x1f")
	if len(parts) < 5 {
		return nil, fmt.Errorf("unexpected git show output for %s", hash)
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse timestamp for %s: %w", hash, err)
	}
	return &gitCommit{
		Hash:          strings.TrimSpace(parts[0]),
		UserName:      strings.TrimSpace(parts[1]),
		UserEmail:     strings.TrimSpace(parts[2]),
		TimestampUnix: ts,
		Subject:       strings.TrimSpace(parts[4]),
	}, nil
}

func gitRootCommit(ctx context.Context, path string) (string, error) {
	out, err := runGit(ctx, path, "rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(out)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if line == "" {
		return "", fmt.Errorf("empty root commit")
	}
	return line, nil
}

func hasWorkingCopyChanges(ctx context.Context, repoProject *db.Project) (projectCommitCoverage, bool) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoProject.Path, "diff", "HEAD", "--quiet")
	if err := cmd.Run(); err == nil {
		return projectCommitCoverage{}, false
	}
	return projectCommitCoverage{
		WorkingCopy:  true,
		ProjectID:    repoProject.ID,
		ProjectLabel: repoProject.Label,
		ProjectPath:  repoProject.Path,
		ProjectGitID: repoProject.GitID,
		CommitHash:   workingCopyCommitHash,
		Subject:      "Working Copy",
	}, true
}

func listAllProjectGroups(ctx context.Context, database *sql.DB) ([]projectGroup, error) {
	active, err := db.ListProjects(ctx, database, false)
	if err != nil {
		return nil, err
	}
	ignored, err := db.ListProjects(ctx, database, true)
	if err != nil {
		return nil, err
	}
	all := append(active, ignored...)
	return groupProjectsByGitID(all), nil
}

func findProjectGroupByProjectID(groups []projectGroup, projectID string) (projectGroup, bool) {
	for _, g := range groups {
		for _, p := range g.Projects {
			if p.ID == projectID {
				return g, true
			}
		}
	}
	return projectGroup{}, false
}

func getProjectByID(ctx context.Context, database *sql.DB, projectID string) (*db.Project, error) {
	var p db.Project
	err := database.QueryRowContext(ctx, "SELECT id, path, old_paths, label, git_id, default_branch, remote, ignored, ignore_diff_paths, ignore_default_diff_paths, team_server_id, git_worktree_paths FROM projects WHERE id = ?", projectID).Scan(
		&p.ID,
		&p.Path,
		&p.OldPaths,
		&p.Label,
		&p.GitID,
		&p.DefaultBranch,
		&p.Remote,
		&p.Ignored,
		&p.IgnoreDiffPaths,
		&p.IgnoreDefaultDiffPaths,
		&p.TeamServerID,
		&p.GitWorktreePaths,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query project: %w", err)
	}
	return &p, nil
}

func ensureProjectRemote(ctx context.Context, database *sql.DB, project *db.Project) string {
	if project == nil {
		return ""
	}
	if project.Remote != "" {
		return project.Remote
	}
	remote, err := runGit(ctx, project.Path, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	if err := db.UpdateProjectRemote(ctx, database, project.ID, remote); err == nil {
		project.Remote = remote
	}
	return remote
}

func ensureProjectLocalUser(ctx context.Context, database *sql.DB, p *db.ProjectDetail) {
	if p.LocalUser != "" {
		return
	}
	name, _ := runGit(ctx, p.Path, "config", "--get", "user.name")
	email, _ := runGit(ctx, p.Path, "config", "--get", "user.email")
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" && email == "" {
		return
	}
	if err := db.UpdateProjectLocalUser(ctx, database, p.ID, name, email); err == nil {
		p.LocalUser = name
		p.LocalEmail = email
	}
}

func groupProjectsByGitID(projects []db.Project) []projectGroup {
	groups := make(map[string][]db.Project)
	for _, p := range projects {
		gitID := strings.TrimSpace(p.GitID)
		if gitID == "" {
			continue
		}
		groups[gitID] = append(groups[gitID], p)
	}

	ids := make([]string, 0, len(groups))
	for gitID := range groups {
		ids = append(ids, gitID)
	}
	sort.Strings(ids)

	out := make([]projectGroup, 0, len(ids))
	for _, gitID := range ids {
		out = append(out, projectGroup{GitID: gitID, Projects: groups[gitID]})
	}
	return out
}

func projectIDs(group projectGroup) []string {
	ids := make([]string, 0, len(group.Projects))
	for _, p := range group.Projects {
		ids = append(ids, p.ID)
	}
	return ids
}

func resolveRepoProject(ctx context.Context, group projectGroup) (*db.Project, error) {
	for i := range group.Projects {
		p := group.Projects[i]
		if info, err := os.Stat(p.Path); err == nil && info.IsDir() {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("no accessible repo path for git id %q", group.GitID)
}
