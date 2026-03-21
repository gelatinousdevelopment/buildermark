package gitmonitor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	defaultDebounceInterval  = 750 * time.Millisecond
	defaultReconcileInterval = 5 * time.Minute
)

type RepoConfig struct {
	RepoID        string
	RepoPath      string
	DefaultBranch string
}

type BranchChange struct {
	RepoID           string
	RepoPath         string
	Branch           string
	PreviousHeadHash string
	HeadHash         string
	Reason           string
}

type Options struct {
	DebounceInterval  time.Duration
	ReconcileInterval time.Duration
	OnBranchChange    func(context.Context, BranchChange)
}

type Manager struct {
	ctx      context.Context
	cancel   context.CancelFunc
	onChange func(context.Context, BranchChange)

	debounceInterval  time.Duration
	reconcileInterval time.Duration

	mu    sync.Mutex
	repos map[string]*repoMonitor
}

type repoMonitor struct {
	ctx      context.Context
	cancel   context.CancelFunc
	onChange func(context.Context, BranchChange)

	config            RepoConfig
	debounceInterval  time.Duration
	reconcileInterval time.Duration

	mu sync.Mutex

	lastHeads   map[string]string
	watchedDirs map[string]struct{}
}

type repoState struct {
	commonDir   string
	branchHeads map[string]string
	watchPaths  []string
}

type worktreeState struct {
	path         string
	gitDir       string
	activeBranch string
}

var runGit = func(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func New(parent context.Context, opts Options) *Manager {
	ctx, cancel := context.WithCancel(parent)
	debounce := opts.DebounceInterval
	if debounce <= 0 {
		debounce = defaultDebounceInterval
	}
	reconcile := opts.ReconcileInterval
	if reconcile <= 0 {
		reconcile = defaultReconcileInterval
	}

	return &Manager{
		ctx:               ctx,
		cancel:            cancel,
		onChange:          opts.OnBranchChange,
		debounceInterval:  debounce,
		reconcileInterval: reconcile,
		repos:             make(map[string]*repoMonitor),
	}
}

func (m *Manager) Close() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()
	for id, repo := range m.repos {
		repo.stop()
		delete(m.repos, id)
	}
}

func (m *Manager) Reconcile(configs []RepoConfig) {
	next := make(map[string]RepoConfig, len(configs))
	for _, cfg := range configs {
		cfg.RepoID = strings.TrimSpace(cfg.RepoID)
		cfg.RepoPath = strings.TrimSpace(cfg.RepoPath)
		cfg.DefaultBranch = strings.TrimSpace(cfg.DefaultBranch)
		if cfg.RepoID == "" || cfg.RepoPath == "" {
			continue
		}
		next[cfg.RepoID] = cfg
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, repo := range m.repos {
		cfg, ok := next[id]
		if !ok {
			repo.stop()
			delete(m.repos, id)
			continue
		}
		if repo.sameConfig(cfg) {
			delete(next, id)
			continue
		}
		repo.stop()
		delete(m.repos, id)
	}

	for id, cfg := range next {
		repo := newRepoMonitor(m.ctx, cfg, m.debounceInterval, m.reconcileInterval, m.onChange)
		m.repos[id] = repo
		go repo.run()
	}
}

func newRepoMonitor(parent context.Context, cfg RepoConfig, debounce, reconcile time.Duration, onChange func(context.Context, BranchChange)) *repoMonitor {
	ctx, cancel := context.WithCancel(parent)
	return &repoMonitor{
		ctx:               ctx,
		cancel:            cancel,
		onChange:          onChange,
		config:            cfg,
		debounceInterval:  debounce,
		reconcileInterval: reconcile,
		lastHeads:   make(map[string]string),
		watchedDirs: make(map[string]struct{}),
	}
}

func (r *repoMonitor) sameConfig(cfg RepoConfig) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.config == cfg
}

func (r *repoMonitor) stop() {
	r.cancel()
}

