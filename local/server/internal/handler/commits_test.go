package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// waitForCommitIngestion polls until no commit ingestion goroutines are running.
func waitForCommitIngestion(t *testing.T, s *Server) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s.commitIngestJobs.isIdle() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for commit ingestion to complete")
}

// waitForCommitRefresh polls until no commit refresh goroutines are running.
func waitForCommitRefresh(t *testing.T, s *Server) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s.refreshJobs.isIdle() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for commit refresh to complete")
}

func TestCommitRefreshTimeout(t *testing.T) {
	if got := commitRefreshTimeout(0); got != defaultCommitRefreshTimeout {
		t.Fatalf("commitRefreshTimeout(0) = %s, want %s", got, defaultCommitRefreshTimeout)
	}
	if got := commitRefreshTimeout(7); got != extendedCommitRefreshTimeout {
		t.Fatalf("commitRefreshTimeout(7) = %s, want %s", got, extendedCommitRefreshTimeout)
	}
}

func TestHasReachableStaleCommitCoverageByBranchIgnoresOrphanedBranchRows(t *testing.T) {
	s := setupTestServer(t)
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	writeAndCommit := func(name, body, message string) string {
		mustWriteFile(t, filepath.Join(repo, name), body)
		gitRun(t, repo, nil, "add", name)
		gitRun(t, repo, nil, "commit", "-m", message)
		return strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))
	}

	reachableHash := writeAndCommit("app.txt", "one\n", "reachable")
	orphanedHash := writeAndCommit("app.txt", "two\n", "orphaned")
	gitRun(t, repo, nil, "reset", "--hard", reachableHash)

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	repoProject := &db.Project{ID: projectID, Path: repo}

	reachableCommit := db.Commit{
		ID:              "commit-reachable",
		ProjectID:       projectID,
		BranchName:      "main",
		CommitHash:      reachableHash,
		Subject:         "reachable",
		UserName:        "Test User",
		UserEmail:       "test@example.com",
		AuthoredAt:      1700000000,
		DiffContent:     "diff --git a/app.txt b/app.txt\n+one",
		LinesTotal:      1,
		CoverageVersion: currentCommitCoverageVersion,
	}
	if err := db.UpsertCommit(ctx, s.DB, reachableCommit); err != nil {
		t.Fatalf("UpsertCommit reachable: %v", err)
	}
	orphanedCommit := db.Commit{
		ID:              "commit-orphaned",
		ProjectID:       projectID,
		BranchName:      "main",
		CommitHash:      orphanedHash,
		Subject:         "orphaned",
		UserName:        "Test User",
		UserEmail:       "test@example.com",
		AuthoredAt:      1700000001,
		DiffContent:     "diff --git a/app.txt b/app.txt\n+two",
		LinesTotal:      1,
		CoverageVersion: 0,
	}
	if err := db.UpsertCommit(ctx, s.DB, orphanedCommit); err != nil {
		t.Fatalf("UpsertCommit orphaned: %v", err)
	}

	stale, err := db.HasStaleCommitCoverageByBranch(ctx, s.DB, projectID, "main", currentCommitCoverageVersion)
	if err != nil {
		t.Fatalf("HasStaleCommitCoverageByBranch: %v", err)
	}
	if !stale {
		t.Fatal("expected DB-only stale check to see orphaned stale row")
	}

	hashes, err := listBranchCommitHashes(ctx, repo, "main")
	if err != nil {
		t.Fatalf("listBranchCommitHashes: %v", err)
	}
	if len(hashes) != 1 || hashes[0] != reachableHash {
		t.Fatalf("reachable hashes = %v, want [%s]", hashes, reachableHash)
	}

	reachableStale, err := hasReachableStaleCommitCoverageByBranch(ctx, s.DB, repoProject, "main", currentCommitCoverageVersion)
	if err != nil {
		t.Fatalf("hasReachableStaleCommitCoverageByBranch: %v", err)
	}
	if reachableStale {
		t.Fatal("expected reachable stale check to ignore orphaned branch row")
	}
}

func TestCommitIngestionQueueDefersAndRestarts(t *testing.T) {
	s := setupTestServer(t)

	key, hashes, started, pendingCount := s.reserveCommitIngestion("project-1", "main", []string{"a", "b", "a"})
	if key != "project-1:main" {
		t.Fatalf("key = %q, want %q", key, "project-1:main")
	}
	if !started {
		t.Fatal("started = false, want true")
	}
	if pendingCount != 0 {
		t.Fatalf("pendingCount = %d, want 0", pendingCount)
	}
	if got, want := strings.Join(hashes, ","), "a,b"; got != want {
		t.Fatalf("hashes = %q, want %q", got, want)
	}

	key, hashes, started, pendingCount = s.reserveCommitIngestion("project-1", "main", []string{"b", "c", "d"})
	if key != "project-1:main" {
		t.Fatalf("key = %q, want %q", key, "project-1:main")
	}
	if started {
		t.Fatal("started = true, want false for deferred enqueue")
	}
	if hashes != nil {
		t.Fatalf("hashes = %v, want nil for deferred enqueue", hashes)
	}
	if pendingCount != 3 {
		t.Fatalf("pendingCount = %d, want 3", pendingCount)
	}

	next, restart := s.releaseCommitIngestion("project-1:main")
	if !restart {
		t.Fatal("restart = false, want true")
	}
	if got, want := strings.Join(next, ","), "b,c,d"; got != want {
		t.Fatalf("next = %q, want %q", got, want)
	}

	next, restart = s.releaseCommitIngestion("project-1:main")
	if restart {
		t.Fatal("restart = true, want false with no pending hashes")
	}
	if len(next) != 0 {
		t.Fatalf("next = %v, want empty", next)
	}
}

func TestListProjectCommits(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	appPath := filepath.Join(repo, "app.txt")
	mustWriteFile(t, appPath, "start\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "add", "app.txt")
	gitRun(t, repo, []string{
		"GIT_AUTHOR_NAME=Other User",
		"GIT_AUTHOR_EMAIL=other@example.com",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	}, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	pid, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, pid, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}

	if err := db.EnsureConversation(ctx, s.DB, "conv-1", pid, "codex"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	agentTs := mustUnixMilli(t, "2026-01-01T00:59:00Z")
	agentDiff := "```diff\n" +
		"diff --git a/app.txt b/app.txt\n" +
		"--- a/app.txt\n" +
		"+++ b/app.txt\n" +
		"@@ -1 +1,2 @@\n" +
		" start\n" +
		"+hello   world\n" +
		"```"
	if err := db.InsertMessages(ctx, s.DB, []db.Message{{
		Timestamp:      agentTs,
		ProjectID:      pid,
		ConversationID: "conv-1",
		Role:           "agent",
		Content:        agentDiff,
		RawJSON:        agent.DerivedDiffRawJSON,
	}}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	mustWriteFile(t, appPath, "start\nhello world\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "add", "app.txt")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "commit", "-m", "agent change")
	agentCommitHash := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	mustWriteFile(t, appPath, "start\nhello world\nmanual change\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T02:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T02:00:00Z"}, "add", "app.txt")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T02:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T02:00:00Z"}, "commit", "-m", "manual change")

	req := httptest.NewRequest("GET", "/api/v1/projects/commits", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok=false, error=%v", env.Error)
	}

	data := env.Data.(map[string]any)
	summary := data["summary"].(map[string]any)
	// Now includes all users' commits: initial (Other User) + agent change + manual change.
	if got := int(summary["commitCount"].(float64)); got != 3 {
		t.Fatalf("summary.commitCount = %d, want 3", got)
	}
	if got := int(summary["linesTotal"].(float64)); got < 2 {
		t.Fatalf("summary.linesTotal = %d, want >= 2", got)
	}
	if got := int(summary["linesFromAgent"].(float64)); got != 1 {
		t.Fatalf("summary.linesFromAgent = %d, want 1", got)
	}

	commits := data["commits"].([]any)
	if len(commits) != 3 {
		t.Fatalf("commits = %d, want 3", len(commits))
	}

	bySubject := map[string]map[string]any{}
	for _, raw := range commits {
		item := raw.(map[string]any)
		bySubject[item["subject"].(string)] = item
	}

	agentCommit, ok := bySubject["agent change"]
	if !ok {
		t.Fatalf("missing commit subject %q", "agent change")
	}
	if got := int(agentCommit["linesTotal"].(float64)); got != 1 {
		t.Fatalf("agent change linesTotal = %d, want 1", got)
	}
	if got := int(agentCommit["linesFromAgent"].(float64)); got != 1 {
		t.Fatalf("agent change linesFromAgent = %d, want 1", got)
	}

	manualCommit, ok := bySubject["manual change"]
	if !ok {
		t.Fatalf("missing commit subject %q", "manual change")
	}
	if got := int(manualCommit["linesFromAgent"].(float64)); got != 0 {
		t.Fatalf("manual change linesFromAgent = %d, want 0", got)
	}

	// The "initial" commit by Other User is now visible.
	_, ok = bySubject["initial"]
	if !ok {
		t.Fatalf("missing commit subject %q (other user's commit should be visible)", "initial")
	}

	workingCopyTs := mustUnixMilli(t, "2026-01-01T02:30:00Z")
	workingCopyDiff := "```diff\n" +
		"diff --git a/app.txt b/app.txt\n" +
		"--- a/app.txt\n" +
		"+++ b/app.txt\n" +
		"@@ -3,0 +4 @@\n" +
		"+scratch   line\n" +
		"```"
	if err := db.InsertMessages(ctx, s.DB, []db.Message{{
		Timestamp:      workingCopyTs,
		ProjectID:      pid,
		ConversationID: "conv-1",
		Role:           "agent",
		Content:        workingCopyDiff,
		RawJSON:        agent.DerivedDiffRawJSON,
	}}); err != nil {
		t.Fatalf("InsertMessages working copy: %v", err)
	}
	mustWriteFile(t, appPath, "start\nhello world\nmanual change\nscratch line\n")

	req = httptest.NewRequest("GET", "/api/v1/projects/"+pid+"/commits?page=1", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("project commits status = %d, want %d", rec.Code, http.StatusOK)
	}

	env = jsonEnvelope{}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode project commits response: %v", err)
	}
	if !env.OK {
		t.Fatalf("project commits ok=false, error=%v", env.Error)
	}
	projectCommits := env.Data.(map[string]any)
	pagination := projectCommits["pagination"].(map[string]any)
	if got := int(pagination["pageSize"].(float64)); got != 20 {
		t.Fatalf("pagination.pageSize = %d, want 20", got)
	}
	// Now includes all users' commits (initial by Other User + 2 by Test User).
	if got := int(pagination["total"].(float64)); got != 3 {
		t.Fatalf("pagination.total = %d, want 3", got)
	}
	commitRows := projectCommits["commits"].([]any)
	if len(commitRows) != 4 {
		t.Fatalf("project commits len = %d, want 4 (working copy + 3 commits)", len(commitRows))
	}
	workingCopyRow := commitRows[0].(map[string]any)
	if got := workingCopyRow["subject"].(string); got != "Working Copy" {
		t.Fatalf("first row subject = %q, want Working Copy", got)
	}
	if got := workingCopyRow["workingCopy"].(bool); !got {
		t.Fatalf("first row workingCopy = %v, want true", got)
	}
	if got := int(workingCopyRow["linesTotal"].(float64)); got != 0 {
		t.Fatalf("working copy linesTotal = %d, want 0", got)
	}
	if got := int(workingCopyRow["linesFromAgent"].(float64)); got != 0 {
		t.Fatalf("working copy linesFromAgent = %d, want 0", got)
	}
	if got := workingCopyRow["commitHash"].(string); got != "working-copy" {
		t.Fatalf("working copy commitHash = %q, want %q", got, "working-copy")
	}

	req = httptest.NewRequest("GET", "/api/v1/projects/"+pid+"/commits?page=1&pageSize=1", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("project commits with pageSize status = %d, want %d", rec.Code, http.StatusOK)
	}
	env = jsonEnvelope{}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode project commits with pageSize response: %v", err)
	}
	if !env.OK {
		t.Fatalf("project commits with pageSize ok=false, error=%v", env.Error)
	}
	projectCommits = env.Data.(map[string]any)
	pagination = projectCommits["pagination"].(map[string]any)
	if got := int(pagination["pageSize"].(float64)); got != 1 {
		t.Fatalf("pagination.pageSize with query = %d, want 1", got)
	}
	commitRows = projectCommits["commits"].([]any)
	if len(commitRows) != 2 {
		t.Fatalf("project commits with pageSize len = %d, want 2 (working copy + 1 commit)", len(commitRows))
	}

	req = httptest.NewRequest("GET", "/api/v1/projects/"+pid+"/commits/working-copy", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("working-copy detail status = %d, want %d", rec.Code, http.StatusOK)
	}

	env = jsonEnvelope{}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode working-copy detail response: %v", err)
	}
	if !env.OK {
		t.Fatalf("working-copy detail ok=false, error=%v", env.Error)
	}
	workingDetail := env.Data.(map[string]any)
	workingCommit := workingDetail["commit"].(map[string]any)
	if got := workingCommit["subject"].(string); got != "Working Copy" {
		t.Fatalf("working-copy detail subject = %q, want %q", got, "Working Copy")
	}

	req = httptest.NewRequest("GET", "/api/v1/projects/"+pid+"/commits/"+agentCommitHash, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d", rec.Code, http.StatusOK)
	}

	env = jsonEnvelope{}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if !env.OK {
		t.Fatalf("detail ok=false, error=%v", env.Error)
	}

	detail := env.Data.(map[string]any)
	commit := detail["commit"].(map[string]any)
	if got := commit["commitHash"].(string); got != agentCommitHash {
		t.Fatalf("detail commit hash = %q, want %q", got, agentCommitHash)
	}
	messages := detail["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("detail messages = %d, want 1", len(messages))
	}
	msg := messages[0].(map[string]any)
	if got := msg["conversationId"].(string); got != "conv-1" {
		t.Fatalf("detail conversationId = %q, want %q", got, "conv-1")
	}
	if got := int(msg["linesMatched"].(float64)); got != 1 {
		t.Fatalf("detail linesMatched = %d, want 1", got)
	}
}

