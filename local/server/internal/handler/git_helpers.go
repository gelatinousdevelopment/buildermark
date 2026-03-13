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
	"sync"
	"time"

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

// gitIdentityCache caches resolved git identities per repo path to avoid
// repeated git config calls (each ~8ms). Cache entries expire after 60s.
var gitIdentityCache struct {
	mu      sync.Mutex
	entries map[string]gitIdentityCacheEntry
}

type gitIdentityCacheEntry struct {
	identity gitIdentity
	err      error
	at       time.Time
}

const gitIdentityCacheTTL = 60 * time.Second

func resolveGitIdentity(ctx context.Context, path string) (gitIdentity, error) {
	gitIdentityCache.mu.Lock()
	if e, ok := gitIdentityCache.entries[path]; ok && time.Since(e.at) < gitIdentityCacheTTL {
		gitIdentityCache.mu.Unlock()
		return e.identity, e.err
	}
	gitIdentityCache.mu.Unlock()

	name, _ := runGit(ctx, path, "config", "--get", "user.name")
	email, _ := runGit(ctx, path, "config", "--get", "user.email")
	id := gitIdentity{Name: strings.TrimSpace(name), Email: strings.TrimSpace(email)}
	var err error
	if id.Name == "" && id.Email == "" {
		err = fmt.Errorf("missing git identity for %q", path)
	}

	gitIdentityCache.mu.Lock()
	if gitIdentityCache.entries == nil {
		gitIdentityCache.entries = make(map[string]gitIdentityCacheEntry)
	}
	gitIdentityCache.entries[path] = gitIdentityCacheEntry{identity: id, err: err, at: time.Now()}
	gitIdentityCache.mu.Unlock()

	return id, err
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
	// Use --author flag and --max-count=1 for a fast single-commit lookup
	// instead of fetching up to 1000 commits and filtering in Go.
	authorArg := identity.Email
	if authorArg == "" {
		authorArg = identity.Name
	}
	if authorArg == "" {
		return nil, nil
	}
	out, err := runGit(ctx, path,
		"log", branch,
		"--pretty=format:%H%x1f%an%x1f%ae%x1f%ct%x1f%s%x1e",
		fmt.Sprintf("--author=%s", authorArg),
		"--max-count=1",
	)
	if err != nil {
		return nil, err
	}
	rec := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(out), "\x1e"))
	if rec == "" {
		return nil, nil
	}
	parts := strings.Split(rec, "\x1f")
	if len(parts) < 5 {
		return nil, nil
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
	if err != nil {
		return nil, nil
	}
	c := gitCommit{
		Hash:          strings.TrimSpace(parts[0]),
		UserName:      strings.TrimSpace(parts[1]),
		UserEmail:     strings.TrimSpace(parts[2]),
		TimestampUnix: ts,
		Subject:       strings.TrimSpace(parts[4]),
	}
	return &c, nil
}

// branchHashCache caches commit hash lists per repo+branch to avoid repeated
// git log calls (~12ms each). Entries expire after 5 seconds.
var branchHashCache struct {
	mu      sync.Mutex
	entries map[string]branchHashCacheEntry
}

type branchHashCacheEntry struct {
	hashes []string
	at     time.Time
}

const branchHashCacheTTL = 5 * time.Second

// listBranchCommitHashes returns all commit hashes on a branch, newest first.
func listBranchCommitHashes(ctx context.Context, repoPath, branch string) ([]string, error) {
	cacheKey := repoPath + "\x00" + branch
	branchHashCache.mu.Lock()
	if e, ok := branchHashCache.entries[cacheKey]; ok && time.Since(e.at) < branchHashCacheTTL {
		branchHashCache.mu.Unlock()
		return e.hashes, nil
	}
	branchHashCache.mu.Unlock()

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

	branchHashCache.mu.Lock()
	if branchHashCache.entries == nil {
		branchHashCache.entries = make(map[string]branchHashCacheEntry)
	}
	branchHashCache.entries[cacheKey] = branchHashCacheEntry{hashes: hashes, at: time.Now()}
	branchHashCache.mu.Unlock()

	return hashes, nil
}

// listBranchCommitHashesSince returns commit hashes unique to branch (not on base), newest first.
func listBranchCommitHashesSince(ctx context.Context, repoPath, base, branch string) ([]string, error) {
	out, err := runGit(ctx, repoPath, "log", base+".."+branch, "--format=%H")
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

// listCommitRangeHashes returns the commits reachable from head that are not
// reachable from base, ordered oldest-first so ingestion processes them in
// chronological order.
func listCommitRangeHashes(ctx context.Context, repoPath, base, head string) ([]string, error) {
	base = strings.TrimSpace(base)
	head = strings.TrimSpace(head)
	if base == "" || head == "" || base == head {
		return nil, nil
	}

	out, err := runGit(ctx, repoPath, "rev-list", "--reverse", base+".."+head)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	hashes := make([]string, 0, len(lines))
	for _, line := range lines {
		hash := strings.TrimSpace(line)
		if hash == "" {
			continue
		}
		hashes = append(hashes, hash)
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
	err := database.QueryRowContext(ctx, "SELECT id, path, old_paths, label, git_id, default_branch, remote, ignored, ignore_diff_paths, ignore_default_diff_paths, team_server_id, git_worktree_paths, alt_remotes FROM projects WHERE id = ?", projectID).Scan(
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
		&p.AltRemotes,
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

// shallowBoundaryHashes returns the set of commit hashes at the shallow clone
// boundary. Returns nil for non-shallow repos.
func shallowBoundaryHashes(ctx context.Context, repoPath string) map[string]bool {
	out, err := runGit(ctx, repoPath, "rev-parse", "--is-shallow-repository")
	if err != nil || strings.TrimSpace(out) != "true" {
		return nil
	}
	// Read the .git/shallow file which contains one hash per line.
	gitDir := repoPath + "/.git"
	// Handle worktrees where .git is a file pointing to the actual git dir.
	if info, statErr := os.Stat(gitDir); statErr == nil && !info.IsDir() {
		content, readErr := os.ReadFile(gitDir)
		if readErr == nil {
			line := strings.TrimSpace(string(content))
			if strings.HasPrefix(line, "gitdir: ") {
				gitDir = strings.TrimPrefix(line, "gitdir: ")
				if !strings.HasPrefix(gitDir, "/") {
					gitDir = repoPath + "/" + gitDir
				}
				// Go up from worktree git dir to find the main .git dir.
				if idx := strings.Index(gitDir, "/worktrees/"); idx >= 0 {
					gitDir = gitDir[:idx]
				}
			}
		}
	}
	shallowFile := gitDir + "/shallow"
	data, err := os.ReadFile(shallowFile)
	if err != nil {
		return nil
	}
	hashes := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		h := strings.TrimSpace(line)
		if h != "" {
			hashes[h] = true
		}
	}
	if len(hashes) == 0 {
		return nil
	}
	return hashes
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