func (r *repoMonitor) run() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("git monitor: create watcher for %s: %v", r.config.RepoPath, err)
		return
	}
	defer watcher.Close()

	r.refresh(watcher, "startup")

	ticker := time.NewTicker(r.reconcileInterval)
	defer ticker.Stop()
	lastReconcile := time.Now()

	var debounce *time.Timer
	var debounceC <-chan time.Time
	resetDebounce := func() {
		if debounce == nil {
			debounce = time.NewTimer(r.debounceInterval)
			debounceC = debounce.C
			return
		}
		if !debounce.Stop() {
			select {
			case <-debounce.C:
			default:
			}
		}
		debounce.Reset(r.debounceInterval)
		debounceC = debounce.C
	}

	for {
		select {
		case <-r.ctx.Done():
			if debounce != nil {
				debounce.Stop()
			}
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			resetDebounce()
		case err, ok := <-watcher.Errors:
			if ok && !errors.Is(err, os.ErrClosed) {
				log.Printf("git monitor: watcher error for %s: %v", r.config.RepoPath, err)
			}
		case <-ticker.C:
			elapsed := time.Since(lastReconcile)
			if elapsed > 2*r.reconcileInterval {
				log.Printf("git monitor: sleep detected for %s (elapsed %s > %s), updating heads silently", r.config.RepoPath, elapsed, 2*r.reconcileInterval)
				r.refreshSilent(watcher)
			} else {
				r.refresh(watcher, "reconcile")
			}
			lastReconcile = time.Now()
		case <-debounceC:
			debounceC = nil
			r.refresh(watcher, "fs_event")
		}
	}
}

func (r *repoMonitor) refresh(watcher *fsnotify.Watcher, reason string) {
	state, err := discoverRepoState(r.ctx, r.config)
	if err != nil {
		if r.ctx.Err() == nil {
			log.Printf("git monitor: refresh %s for %s failed: %v", reason, r.config.RepoPath, err)
		}
		return
	}

	r.applyWatchPaths(watcher, state.watchPaths)

	r.mu.Lock()
	prev := make(map[string]string, len(r.lastHeads))
	for branch, hash := range r.lastHeads {
		prev[branch] = hash
	}
	r.lastHeads = state.branchHeads
	r.mu.Unlock()

	for branch, head := range state.branchHeads {
		if head == "" {
			continue
		}
		prevHead := strings.TrimSpace(prev[branch])
		if prevHead == head {
			continue
		}
		if r.onChange == nil {
			continue
		}
		change := BranchChange{
			RepoID:           r.config.RepoID,
			RepoPath:         r.config.RepoPath,
			Branch:           branch,
			PreviousHeadHash: prevHead,
			HeadHash:         head,
			Reason:           reason,
		}
		go r.onChange(r.ctx, change)
	}
}

// refreshSilent updates lastHeads and watch paths without firing onChange.
// Used after detecting a system sleep to avoid treating stale state as changes.
func (r *repoMonitor) refreshSilent(watcher *fsnotify.Watcher) {
	state, err := discoverRepoState(r.ctx, r.config)
	if err != nil {
		if r.ctx.Err() == nil {
			log.Printf("git monitor: silent refresh for %s failed: %v", r.config.RepoPath, err)
		}
		return
	}

	r.applyWatchPaths(watcher, state.watchPaths)

	r.mu.Lock()
	r.lastHeads = state.branchHeads
	r.mu.Unlock()
}