func TestListProjectCommits_UsesRawJSONDiffWithoutDerivedFlag(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	appPath := filepath.Join(repo, "app.txt")
	mustWriteFile(t, appPath, "start\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "add", "app.txt")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	pid, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, pid, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-raw-json", pid, "codex"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	rawOnlyTs := mustUnixMilli(t, "2026-01-01T00:59:00Z")
	rawOnlyJSON := `{"type":"response_item","payload":{"type":"function_call_output","output":"diff --git a/app.txt b/app.txt\n--- a/app.txt\n+++ b/app.txt\n@@ -1 +1,2 @@\n start\n+hello world\n"}}`
	if err := db.InsertMessages(ctx, s.DB, []db.Message{{
		Timestamp:      rawOnlyTs,
		ProjectID:      pid,
		ConversationID: "conv-raw-json",
		Role:           "agent",
		Content:        "[response_item:function_call_output] function_call_output",
		RawJSON:        rawOnlyJSON,
	}}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	mustWriteFile(t, appPath, "start\nhello world\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "add", "app.txt")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "commit", "-m", "agent change")

	req := httptest.NewRequest("GET", "/api/v1/projects/commits", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok=false, error=%v", env.Error)
	}

	data := env.Data.(map[string]any)
	commits := data["commits"].([]any)
	if len(commits) != 2 {
		t.Fatalf("commits = %d, want 2", len(commits))
	}
	bySubject := map[string]map[string]any{}
	for _, raw := range commits {
		item := raw.(map[string]any)
		bySubject[item["subject"].(string)] = item
	}
	agentCommit, ok := bySubject["agent change"]
	if !ok {
		t.Fatalf("missing commit subject %q", "agent change")
	}
	if got := int(agentCommit["linesFromAgent"].(float64)); got != 1 {
		t.Fatalf("agent change linesFromAgent = %d, want 1", got)
	}
}

func TestListProjectCommits_UsesRawJSONFileSnapshotForNewFile(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	readmePath := filepath.Join(repo, "README.md")
	mustWriteFile(t, readmePath, "start\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "add", "README.md")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	pid, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, pid, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-file-snapshot", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	filePath := filepath.Join(repo, "web", "frontend", "src", "app.svelte")
	rawOnlyTs := mustUnixMilli(t, "2026-01-01T00:59:00Z")
	rawOnlyJSON := `{"cwd":"` + strings.ReplaceAll(filepath.Join(repo, "web", "server"), `\`, `\\`) + `","toolUseResult":{"file":{"filePath":"` + strings.ReplaceAll(filePath, `\`, `\\`) + `","content":"     1→<script>\n     2→\tlet x = 1;\n     3→</script>\n     4→","numLines":4,"startLine":1,"totalLines":4}}}`
	if err := db.InsertMessages(ctx, s.DB, []db.Message{{
		Timestamp:      rawOnlyTs,
		ProjectID:      pid,
		ConversationID: "conv-file-snapshot",
		Role:           "agent",
		Content:        "     1→<script>\n     2→\tlet x = 1;\n     3→</script>\n     4→",
		RawJSON:        rawOnlyJSON,
	}}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	mustWriteFile(t, filePath, "<script>\n\tlet x = 1;\n</script>\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "add", "web/frontend/src/app.svelte")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "commit", "-m", "add snapshot file")

	req := httptest.NewRequest("GET", "/api/v1/projects/commits", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok=false, error=%v", env.Error)
	}

	data := env.Data.(map[string]any)
	commits := data["commits"].([]any)
	bySubject := map[string]map[string]any{}
	for _, raw := range commits {
		item := raw.(map[string]any)
		bySubject[item["subject"].(string)] = item
	}
	agentCommit, ok := bySubject["add snapshot file"]
	if !ok {
		t.Fatalf("missing commit subject %q", "add snapshot file")
	}
	if got := int(agentCommit["linesFromAgent"].(float64)); got != 3 {
		t.Fatalf("add snapshot file linesFromAgent = %d, want 3", got)
	}
}

func TestListProjectCommits_LinksFallbackConversationWhenPathsDiffer(t *testing.T) {
	lines := []string{
		"alpha01", "alpha02", "alpha03", "alpha04", "alpha05", "alpha06",
		"alpha07", "alpha08", "alpha09", "alpha10", "alpha11", "alpha12",
	}
	commitTokens := make([]diffToken, 0, len(lines))
	for _, line := range lines {
		commitTokens = append(commitTokens, testToken("local/frontend/src/lib/diffmerge.ts", '+', line, len(line)))
	}
	messages := []messageDiff{
		{
			ID:                "m-old",
			Timestamp:         1000,
			ConversationID:    "conv-old-path",
			ConversationTitle: "old path source",
			Agent:             "claude",
			Tokens: func() []diffToken {
				out := make([]diffToken, 0, len(lines))
				for _, line := range lines {
					out = append(out, testToken("diffmerge.ts", '+', line, len(line)))
				}
				return out
			}(),
		},
		{
			ID:                "m-local",
			Timestamp:         1500,
			ConversationID:    "conv-local",
			ConversationTitle: "local exact tweak",
			Agent:             "claude",
			Tokens: []diffToken{
				testToken("local/frontend/src/lib/diffmerge.ts", '+', lines[len(lines)-1], len(lines[len(lines)-1])),
			},
		},
	}
	diffText := strings.Join([]string{
		"diff --git a/local/frontend/src/lib/diffmerge.ts b/local/frontend/src/lib/diffmerge.ts",
		"--- /dev/null",
		"+++ b/local/frontend/src/lib/diffmerge.ts",
		"@@ -0,0 +1,12 @@",
		"+" + strings.Join(lines, "\n+"),
		"",
	}, "\n")

	contribs, exactLines, fileAgent, exactConversationByPath, remainingNorms, unmatchedNormsByPath := attributeCommitToMessages(commitTokens, messages, 0, 2000)
	if exactLines != 1 {
		t.Fatalf("exact matched lines = %d, want 1", exactLines)
	}
	if len(contribs) != 1 || contribs[0].ConversationID != "conv-local" {
		t.Fatalf("exact contribs = %+v, want only conv-local", contribs)
	}

	files := summarizeDiffFiles(parseUnifiedDiffTokensWithFiles(diffText, nil).Files, fileAgent)
	files, fallbackLines, fallbackConvIDs := applyFallbackFileCoverage(files, fileAgent, exactConversationByPath, unmatchedNormsByPath, remainingNorms, buildMessageIndex(messages, 0, 2000))
	if fallbackLines != len(lines)-1 {
		t.Fatalf("fallback lines = %d, want %d", fallbackLines, len(lines)-1)
	}
	if got, want := strings.Join(fallbackConvIDs, ","), "conv-old-path"; got != want {
		t.Fatalf("fallback conversation ids = %q, want %q", got, want)
	}
	if len(files) != 1 {
		t.Fatalf("files len = %d, want 1", len(files))
	}
	if got := files[0].LinesFromAgent; got != len(lines) {
		t.Fatalf("file linesFromAgent = %d, want %d", got, len(lines))
	}
}

func TestProjectCommitsPageAlwaysImportsLatestCommits(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	appPath := filepath.Join(repo, "app.txt")
	mustWriteFile(t, appPath, "start\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "add", "app.txt")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}

	// First call triggers async ingestion.
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/commits?page=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first call status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Wait for async ingestion to finish, then re-query.
	waitForCommitIngestion(t, s)

	req = httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/commits?page=1", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if !env.OK {
		t.Fatalf("first call ok=false, error=%v", env.Error)
	}
	firstData := env.Data.(map[string]any)
	firstPagination := firstData["pagination"].(map[string]any)
	if got := int(firstPagination["total"].(float64)); got != 1 {
		t.Fatalf("first pagination.total = %d, want 1", got)
	}

	mustWriteFile(t, appPath, "start\nsecond line\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "add", "app.txt")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "commit", "-m", "second commit")

	// Trigger async ingestion for the second commit.
	req = httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/commits?page=1", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second call status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Wait for async ingestion to finish, then re-query.
	waitForCommitIngestion(t, s)

	req = httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/commits?page=1", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	env = jsonEnvelope{}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if !env.OK {
		t.Fatalf("second call ok=false, error=%v", env.Error)
	}
	secondData := env.Data.(map[string]any)
	secondPagination := secondData["pagination"].(map[string]any)
	if got := int(secondPagination["total"].(float64)); got != 2 {
		t.Fatalf("second pagination.total = %d, want 2", got)
	}

	commits := secondData["commits"].([]any)
	foundSecond := false
	for _, raw := range commits {
		item := raw.(map[string]any)
		if item["subject"].(string) == "second commit" {
			foundSecond = true
			break
		}
	}
	if !foundSecond {
		t.Fatalf("second commit not found in project commits response")
	}
}

func TestProjectCommitsPageAutoHealsStaleCoverage(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	appPath := filepath.Join(repo, "app.txt")
	mustWriteFile(t, appPath, "start\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "add", "app.txt")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-heal", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	agentTs := mustUnixMilli(t, "2026-01-01T00:59:00Z")
	agentDiff := "```diff\n" +
		"diff --git a/app.txt b/app.txt\n" +
		"--- a/app.txt\n" +
		"+++ b/app.txt\n" +
		"@@ -1 +1,2 @@\n" +
		" start\n" +
		"+hello world\n" +
		"```"
	if err := db.InsertMessages(ctx, s.DB, []db.Message{{
		Timestamp:      agentTs,
		ProjectID:      projectID,
		ConversationID: "conv-heal",
		Role:           "agent",
		Content:        agentDiff,
		RawJSON:        agent.DerivedDiffRawJSON,
	}}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	mustWriteFile(t, appPath, "start\nhello world\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "add", "app.txt")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "commit", "-m", "agent change")
	agentCommitHash := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	// Prime ingestion (async).
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/commits?page=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("prime status = %d, want %d", rec.Code, http.StatusOK)
	}
	waitForCommitIngestion(t, s)
	waitForCommitRefresh(t, s)

	// Force stale persisted coverage for the commit and remove per-agent segments.
	if _, err := s.DB.ExecContext(ctx,
		`UPDATE commits
		 SET lines_from_agent = 0, coverage_version = 0
		 WHERE project_id = ? AND branch_name = ? AND commit_hash = ?`,
		projectID, "main", agentCommitHash,
	); err != nil {
		t.Fatalf("force stale commit coverage: %v", err)
	}
	if _, err := s.DB.ExecContext(ctx,
		`DELETE FROM commit_agent_coverage
		 WHERE commit_id = (SELECT id FROM commits WHERE project_id = ? AND branch_name = ? AND commit_hash = ?)`,
		projectID, "main", agentCommitHash,
	); err != nil {
		t.Fatalf("delete stale commit agent coverage: %v", err)
	}

	// Detail remains the source of truth.
	req = httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/commits/"+agentCommitHash+"?branch=main", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d", rec.Code, http.StatusOK)
	}
	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if !env.OK {
		t.Fatalf("detail ok=false, error=%v", env.Error)
	}
	detail := env.Data.(map[string]any)
	detailCommit := detail["commit"].(map[string]any)
	expectedLinesFromAgent := int(detailCommit["linesFromAgent"].(float64))
	if expectedLinesFromAgent <= 0 {
		t.Fatalf("detail linesFromAgent = %d, want > 0", expectedLinesFromAgent)
	}

	// List endpoint should return stale data immediately and signal isStale.
	// Background refresh handles recomputation asynchronously.
	req = httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/commits?page=1&branch=main", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", rec.Code, http.StatusOK)
	}
	env = jsonEnvelope{}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if !env.OK {
		t.Fatalf("list ok=false, error=%v", env.Error)
	}

	data := env.Data.(map[string]any)
	commits := data["commits"].([]any)
	found := false
	for _, raw := range commits {
		row := raw.(map[string]any)
		if row["commitHash"].(string) != agentCommitHash {
			continue
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("did not find commit %s in list response", agentCommitHash)
	}

	refresh := data["refresh"].(map[string]any)
	if isStale, ok := refresh["isStale"].(bool); !ok || !isStale {
		t.Fatalf("refresh.isStale = %v, want true", refresh["isStale"])
	}
}

func TestGetProjectCommit_RecoversWhenStoredDiffMissing(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	appPath := filepath.Join(repo, "app.txt")
	mustWriteFile(t, appPath, "start\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "add", "app.txt")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-recover", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	agentTs := mustUnixMilli(t, "2026-01-01T00:59:00Z")
	agentDiff := "```diff\n" +
		"diff --git a/app.txt b/app.txt\n" +
		"--- a/app.txt\n" +
		"+++ b/app.txt\n" +
		"@@ -1 +1,2 @@\n" +
		" start\n" +
		"+hello world\n" +
		"```"
	if err := db.InsertMessages(ctx, s.DB, []db.Message{{
		Timestamp:      agentTs,
		ProjectID:      projectID,
		ConversationID: "conv-recover",
		Role:           "agent",
		Content:        agentDiff,
		RawJSON:        agent.DerivedDiffRawJSON,
	}}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	mustWriteFile(t, appPath, "start\nhello world\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "add", "app.txt")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "commit", "-m", "agent change")
	agentCommitHash := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	// Prime ingestion into DB.
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/commits?page=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("prime status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Simulate bad persisted state: coverage exists but stored diff is empty.
	if _, err := s.DB.ExecContext(ctx,
		`UPDATE commits
		 SET diff_content = '', coverage_version = ?
		 WHERE project_id = ? AND branch_name = ? AND commit_hash = ?`,
		currentCommitCoverageVersion, projectID, "main", agentCommitHash,
	); err != nil {
		t.Fatalf("clear diff_content: %v", err)
	}

	req = httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/commits/"+agentCommitHash+"?branch=main", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if !env.OK {
		t.Fatalf("detail ok=false, error=%v", env.Error)
	}
	data := env.Data.(map[string]any)
	commit := data["commit"].(map[string]any)
	if got := int(commit["linesTotal"].(float64)); got <= 0 {
		t.Fatalf("detail linesTotal = %d, want > 0", got)
	}
	if got := int(commit["linesFromAgent"].(float64)); got <= 0 {
		t.Fatalf("detail linesFromAgent = %d, want > 0", got)
	}
	if got := len(data["files"].([]any)); got <= 0 {
		t.Fatalf("detail files = %d, want > 0", got)
	}
	if got := len(data["messages"].([]any)); got <= 0 {
		t.Fatalf("detail messages = %d, want > 0", got)
	}
}

func TestListProjectCommitsIgnoresConfiguredDiffPaths(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	appPath := filepath.Join(repo, "app.txt")
	agentsPath := filepath.Join(repo, "AGENTS.md")
	mustWriteFile(t, appPath, "start\n")
	mustWriteFile(t, agentsPath, "old rules\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "add", "app.txt", "AGENTS.md")
	gitRun(t, repo, []string{
		"GIT_AUTHOR_NAME=Other User",
		"GIT_AUTHOR_EMAIL=other@example.com",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	}, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	pid, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, pid, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.SetProjectIgnoreDiffPaths(ctx, s.DB, pid, "AGENTS.md"); err != nil {
		t.Fatalf("SetProjectIgnoreDiffPaths: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-1", pid, "codex"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	agentTs := mustUnixMilli(t, "2026-01-01T00:59:00Z")
	agentDiff := "```diff\n" +
		"diff --git a/app.txt b/app.txt\n" +
		"--- a/app.txt\n" +
		"+++ b/app.txt\n" +
		"@@ -1 +1,2 @@\n" +
		" start\n" +
		"+hello world\n" +
		"diff --git a/AGENTS.md b/AGENTS.md\n" +
		"--- a/AGENTS.md\n" +
		"+++ b/AGENTS.md\n" +
		"@@ -1 +1 @@\n" +
		"-old rules\n" +
		"+new rules\n" +
		"```"
	if err := db.InsertMessages(ctx, s.DB, []db.Message{{
		Timestamp:      agentTs,
		ProjectID:      pid,
		ConversationID: "conv-1",
		Role:           "agent",
		Content:        agentDiff,
		RawJSON:        agent.DerivedDiffRawJSON,
	}}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	mustWriteFile(t, appPath, "start\nhello world\n")
	mustWriteFile(t, agentsPath, "new rules\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "add", "app.txt", "AGENTS.md")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T01:00:00Z"}, "commit", "-m", "mixed change")

	req := httptest.NewRequest("GET", "/api/v1/projects/commits", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok=false, error=%v", env.Error)
	}

	data := env.Data.(map[string]any)
	commits := data["commits"].([]any)
	// Now includes all users' commits: initial (Other User) + mixed change.
	if len(commits) != 2 {
		t.Fatalf("commits = %d, want 2", len(commits))
	}
	bySubject := map[string]map[string]any{}
	for _, raw := range commits {
		item := raw.(map[string]any)
		bySubject[item["subject"].(string)] = item
	}
	commit := bySubject["mixed change"]
	if commit == nil {
		t.Fatalf("missing commit subject %q", "mixed change")
	}
	if got := int(commit["linesTotal"].(float64)); got != 1 {
		t.Fatalf("linesTotal = %d, want 1 after ignoring AGENTS.md", got)
	}
	if got := int(commit["linesFromAgent"].(float64)); got != 1 {
		t.Fatalf("linesFromAgent = %d, want 1", got)
	}
}

func TestSummarizeDiffFiles_ExactUsesTokenTotalsAndFallbackCopyStillApplies(t *testing.T) {
	diffText := strings.Join([]string{
		"diff --git a/exact.txt b/exact.txt",
		"--- a/exact.txt",
		"+++ b/exact.txt",
		"@@ -0,0 +1,3 @@",
		"+alpha",
		"+   ",
		"+beta",
		"diff --git a/copy.txt b/copy.txt",
		"--- a/copy.txt",
		"+++ b/copy.txt",
		"@@ -0,0 +1,10 @@",
		"+c1",
		"+c2",
		"+c3",
		"+c4",
		"+c5",
		"+c6",
		"+c7",
		"+c8",
		"+c9",
		"+c10",
		"",
	}, "\n")

	commitTokens := []diffToken{
		testToken("exact.txt", '+', "alpha", 5),
		testToken("exact.txt", '+', "beta", 4),
		testToken("copy.txt", '+', "c1", 2),
		testToken("copy.txt", '+', "c2", 2),
		testToken("copy.txt", '+', "c3", 2),
		testToken("copy.txt", '+', "c4", 2),
		testToken("copy.txt", '+', "c5", 2),
		testToken("copy.txt", '+', "c6", 2),
		testToken("copy.txt", '+', "c7", 2),
		testToken("copy.txt", '+', "c8", 2),
		testToken("copy.txt", '+', "c9", 2),
		testToken("copy.txt", '+', "c10", 3),
	}

	fileAgent := map[string]commitFileCoverage{
		"exact.txt": {
			Path:    "exact.txt",
			Added:   2,
			Removed: 2,
		},
	}

	remainingNorms := map[string]int{
		"c1":  1,
		"c2":  1,
		"c3":  1,
		"c4":  1,
		"c5":  1,
		"c6":  1,
		"c7":  1,
		"c8":  1,
		"c9":  1,
		"c10": 1,
	}

	files := summarizeDiffFiles(parseUnifiedDiffTokensWithFiles(diffText, nil).Files, fileAgent)
	files, _, _ = applyFallbackFileCoverage(files, fileAgent, nil, normsByPathFromTokens(commitTokens), remainingNorms, &messageIndex{})
	if len(files) != 2 {
		t.Fatalf("files len = %d, want 2", len(files))
	}

	byPath := make(map[string]commitFileCoverage, len(files))
	for _, f := range files {
		byPath[f.Path] = f
	}

	exact := byPath["exact.txt"]
	if exact.LinesTotal != 3 {
		t.Fatalf("exact linesTotal = %d, want 3 raw lines", exact.LinesTotal)
	}
	if exact.LinesFromAgent != 2 {
		t.Fatalf("exact linesFromAgent = %d, want 2", exact.LinesFromAgent)
	}
	if exact.LinePercent != 100 {
		t.Fatalf("exact linePercent = %.1f, want 100.0", exact.LinePercent)
	}

	copyFile := byPath["copy.txt"]
	if copyFile.LinesTotal != 10 {
		t.Fatalf("copy linesTotal = %d, want 10", copyFile.LinesTotal)
	}
	if !copyFile.CopiedFromAgent {
		t.Fatalf("copy copiedFromAgent = %v, want true", copyFile.CopiedFromAgent)
	}
	if copyFile.LinesFromAgent != 10 {
		t.Fatalf("copy linesFromAgent = %d, want 10", copyFile.LinesFromAgent)
	}
	if copyFile.LinePercent != 100 {
		t.Fatalf("copy linePercent = %.1f, want 100.0", copyFile.LinePercent)
	}
}

func TestSummarizeDiffFiles_CopiedFallbackUsesFullNormPool(t *testing.T) {
	commitTokens := []diffToken{
		testToken("exact.txt", '+', "c1", 2),
		testToken("exact.txt", '+', "c2", 2),
		testToken("exact.txt", '+', "c3", 2),
		testToken("exact.txt", '+', "c4", 2),
		testToken("exact.txt", '+', "c5", 2),
		testToken("exact.txt", '+', "c6", 2),
		testToken("exact.txt", '+', "c7", 2),
		testToken("exact.txt", '+', "c8", 2),
		testToken("exact.txt", '+', "c9", 2),
		testToken("exact.txt", '+', "c10", 3),
		testToken("copy.txt", '+', "c1", 2),
		testToken("copy.txt", '+', "c2", 2),
		testToken("copy.txt", '+', "c3", 2),
		testToken("copy.txt", '+', "c4", 2),
		testToken("copy.txt", '+', "c5", 2),
		testToken("copy.txt", '+', "c6", 2),
		testToken("copy.txt", '+', "c7", 2),
		testToken("copy.txt", '+', "c8", 2),
		testToken("copy.txt", '+', "c9", 2),
		testToken("copy.txt", '+', "c10", 3),
	}
	messages := []messageDiff{
		{
			ID:        "m1",
			Timestamp: 1000,
			Tokens: []diffToken{
				testToken("exact.txt", '+', "c1", 2),
				testToken("exact.txt", '+', "c2", 2),
				testToken("exact.txt", '+', "c3", 2),
				testToken("exact.txt", '+', "c4", 2),
				testToken("exact.txt", '+', "c5", 2),
				testToken("exact.txt", '+', "c6", 2),
				testToken("exact.txt", '+', "c7", 2),
				testToken("exact.txt", '+', "c8", 2),
				testToken("exact.txt", '+', "c9", 2),
				testToken("exact.txt", '+', "c10", 3),
			},
		},
	}

	diffText := strings.Join([]string{
		"diff --git a/exact.txt b/exact.txt",
		"--- a/exact.txt",
		"+++ b/exact.txt",
		"@@ -0,0 +1,10 @@",
		"+c1",
		"+c2",
		"+c3",
		"+c4",
		"+c5",
		"+c6",
		"+c7",
		"+c8",
		"+c9",
		"+c10",
		"diff --git a/copy.txt b/copy.txt",
		"--- a/copy.txt",
		"+++ b/copy.txt",
		"@@ -0,0 +1,10 @@",
		"+c1",
		"+c2",
		"+c3",
		"+c4",
		"+c5",
		"+c6",
		"+c7",
		"+c8",
		"+c9",
		"+c10",
		"",
	}, "\n")

	_, _, fileAgent, exactConversationByPath, normCounts, unmatchedNormsByPath := attributeCommitToMessages(commitTokens, messages, 0, 2000)
	files := summarizeDiffFiles(parseUnifiedDiffTokensWithFiles(diffText, nil).Files, fileAgent)
	files, _, _ = applyFallbackFileCoverage(files, fileAgent, exactConversationByPath, unmatchedNormsByPath, normCounts, buildMessageIndex(messages, 0, 2000))

	byPath := make(map[string]commitFileCoverage, len(files))
	for _, f := range files {
		byPath[f.Path] = f
	}
	copyFile := byPath["copy.txt"]
	if !copyFile.CopiedFromAgent {
		t.Fatalf("copy copiedFromAgent = %v, want true", copyFile.CopiedFromAgent)
	}
	if copyFile.LinesFromAgent != 10 {
		t.Fatalf("copy linesFromAgent = %d, want 10", copyFile.LinesFromAgent)
	}
	if copyFile.LinePercent != 100 {
		t.Fatalf("copy linePercent = %.1f, want 100.0", copyFile.LinePercent)
	}
}

func TestAttributeCommitToMessages_MatchesFormattingOnlyLineWraps(t *testing.T) {
	commitTokens := []diffToken{
		testToken("src/app.js", '+', "constresult=foo(bar,baz);", 25),
		testToken("src/reflow.js", '+', "returnalpha+", 12),
		testToken("src/reflow.js", '+', "beta+gamma;", 11),
	}

	messages := []messageDiff{
		{
			ID:        "m1",
			Timestamp: 1000,
			Tokens: []diffToken{
				testToken("src/app.js", '+', "constresult=foo(", 16),
				testToken("src/app.js", '+', "bar,baz);", 9),
				testToken("src/reflow.js", '+', "returnalpha+beta+gamma;", 23),
			},
		},
	}

	contrib, lines, fileAgent, _, _, _ := attributeCommitToMessages(commitTokens, messages, 0, 2000)
	if lines != 3 {
		t.Fatalf("matched lines = %d, want 3", lines)
	}
	if len(contrib) != 1 {
		t.Fatalf("contrib len = %d, want 1", len(contrib))
	}
	if contrib[0].LinesMatched != 3 {
		t.Fatalf("contrib lines = %d, want 3", contrib[0].LinesMatched)
	}

	if fileAgent["src/app.js"].Removed != 1 {
		t.Fatalf("src/app.js removed = %d, want 1", fileAgent["src/app.js"].Removed)
	}
	if fileAgent["src/reflow.js"].Removed != 2 {
		t.Fatalf("src/reflow.js removed = %d, want 2", fileAgent["src/reflow.js"].Removed)
	}
}

func TestSummarizeDiffFiles_IncludesPerFileAgentSegments(t *testing.T) {
	commitTokens := []diffToken{
		testToken("src/app.ts", '+', "line-a", 6),
		testToken("src/app.ts", '+', "line-b", 6),
	}
	messages := []messageDiff{
		{
			ID:        "m1",
			Timestamp: 1000,
			Agent:     "codex",
			Tokens: []diffToken{
				testToken("src/app.ts", '+', "line-a", 6),
			},
		},
		{
			ID:        "m2",
			Timestamp: 1000,
			Agent:     "claude",
			Tokens: []diffToken{
				testToken("src/app.ts", '+', "line-b", 6),
			},
		},
	}
	diffText := strings.Join([]string{
		"diff --git a/src/app.ts b/src/app.ts",
		"--- a/src/app.ts",
		"+++ b/src/app.ts",
		"@@ -0,0 +1,2 @@",
		"+line-a",
		"+line-b",
		"",
	}, "\n")

	_, _, fileAgent, exactConversationByPath, remainingNorms, unmatchedNormsByPath := attributeCommitToMessages(commitTokens, messages, 0, 2000)
	files := summarizeDiffFiles(parseUnifiedDiffTokensWithFiles(diffText, nil).Files, fileAgent)
	files, _, _ = applyFallbackFileCoverage(files, fileAgent, exactConversationByPath, unmatchedNormsByPath, remainingNorms, buildMessageIndex(messages, 0, 2000))
	if len(files) != 1 {
		t.Fatalf("files len = %d, want 1", len(files))
	}
	f := files[0]
	if f.LinePercent != 100 {
		t.Fatalf("linePercent = %.1f, want 100.0", f.LinePercent)
	}
	if len(f.AgentSegments) != 2 {
		t.Fatalf("agentSegments len = %d, want 2", len(f.AgentSegments))
	}

	byAgent := make(map[string]agentCoverageSegment, len(f.AgentSegments))
	for _, seg := range f.AgentSegments {
		byAgent[seg.Agent] = seg
	}
	if byAgent["codex"].LinesFromAgent != 1 || byAgent["codex"].LinePercent != 50 {
		t.Fatalf("codex segment = %+v, want lines=1 percent=50", byAgent["codex"])
	}
	if byAgent["claude"].LinesFromAgent != 1 || byAgent["claude"].LinePercent != 50 {
		t.Fatalf("claude segment = %+v, want lines=1 percent=50", byAgent["claude"])
	}
}

func TestAttributeCommitToMessages_DeletionMatchesDeletingAgent(t *testing.T) {
	commitTokens := []diffToken{
		testToken("src/app.ts", '-', "oldline", 7),
	}
	messages := []messageDiff{
		{
			ID:        "m-old",
			Timestamp: 900,
			Agent:     "claude",
			Tokens: []diffToken{
				testToken("src/app.ts", '+', "oldline", 7),
			},
		},
		{
			ID:        "m-new",
			Timestamp: 1000,
			Agent:     "codex",
			Tokens: []diffToken{
				testToken("src/app.ts", '-', "oldline", 7),
			},
		},
	}

	contrib, lines, _, _, _, _ := attributeCommitToMessages(commitTokens, messages, 0, 2000)
	if lines != 1 {
		t.Fatalf("matched lines = %d, want 1", lines)
	}
	if len(contrib) != 1 {
		t.Fatalf("contrib len = %d, want 1", len(contrib))
	}
	if contrib[0].Agent != "codex" {
		t.Fatalf("matched agent = %q, want %q", contrib[0].Agent, "codex")
	}
}

func TestAttributeCommitToMessages_PrefersNewerMessage(t *testing.T) {
	// When two messages from different agents contain the same token,
	// the newer message should be preferred for attribution.
	commitTokens := []diffToken{
		testToken("src/app.ts", '+', "sharedline", 10),
	}
	messages := []messageDiff{
		{
			ID:             "m-old",
			Timestamp:      1000,
			ConversationID: "conv-old",
			Agent:          "codex",
			Tokens: []diffToken{
				testToken("src/app.ts", '+', "sharedline", 10),
			},
		},
		{
			ID:             "m-new",
			Timestamp:      5000,
			ConversationID: "conv-new",
			Agent:          "claude",
			Tokens: []diffToken{
				testToken("src/app.ts", '+', "sharedline", 10),
			},
		},
	}

	contrib, lines, _, _, _, _ := attributeCommitToMessages(commitTokens, messages, 0, 10000)
	if lines != 1 {
		t.Fatalf("matched lines = %d, want 1", lines)
	}
	if len(contrib) != 1 {
		t.Fatalf("contrib len = %d, want 1", len(contrib))
	}
	if contrib[0].ID != "m-new" {
		t.Fatalf("matched message = %q, want %q (newer message)", contrib[0].ID, "m-new")
	}
	if contrib[0].Agent != "claude" {
		t.Fatalf("matched agent = %q, want %q", contrib[0].Agent, "claude")
	}
}

func TestAttributeCommitToMessages_PrefersLongestPathSuffixOverNewerMessage(t *testing.T) {
	commitTokens := []diffToken{
		testToken("local/frontend/src/app.html", '+', "sharedline", 10),
	}
	messages := []messageDiff{
		{
			ID:             "m-absolute",
			Timestamp:      1000,
			ConversationID: "conv-current",
			Agent:          "claude_cloud",
			Tokens: []diffToken{
				testToken("home/user/buildermark/local/frontend/src/app.html", '+', "sharedline", 10),
			},
		},
		{
			ID:             "m-shorter",
			Timestamp:      5000,
			ConversationID: "conv-other",
			Agent:          "claude",
			Tokens: []diffToken{
				testToken("src/app.html", '+', "sharedline", 10),
			},
		},
	}

	contrib, lines, _, _, _, _ := attributeCommitToMessages(commitTokens, messages, 0, 10000)
	if lines != 1 {
		t.Fatalf("matched lines = %d, want 1", lines)
	}
	if len(contrib) != 1 {
		t.Fatalf("contrib len = %d, want 1", len(contrib))
	}
	if contrib[0].ConversationID != "conv-current" {
		t.Fatalf("matched conversation = %q, want %q", contrib[0].ConversationID, "conv-current")
	}
	if contrib[0].Agent != "claude_cloud" {
		t.Fatalf("matched agent = %q, want %q", contrib[0].Agent, "claude_cloud")
	}
}

func TestAttributeCommitToMessages_MatchesCSSQuoteStyleOnlyChanges(t *testing.T) {
	commitTokens := []diffToken{
		testToken("local/frontend/src/routes/local.css", '+', `html[data-theme='light']{`, 26),
	}
	messages := []messageDiff{
		{
			ID:             "m-css",
			Timestamp:      1000,
			ConversationID: "conv-current",
			Agent:          "claude_cloud",
			Tokens: []diffToken{
				testToken("home/user/buildermark/local/frontend/src/routes/local.css", '+', `html[data-theme="light"]{`, 26),
			},
		},
	}

	contrib, lines, _, _, _, _ := attributeCommitToMessages(commitTokens, messages, 0, 2000)
	if lines != 1 {
		t.Fatalf("matched lines = %d, want 1", lines)
	}
	if len(contrib) != 1 {
		t.Fatalf("contrib len = %d, want 1", len(contrib))
	}
	if contrib[0].ConversationID != "conv-current" {
		t.Fatalf("matched conversation = %q, want %q", contrib[0].ConversationID, "conv-current")
	}
}

func TestAttributeCommitToMessages_StylePassPrefersConversationAlreadyMatchedInFile(t *testing.T) {
	commitTokens := []diffToken{
		testToken("local/frontend/src/routes/local.css", '+', `:root{color-scheme:lightdark;}`, 30),
		testToken("local/frontend/src/routes/local.css", '+', `html[data-theme='light']{`, 26),
	}
	messages := []messageDiff{
		{
			ID:             "m-current",
			Timestamp:      1000,
			ConversationID: "conv-current",
			Agent:          "claude_cloud",
			Tokens: []diffToken{
				testToken("home/user/buildermark/local/frontend/src/routes/local.css", '+', `:root{color-scheme:lightdark;}`, 30),
				testToken("home/user/buildermark/local/frontend/src/routes/local.css", '+', `html[data-theme="light"]{`, 26),
			},
		},
		{
			ID:             "m-newer",
			Timestamp:      5000,
			ConversationID: "conv-newer",
			Agent:          "claude",
			Tokens: []diffToken{
				testToken("local/frontend/src/routes/local.css", '+', `html[data-theme="light"]{`, 26),
			},
		},
	}

	contrib, lines, _, _, _, _ := attributeCommitToMessages(commitTokens, messages, 0, 10000)
	if lines != 2 {
		t.Fatalf("matched lines = %d, want 2", lines)
	}
	if len(contrib) != 1 {
		t.Fatalf("contrib len = %d, want 1", len(contrib))
	}
	if contrib[0].ConversationID != "conv-current" {
		t.Fatalf("matched conversation = %q, want %q", contrib[0].ConversationID, "conv-current")
	}
	if contrib[0].LinesMatched != 2 {
		t.Fatalf("matched lines for current conversation = %d, want 2", contrib[0].LinesMatched)
	}
}

func TestApplyFallbackFileCoverage_PrefersConversationWithExactMatchInSameFile(t *testing.T) {
	diffText := strings.Join([]string{
		"diff --git a/local/frontend/src/routes/settings/+page.svelte b/local/frontend/src/routes/settings/+page.svelte",
		"--- a/local/frontend/src/routes/settings/+page.svelte",
		"+++ b/local/frontend/src/routes/settings/+page.svelte",
		"@@ -0,0 +1,11 @@",
		"+themeExact",
		"+shared1",
		"+shared2",
		"+shared3",
		"+shared4",
		"+shared5",
		"+shared6",
		"+shared7",
		"+shared8",
		"+shared9",
		"+shared10",
		"",
	}, "\n")

	commitTokens := []diffToken{
		testToken("local/frontend/src/routes/settings/+page.svelte", '+', "themeExact", 10),
	}
	for i := 1; i <= 10; i++ {
		commitTokens = append(commitTokens, testToken(
			"local/frontend/src/routes/settings/+page.svelte",
			'+',
			fmt.Sprintf("shared%d", i),
			7,
		))
	}

	currentTokens := []diffToken{
		testToken("home/user/buildermark/local/frontend/src/routes/settings/+page.svelte", '+', "themeExact", 10),
	}
	oldSharedTokens := make([]diffToken, 0, 10)
	currentSharedTokens := make([]diffToken, 0, 10)
	for i := 1; i <= 10; i++ {
		norm := fmt.Sprintf("shared%d", i)
		oldSharedTokens = append(oldSharedTokens, testToken("snippets/settings-block.txt", '+', norm, len(norm)))
		currentSharedTokens = append(currentSharedTokens, testToken("drafts/theme-block.txt", '+', norm, len(norm)))
	}

	messages := []messageDiff{
		{
			ID:             "m-current-exact",
			Timestamp:      3000,
			ConversationID: "conv-current",
			Agent:          "claude_cloud",
			Tokens:         currentTokens,
		},
		{
			ID:             "m-current-fallback",
			Timestamp:      2500,
			ConversationID: "conv-current",
			Agent:          "claude_cloud",
			Tokens:         currentSharedTokens,
		},
		{
			ID:             "m-old-fallback",
			Timestamp:      2000,
			ConversationID: "conv-older",
			Agent:          "claude",
			Tokens:         oldSharedTokens,
		},
	}

	_, _, fileAgent, exactConversationByPath, remainingNorms, unmatchedNormsByPath := attributeCommitToMessages(commitTokens, messages, 0, 10000)
	files := summarizeDiffFiles(parseUnifiedDiffTokensWithFiles(diffText, nil).Files, fileAgent)
	files, fallbackLines, fallbackConvIDs := applyFallbackFileCoverage(
		files,
		fileAgent,
		exactConversationByPath,
		unmatchedNormsByPath,
		remainingNorms,
		buildMessageIndex(messages, 0, 10000),
	)

	if fallbackLines != 10 {
		t.Fatalf("fallback lines = %d, want 10", fallbackLines)
	}
	if got, want := strings.Join(fallbackConvIDs, ","), "conv-current"; got != want {
		t.Fatalf("fallback conversation ids = %q, want %q", got, want)
	}
	if len(files) != 1 {
		t.Fatalf("files len = %d, want 1", len(files))
	}
	file := files[0]
	if !file.CopiedFromAgent {
		t.Fatalf("copiedFromAgent = false, want true")
	}
	if file.LinesFromAgent != 11 {
		t.Fatalf("linesFromAgent = %d, want 11", file.LinesFromAgent)
	}

	byAgent := make(map[string]int, len(file.AgentSegments))
	for _, seg := range file.AgentSegments {
		byAgent[seg.Agent] = seg.LinesFromAgent
	}
	if got := byAgent["claude_cloud"]; got != 11 {
		t.Fatalf("claude_cloud lines = %d, want 11", got)
	}
	if got := byAgent["claude"]; got != 0 {
		t.Fatalf("claude lines = %d, want 0", got)
	}
}

func TestNormalizeStyleEquivalentLine_DoesNotApplyToJSON(t *testing.T) {
	line := `"theme": "dark"`
	norm := normalizeWhitespace(line)
	if got := normalizeStyleEquivalentLine("config.json", line, norm); got != "" {
		t.Fatalf("style norm for json = %q, want empty", got)
	}
}

func TestAttributeCommitToMessages_FormattingPassPrefersNewerMessage(t *testing.T) {
	// When a formatting-only match could match multiple messages,
	// the newer one should win.
	commitTokens := []diffToken{
		testToken("src/app.js", '+', "constx=foo(bar,baz);", 20),
	}
	messages := []messageDiff{
		{
			ID:             "m-old",
			Timestamp:      1000,
			ConversationID: "conv-old",
			Agent:          "codex",
			Tokens: []diffToken{
				testToken("src/app.js", '+', "constx=foo(", 11),
				testToken("src/app.js", '+', "bar,baz);", 9),
			},
		},
		{
			ID:             "m-new",
			Timestamp:      5000,
			ConversationID: "conv-new",
			Agent:          "claude",
			Tokens: []diffToken{
				testToken("src/app.js", '+', "constx=foo(", 11),
				testToken("src/app.js", '+', "bar,baz);", 9),
			},
		},
	}

	contrib, lines, _, _, _, _ := attributeCommitToMessages(commitTokens, messages, 0, 10000)
	if lines != 1 {
		t.Fatalf("matched lines = %d, want 1", lines)
	}
	if len(contrib) != 1 {
		t.Fatalf("contrib len = %d, want 1", len(contrib))
	}
	if contrib[0].ID != "m-new" {
		t.Fatalf("formatting pass matched message = %q, want %q (newer message)", contrib[0].ID, "m-new")
	}
}

func TestParseUnifiedDiffTokens_IgnoresPunctuationOnlyForAttribution(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/src/app.ts b/src/app.ts",
		"--- a/src/app.ts",
		"+++ b/src/app.ts",
		"@@ -1,2 +1,2 @@",
		"-}",
		"+}",
		"-value",
		"+nextValue",
		"",
	}, "\n")

	tokens := parseUnifiedDiffTokens(diff, nil)
	if len(tokens) != 4 {
		t.Fatalf("tokens len = %d, want 4", len(tokens))
	}
	if tokens[0].Attributable || tokens[1].Attributable {
		t.Fatalf("punctuation-only tokens should not be attributable")
	}
	if !tokens[2].Attributable || !tokens[3].Attributable {
		t.Fatalf("identifier tokens should be attributable")
	}
}

func TestBuildDailySummary(t *testing.T) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Create commits on today, yesterday, and 10 days ago.
	commits := []projectCommitCoverage{
		{
			ProjectID:        "p1",
			CommitHash:       "aaa",
			Subject:          "first today",
			AuthoredAtUnixMs: today.Add(2 * time.Hour).UnixMilli(),
			LinesTotal:       10,
			LinesFromAgent:   6,
			AgentSegments: []agentCoverageSegment{
				{Agent: "claude", LinesFromAgent: 4},
				{Agent: "codex", LinesFromAgent: 2},
			},
		},
		{
			ProjectID:        "p1",
			CommitHash:       "bbb",
			Subject:          "second today",
			AuthoredAtUnixMs: today.Add(5 * time.Hour).UnixMilli(),
			LinesTotal:       20,
			LinesFromAgent:   10,
			AgentSegments: []agentCoverageSegment{
				{Agent: "claude", LinesFromAgent: 10},
			},
		},
		{
			ProjectID:        "p1",
			CommitHash:       "ccc",
			Subject:          "yesterday",
			AuthoredAtUnixMs: today.Add(-20 * time.Hour).UnixMilli(),
			LinesTotal:       5,
			LinesFromAgent:   3,
			AgentSegments: []agentCoverageSegment{
				{Agent: "codex", LinesFromAgent: 3},
			},
		},
		{
			ProjectID:        "p1",
			CommitHash:       "ddd",
			Subject:          "ten days ago",
			AuthoredAtUnixMs: today.AddDate(0, 0, -10).Add(3 * time.Hour).UnixMilli(),
			LinesTotal:       8,
			LinesFromAgent:   0,
		},
	}

	result := buildDailySummary(commits, 30, time.UTC)

	// Should return exactly 30 entries.
	if len(result) != 30 {
		t.Fatalf("len = %d, want 30", len(result))
	}

	// Last entry should be today's date.
	todayStr := today.Format("2006-01-02")
	if result[29].Date != todayStr {
		t.Fatalf("last date = %q, want %q", result[29].Date, todayStr)
	}

	// First entry should be 29 days ago.
	firstDate := today.AddDate(0, 0, -29).Format("2006-01-02")
	if result[0].Date != firstDate {
		t.Fatalf("first date = %q, want %q", result[0].Date, firstDate)
	}

	// Today's entry (index 29) should aggregate both commits.
	todayEntry := result[29]
	if todayEntry.LinesTotal != 30 {
		t.Fatalf("today linesTotal = %d, want 30", todayEntry.LinesTotal)
	}
	if todayEntry.LinesFromAgent != 16 {
		t.Fatalf("today linesFromAgent = %d, want 16", todayEntry.LinesFromAgent)
	}
	if len(todayEntry.Commits) != 2 {
		t.Fatalf("today commits = %d, want 2", len(todayEntry.Commits))
	}
	// Agent segments should be aggregated: claude=14, codex=2.
	if len(todayEntry.AgentSegments) != 2 {
		t.Fatalf("today agentSegments = %d, want 2", len(todayEntry.AgentSegments))
	}
	agentMap := make(map[string]int)
	for _, seg := range todayEntry.AgentSegments {
		agentMap[seg.Agent] = seg.LinesFromAgent
	}
	if agentMap["claude"] != 14 {
		t.Fatalf("today claude lines = %d, want 14", agentMap["claude"])
	}
	if agentMap["codex"] != 2 {
		t.Fatalf("today codex lines = %d, want 2", agentMap["codex"])
	}

	// Yesterday's entry (index 28).
	yesterdayEntry := result[28]
	if yesterdayEntry.LinesTotal != 5 {
		t.Fatalf("yesterday linesTotal = %d, want 5", yesterdayEntry.LinesTotal)
	}
	if len(yesterdayEntry.Commits) != 1 {
		t.Fatalf("yesterday commits = %d, want 1", len(yesterdayEntry.Commits))
	}

	// Ten days ago entry (index 19).
	tenDaysAgoEntry := result[19]
	expectedDate := today.AddDate(0, 0, -10).Format("2006-01-02")
	if tenDaysAgoEntry.Date != expectedDate {
		t.Fatalf("ten days ago date = %q, want %q", tenDaysAgoEntry.Date, expectedDate)
	}
	if tenDaysAgoEntry.LinesTotal != 8 {
		t.Fatalf("ten days ago linesTotal = %d, want 8", tenDaysAgoEntry.LinesTotal)
	}
	if tenDaysAgoEntry.LinesFromAgent != 0 {
		t.Fatalf("ten days ago linesFromAgent = %d, want 0", tenDaysAgoEntry.LinesFromAgent)
	}

	// Empty days should have zero values and empty commits slice.
	emptyEntry := result[0]
	if emptyEntry.LinesTotal != 0 {
		t.Fatalf("empty day linesTotal = %d, want 0", emptyEntry.LinesTotal)
	}
	if len(emptyEntry.Commits) != 0 {
		t.Fatalf("empty day commits = %d, want 0", len(emptyEntry.Commits))
	}
}

func TestBuildDailySummaryWindow_CustomWindow(t *testing.T) {
	loc := time.UTC
	windowEnd := time.Date(2026, time.January, 10, 12, 0, 0, 0, loc)
	commits := []projectCommitCoverage{
		{
			ProjectID:        "p1",
			CommitHash:       "in-window",
			Subject:          "inside",
			AuthoredAtUnixMs: time.Date(2026, time.January, 9, 8, 0, 0, 0, loc).UnixMilli(),
			LinesTotal:       5,
			LinesFromAgent:   2,
		},
		{
			ProjectID:        "p1",
			CommitHash:       "out-window",
			Subject:          "outside",
			AuthoredAtUnixMs: time.Date(2025, time.December, 25, 8, 0, 0, 0, loc).UnixMilli(),
			LinesTotal:       7,
			LinesFromAgent:   3,
		},
	}

	result := buildDailySummaryWindow(commits, 7, loc, &windowEnd, false)
	if len(result) != 7 {
		t.Fatalf("len = %d, want 7", len(result))
	}

	if got := result[0].Date; got != "2026-01-04" {
		t.Fatalf("first date = %q, want %q", got, "2026-01-04")
	}
	if got := result[6].Date; got != "2026-01-10" {
		t.Fatalf("last date = %q, want %q", got, "2026-01-10")
	}
	if got := result[5].LinesTotal; got != 5 {
		t.Fatalf("day before end linesTotal = %d, want 5", got)
	}
	if got := result[5].LinesFromAgent; got != 2 {
		t.Fatalf("day before end linesFromAgent = %d, want 2", got)
	}
}

func TestBuildDailySummaryWindow_UsesOverrideAgentPercents(t *testing.T) {
	loc := time.UTC
	windowEnd := time.Date(2026, time.March, 21, 12, 0, 0, 0, loc)

	overrideCommit := effectiveCommitCoverage(
		projectCommitCoverage{
			ProjectID:             "p1",
			CommitHash:            "override",
			Subject:               "override commit",
			AuthoredAtUnixMs:      time.Date(2026, time.March, 21, 8, 0, 0, 0, loc).UnixMilli(),
			LinesTotal:            961,
			LinesFromAgent:        738,
			LinePercent:           percentage(738, 961),
			OverrideAgentPercents: map[string]int{"claude": 100},
		},
		[]agentCoverageSegment{{
			Agent:          "claude",
			LinesFromAgent: 738,
			LinePercent:    percentage(738, 961),
		}},
	)
	if got := overrideCommit.LinesFromAgent; got != 961 {
		t.Fatalf("override linesFromAgent = %d, want 961", got)
	}
	if len(overrideCommit.AgentSegments) != 1 || overrideCommit.AgentSegments[0].LinesFromAgent != 961 {
		t.Fatalf("override agent segments = %#v, want claude=961", overrideCommit.AgentSegments)
	}

	normalCommit := effectiveCommitCoverage(
		projectCommitCoverage{
			ProjectID:        "p1",
			CommitHash:       "normal",
			Subject:          "normal commit",
			AuthoredAtUnixMs: time.Date(2026, time.March, 21, 9, 0, 0, 0, loc).UnixMilli(),
			LinesTotal:       5,
			LinesFromAgent:   4,
			LinePercent:      percentage(4, 5),
		},
		[]agentCoverageSegment{{
			Agent:          "claude",
			LinesFromAgent: 4,
			LinePercent:    percentage(4, 5),
		}},
	)

	result := buildDailySummaryWindow([]projectCommitCoverage{overrideCommit, normalCommit}, 1, loc, &windowEnd, false)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}

	day := result[0]
	if got := day.LinesTotal; got != 966 {
		t.Fatalf("linesTotal = %d, want 966", got)
	}
	if got := day.LinesFromAgent; got != 965 {
		t.Fatalf("linesFromAgent = %d, want 965", got)
	}
	if len(day.AgentSegments) != 1 {
		t.Fatalf("agentSegments len = %d, want 1", len(day.AgentSegments))
	}
	if got := day.AgentSegments[0].Agent; got != "claude" {
		t.Fatalf("agent = %q, want %q", got, "claude")
	}
	if got := day.AgentSegments[0].LinesFromAgent; got != 965 {
		t.Fatalf("agent linesFromAgent = %d, want 965", got)
	}
}

func TestGetFileTypeCoverage_UsesOverrideAgentPercents(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	detailFiles, err := json.Marshal([]commitFileCoverage{{
		Path:       "cmd/app/main.go",
		LinesTotal: 10,
		AgentSegments: []agentCoverageSegment{{
			Agent:          "claude",
			LinesFromAgent: 7,
			LinePercent:    70,
		}},
	}})
	if err != nil {
		t.Fatalf("Marshal detail files: %v", err)
	}

	if err := db.UpsertCommit(ctx, s.DB, db.Commit{
		ProjectID:       projectID,
		BranchName:      "main",
		CommitHash:      "override-hash",
		Subject:         "override commit",
		UserName:        "Test User",
		UserEmail:       "test@example.com",
		AuthoredAt:      time.Date(2026, time.March, 21, 8, 0, 0, 0, time.UTC).Unix(),
		LinesTotal:      10,
		LinesFromAgent:  7,
		CoverageVersion: currentCommitCoverageVersion,
		DetailFiles:     string(detailFiles),
	}); err != nil {
		t.Fatalf("UpsertCommit: %v", err)
	}
	if err := db.SetCommitOverrideAgentPercents(ctx, s.DB, projectID, "override-hash", map[string]int{"claude": 100}); err != nil {
		t.Fatalf("SetCommitOverrideAgentPercents: %v", err)
	}

	startMs := time.Date(2026, time.March, 21, 0, 0, 0, 0, time.UTC).UnixMilli()
	endMs := time.Date(2026, time.March, 22, 0, 0, 0, 0, time.UTC).UnixMilli()
	req := httptest.NewRequest(
		"GET",
		fmt.Sprintf("/api/v1/projects/%s/file-type-coverage?start=%d&end=%d", projectID, startMs, endMs),
		nil,
	)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok=false, error=%v", env.Error)
	}

	rows := env.Data.([]any)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0].(map[string]any)
	if got := row["extension"].(string); got != ".go" {
		t.Fatalf("extension = %q, want %q", got, ".go")
	}
	if got := int(row["totalLines"].(float64)); got != 10 {
		t.Fatalf("totalLines = %d, want 10", got)
	}
	if got := row["manualPercent"].(float64); got != 0 {
		t.Fatalf("manualPercent = %v, want 0", got)
	}

	segments := row["agentSegments"].([]any)
	if len(segments) != 1 {
		t.Fatalf("agentSegments len = %d, want 1", len(segments))
	}
	segment := segments[0].(map[string]any)
	if got := segment["agent"].(string); got != "claude" {
		t.Fatalf("agent = %q, want %q", got, "claude")
	}
	if got := int(segment["linesFromAgent"].(float64)); got != 10 {
		t.Fatalf("linesFromAgent = %d, want 10", got)
	}
	if got := segment["linePercent"].(float64); got != 100 {
		t.Fatalf("linePercent = %v, want 100", got)
	}
}

func TestListProjectCommitsForProject_DailyWindowDaysQuery(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	appPath := filepath.Join(repo, "app.txt")
	mustWriteFile(t, appPath, "hello\n")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "add", "app.txt")
	gitRun(t, repo, []string{"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}, "commit", "-m", "seed")

	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))
	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/commits?page=1&dailyWindowDays=365", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok=false, error=%v", env.Error)
	}

	data := env.Data.(map[string]any)
	dailySummary, ok := data["dailySummary"].([]any)
	if !ok {
		t.Fatalf("dailySummary missing or invalid type")
	}
	if got := len(dailySummary); got != 365 {
		t.Fatalf("dailySummary len = %d, want 365", got)
	}
}

func TestReadOnlyCommitEndpointsUseDatabaseWhenRepoMissing(t *testing.T) {
	s := setupTestServer(t)
	s.ReadOnly = true
	handler := s.Routes()
	ctx := context.Background()

	projectPath := filepath.Join(t.TempDir(), "missing-project")
	projectID, err := db.EnsureProject(ctx, s.DB, projectPath)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, "git-readonly"); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.UpdateProjectDefaultBranch(ctx, s.DB, projectID, "main"); err != nil {
		t.Fatalf("UpdateProjectDefaultBranch: %v", err)
	}
	if err := db.UpdateProjectLocalUser(ctx, s.DB, projectID, "Test User", "test@example.com"); err != nil {
		t.Fatalf("UpdateProjectLocalUser: %v", err)
	}

	if err := db.EnsureConversation(ctx, s.DB, "conv-readonly", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	agentTs := mustUnixMilli(t, "2026-01-01T00:59:00Z")
	agentDiff := "```diff\n" +
		"diff --git a/app.txt b/app.txt\n" +
		"--- a/app.txt\n" +
		"+++ b/app.txt\n" +
		"@@ -1 +1,2 @@\n" +
		" start\n" +
		"+hello world\n" +
		"```"
	if err := db.InsertMessages(ctx, s.DB, []db.Message{{
		Timestamp:      agentTs,
		ProjectID:      projectID,
		ConversationID: "conv-readonly",
		Role:           "agent",
		Content:        agentDiff,
		RawJSON:        agent.DerivedDiffRawJSON,
	}}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	commitHash := "readonly-hash"
	commitDiff := strings.Join([]string{
		"diff --git a/app.txt b/app.txt",
		"--- a/app.txt",
		"+++ b/app.txt",
		"@@ -1 +1,2 @@",
		" start",
		"+hello world",
		"",
	}, "\n")
	if err := db.UpsertCommit(ctx, s.DB, db.Commit{
		ProjectID:       projectID,
		BranchName:      "main",
		CommitHash:      commitHash,
		Subject:         "readonly commit",
		UserName:        "Test User",
		UserEmail:       "test@example.com",
		AuthoredAt:      mustUnixMilli(t, "2026-01-01T01:00:00Z") / 1000,
		DiffContent:     commitDiff,
		LinesTotal:      1,
		LinesFromAgent:  1,
		CoverageVersion: currentCommitCoverageVersion,
	}); err != nil {
		t.Fatalf("UpsertCommit: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/projects/commits", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("global commits status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode global commits response: %v", err)
	}
	if !env.OK {
		t.Fatalf("global commits ok=false, error=%v", env.Error)
	}
	globalData := env.Data.(map[string]any)
	globalCommits := globalData["commits"].([]any)
	if len(globalCommits) != 1 {
		t.Fatalf("global commits len = %d, want 1", len(globalCommits))
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/commits", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("project commits status = %d, want %d", rec.Code, http.StatusOK)
	}
	env = jsonEnvelope{}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode project commits response: %v", err)
	}
	if !env.OK {
		t.Fatalf("project commits ok=false, error=%v", env.Error)
	}
	pageData := env.Data.(map[string]any)
	if got := pageData["currentEmail"].(string); got != "test@example.com" {
		t.Fatalf("currentEmail = %q, want %q", got, "test@example.com")
	}
	pageCommits := pageData["commits"].([]any)
	if len(pageCommits) != 1 {
		t.Fatalf("project commits len = %d, want 1", len(pageCommits))
	}
	if got := pageCommits[0].(map[string]any)["workingCopy"].(bool); got {
		t.Fatal("workingCopy = true, want false in read-only mode")
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/commits/"+commitHash, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("commit detail status = %d, want %d", rec.Code, http.StatusOK)
	}
	env = jsonEnvelope{}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode commit detail response: %v", err)
	}
	if !env.OK {
		t.Fatalf("commit detail ok=false, error=%v", env.Error)
	}
	detail := env.Data.(map[string]any)
	if got := detail["diff"].(string); !strings.Contains(got, "hello world") {
		t.Fatalf("diff = %q, want stored diff content", got)
	}
	if got := len(detail["files"].([]any)); got == 0 {
		t.Fatal("files len = 0, want recomputed file detail from stored diff")
	}
	if got := len(detail["messages"].([]any)); got == 0 {
		t.Fatal("messages len = 0, want recomputed message attribution from database")
	}
}

func testToken(path string, sign byte, norm string, _ int) diffToken {
	styleNorm := normalizeStyleEquivalentLine(path, norm, norm)
	return diffToken{
		Path:           path,
		Sign:           sign,
		Norm:           norm,
		StyleNorm:      styleNorm,
		Key:            path + "\x1f" + string(sign) + "\x1f" + norm,
		MatchKeys:      buildDiffTokenMatchKeys(path, sign, norm),
		StyleMatchKeys: buildDiffTokenMatchKeys(path, sign, styleNorm),
		Attributable:   true,
	}
}

func normsByPathFromTokens(tokens []diffToken) map[string][]string {
	out := make(map[string][]string)
	for _, tok := range tokens {
		if tok.Path == "" || tok.Norm == "" || !tok.Attributable {
			continue
		}
		out[tok.Path] = append(out[tok.Path], tok.Norm)
	}
	return out
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustUnixMilli(t *testing.T, rfc3339 string) int64 {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		t.Fatalf("parse timestamp %q: %v", rfc3339, err)
	}
	return ts.UnixMilli()
}

func gitRun(t *testing.T, repo string, extraEnv []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return string(out)
}

func TestListProjectCommitsForProject_ByBranch(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	appPath := filepath.Join(repo, "app.txt")
	mustWriteFile(t, appPath, "start\n")
	gitRun(t, repo, nil, "add", "app.txt")
	gitRun(t, repo, nil, "commit", "-m", "initial")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	gitRun(t, repo, nil, "checkout", "-b", "feature/demo")
	mustWriteFile(t, appPath, "start\nfeature\n")
	gitRun(t, repo, nil, "add", "app.txt")
	gitRun(t, repo, nil, "commit", "-m", "feature change")

	pid, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, pid, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	if err := db.UpdateProjectDefaultBranch(ctx, s.DB, pid, "main"); err != nil {
		t.Fatalf("UpdateProjectDefaultBranch: %v", err)
	}

	// Trigger async ingestion.
	req := httptest.NewRequest("GET", "/api/v1/projects/"+pid+"/commits?page=1&branch=feature/demo", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	waitForCommitIngestion(t, s)

	// Re-query after ingestion completes.
	req = httptest.NewRequest("GET", "/api/v1/projects/"+pid+"/commits?page=1&branch=feature/demo", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status after ingestion = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok=false, error=%v", env.Error)
	}

	data := env.Data.(map[string]any)
	if got := data["branch"].(string); got != "feature/demo" {
		t.Fatalf("branch = %q, want %q", got, "feature/demo")
	}
	commits := data["commits"].([]any)
	if len(commits) == 0 {
		t.Fatalf("expected commits for feature branch")
	}
	found := false
	for _, raw := range commits {
		item := raw.(map[string]any)
		if item["subject"].(string) == "feature change" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("feature branch response missing feature commit")
	}
}

func TestSummarizeDiffFiles_SmallDiffFullMatchAttributed(t *testing.T) {
	// A 5-line diff where all 5 norms match agent output should be attributed.
	diffText := strings.Join([]string{
		"diff --git a/small.txt b/small.txt",
		"--- a/small.txt",
		"+++ b/small.txt",
		"@@ -0,0 +1,5 @@",
		"+alpha",
		"+beta",
		"+gamma",
		"+delta",
		"+epsilon",
		"",
	}, "\n")

	commitTokens := []diffToken{
		testToken("small.txt", '+', "alpha", 5),
		testToken("small.txt", '+', "beta", 4),
		testToken("small.txt", '+', "gamma", 5),
		testToken("small.txt", '+', "delta", 5),
		testToken("small.txt", '+', "epsilon", 7),
	}

	// No exact file match — fileAgent is empty.
	fileAgent := map[string]commitFileCoverage{}

	// All norms available in the pool (agent produced them).
	remainingNorms := map[string]int{
		"alpha":   1,
		"beta":    1,
		"gamma":   1,
		"delta":   1,
		"epsilon": 1,
	}

	files := summarizeDiffFiles(parseUnifiedDiffTokensWithFiles(diffText, nil).Files, fileAgent)
	files, fbLines, _ := applyFallbackFileCoverage(files, fileAgent, nil, normsByPathFromTokens(commitTokens), remainingNorms, &messageIndex{})
	if len(files) != 1 {
		t.Fatalf("files len = %d, want 1", len(files))
	}
	f := files[0]
	if !f.CopiedFromAgent {
		t.Fatalf("CopiedFromAgent = false, want true")
	}
	if f.LinesFromAgent != 5 {
		t.Fatalf("LinesFromAgent = %d, want 5", f.LinesFromAgent)
	}
	if f.LinePercent != 100 {
		t.Fatalf("LinePercent = %.1f, want 100", f.LinePercent)
	}
	if fbLines != 5 {
		t.Fatalf("fallback lines = %d, want 5", fbLines)
	}
}

func TestSummarizeDiffFiles_SingleLineDiffNotAttributed(t *testing.T) {
	// A single-line diff should NOT be attributed even if the norm matches,
	// because the minimum is 2 attributable lines.
	diffText := strings.Join([]string{
		"diff --git a/tiny.txt b/tiny.txt",
		"--- a/tiny.txt",
		"+++ b/tiny.txt",
		"@@ -0,0 +1 @@",
		"+onlyone",
		"",
	}, "\n")

	commitTokens := []diffToken{
		testToken("tiny.txt", '+', "onlyone", 7),
	}

	fileAgent := map[string]commitFileCoverage{}
	remainingNorms := map[string]int{"onlyone": 1}

	files := summarizeDiffFiles(parseUnifiedDiffTokensWithFiles(diffText, nil).Files, fileAgent)
	files, fbLines, _ := applyFallbackFileCoverage(files, fileAgent, nil, normsByPathFromTokens(commitTokens), remainingNorms, &messageIndex{})
	if len(files) != 1 {
		t.Fatalf("files len = %d, want 1", len(files))
	}
	f := files[0]
	if f.CopiedFromAgent {
		t.Fatalf("CopiedFromAgent = true, want false for single-line diff")
	}
	if f.LinesFromAgent != 0 {
		t.Fatalf("LinesFromAgent = %d, want 0", f.LinesFromAgent)
	}
	if fbLines != 0 {
		t.Fatalf("fallback lines = %d, want 0", fbLines)
	}
}

func TestSummarizeDiffFiles_FallbackTotalsReturnedForLargeDiff(t *testing.T) {
	// Verify fallback totals are returned for a standard >=10 line fallback match.
	lines := []string{
		"diff --git a/big.txt b/big.txt",
		"--- a/big.txt",
		"+++ b/big.txt",
		"@@ -0,0 +1,10 @@",
	}
	norms := []string{"a1", "b2", "c3", "d4", "e5", "f6", "g7", "h8", "i9", "j10"}
	for _, n := range norms {
		lines = append(lines, "+"+n)
	}
	lines = append(lines, "")
	diffText := strings.Join(lines, "\n")

	var commitTokens []diffToken
	remainingNorms := map[string]int{}
	for _, n := range norms {
		commitTokens = append(commitTokens, testToken("big.txt", '+', n, len(n)))
		remainingNorms[n] = 1
	}

	fileAgent := map[string]commitFileCoverage{}
	files := summarizeDiffFiles(parseUnifiedDiffTokensWithFiles(diffText, nil).Files, fileAgent)
	files, fbLines, _ := applyFallbackFileCoverage(files, fileAgent, nil, normsByPathFromTokens(commitTokens), remainingNorms, &messageIndex{})

	if len(files) != 1 {
		t.Fatalf("files len = %d, want 1", len(files))
	}
	f := files[0]
	if !f.CopiedFromAgent {
		t.Fatalf("CopiedFromAgent = false, want true")
	}
	if fbLines != 10 {
		t.Fatalf("fallback lines = %d, want 10", fbLines)
	}
}