func (r *repoMonitor) applyWatchPaths(watcher *fsnotify.Watcher, dirs []string) {
	next := make(map[string]struct{}, len(dirs))
	for _, dir := range dirs {
		dir = filepath.Clean(strings.TrimSpace(dir))
		if dir == "" {
			continue
		}
		next[dir] = struct{}{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for dir := range r.watchedDirs {
		if _, ok := next[dir]; ok {
			continue
		}
		if err := watcher.Remove(dir); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
			log.Printf("git monitor: remove watch %s: %v", dir, err)
		}
		delete(r.watchedDirs, dir)
	}

	for dir := range next {
		if _, ok := r.watchedDirs[dir]; ok {
			continue
		}
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		if err := watcher.Add(dir); err != nil {
			log.Printf("git monitor: add watch %s: %v", dir, err)
			continue
		}
		r.watchedDirs[dir] = struct{}{}
		log.Printf("git monitor: watching repo_id=%s repo_path=%s dir=%s", r.config.RepoID, r.config.RepoPath, dir)
	}
}

func discoverRepoState(ctx context.Context, cfg RepoConfig) (repoState, error) {
	commonDir, err := gitPath(ctx, cfg.RepoPath, "--git-common-dir")
	if err != nil {
		return repoState{}, fmt.Errorf("resolve git common dir: %w", err)
	}

	defaultBranch := strings.TrimSpace(cfg.DefaultBranch)
	if defaultBranch == "" {
		defaultBranch, err = detectDefaultBranch(ctx, cfg.RepoPath)
		if err != nil {
			return repoState{}, err
		}
	}

	worktrees, err := listWorktrees(ctx, cfg.RepoPath)
	if err != nil {
		return repoState{}, fmt.Errorf("list worktrees: %w", err)
	}

	branches := make(map[string]struct{}, len(worktrees)+1)
	if defaultBranch != "" {
		branches[defaultBranch] = struct{}{}
	}

	for _, wt := range worktrees {
		if wt.activeBranch != "" {
			branches[wt.activeBranch] = struct{}{}
		}
	}

	// Watch directories rather than individual files. Many programs
	// (including git) update files atomically via write-to-temp-then-rename,
	// which silently removes fsnotify watches on individual files.
	// We watch the reflog directories since git appends to reflog files
	// on every commit, and directory watches reliably fire for these writes.
	watchDirs := make(map[string]struct{}, len(worktrees)+1)
	for _, wt := range worktrees {
		// Watch <gitDir>/logs/ for worktree HEAD reflog changes.
		watchDirs[filepath.Join(wt.gitDir, "logs")] = struct{}{}
	}
	// Watch logs/refs/heads/ for branch reflog updates.
	watchDirs[filepath.Join(commonDir, "logs", "refs", "heads")] = struct{}{}

	watchPaths := make([]string, 0, len(watchDirs))
	for dir := range watchDirs {
		watchPaths = append(watchPaths, dir)
	}

	heads := make(map[string]string, len(branches))
	for branch := range branches {
		head, err := branchHeadHash(ctx, cfg.RepoPath, branch)
		if err != nil {
			continue
		}
		if head != "" {
			heads[branch] = head
		}
	}

	return repoState{
		commonDir:   commonDir,
		branchHeads: heads,
		watchPaths:  dedupePaths(watchPaths),
	}, nil
}

func listWorktrees(ctx context.Context, repoPath string) ([]worktreeState, error) {
	out, err := runGit(ctx, repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []worktreeState
	var current worktreeState
	flush := func() {
		if current.path == "" {
			return
		}
		gitDir, err := gitPath(ctx, current.path, "--git-dir")
		if err == nil {
			current.gitDir = gitDir
		}
		current.activeBranch = detectCurrentBranch(ctx, current.path)
		worktrees = append(worktrees, current)
		current = worktreeState{}
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			current.path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		}
	}
	flush()

	if len(worktrees) == 0 {
		gitDir, err := gitPath(ctx, repoPath, "--git-dir")
		if err != nil {
			return nil, err
		}
		worktrees = append(worktrees, worktreeState{
			path:         repoPath,
			gitDir:       gitDir,
			activeBranch: detectCurrentBranch(ctx, repoPath),
		})
	}

	return worktrees, nil
}

func branchHeadHash(ctx context.Context, repoPath, branch string) (string, error) {
	out, err := runGit(ctx, repoPath, "rev-parse", branch)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func detectCurrentBranch(ctx context.Context, repoPath string) string {
	out, err := runGit(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(out)
	if branch == "" || branch == "HEAD" {
		return ""
	}
	return branch
}

func detectDefaultBranch(ctx context.Context, repoPath string) (string, error) {
	if out, err := runGit(ctx, repoPath, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		name := strings.TrimSpace(out)
		if idx := strings.Index(name, "/"); idx >= 0 && idx < len(name)-1 {
			return strings.TrimSpace(name[idx+1:]), nil
		}
	}
	if branch := detectCurrentBranch(ctx, repoPath); branch != "" {
		return branch, nil
	}
	for _, fallback := range []string{"main", "master"} {
		if _, err := runGit(ctx, repoPath, "show-ref", "--verify", "--quiet", "refs/heads/"+fallback); err == nil {
			return fallback, nil
		}
	}
	return "", fmt.Errorf("could not resolve default branch")
}

func gitPath(ctx context.Context, repoPath string, flag string) (string, error) {
	out, err := runGit(ctx, repoPath, "rev-parse", flag)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(out)
	if path == "" {
		return "", fmt.Errorf("empty git path for %s", flag)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoPath, path)
	}
	return filepath.Clean(path), nil
}

func dedupePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}
