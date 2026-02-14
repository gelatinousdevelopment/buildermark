package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidcann/zrate/web/server/internal/db"
)

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
		RawJSON:        `{"source":"derived_diff"}`,
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
	if got := int(summary["commitCount"].(float64)); got != 2 {
		t.Fatalf("summary.commitCount = %d, want 2", got)
	}
	if got := int(summary["linesTotal"].(float64)); got != 2 {
		t.Fatalf("summary.linesTotal = %d, want 2", got)
	}
	if got := int(summary["linesFromAgent"].(float64)); got != 1 {
		t.Fatalf("summary.linesFromAgent = %d, want 1", got)
	}

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
	if got := int(agentCommit["linesTotal"].(float64)); got != 1 {
		t.Fatalf("agent change linesTotal = %d, want 1", got)
	}
	if got := int(agentCommit["linesFromAgent"].(float64)); got != 1 {
		t.Fatalf("agent change linesFromAgent = %d, want 1", got)
	}
	if got := int(agentCommit["charsFromAgent"].(float64)); got != 10 {
		t.Fatalf("agent change charsFromAgent = %d, want 10", got)
	}

	manualCommit, ok := bySubject["manual change"]
	if !ok {
		t.Fatalf("missing commit subject %q", "manual change")
	}
	if got := int(manualCommit["linesFromAgent"].(float64)); got != 0 {
		t.Fatalf("manual change linesFromAgent = %d, want 0", got)
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
		RawJSON:        `{"source":"derived_diff"}`,
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
	if got := int(pagination["total"].(float64)); got != 2 {
		t.Fatalf("pagination.total = %d, want 2", got)
	}
	commitRows := projectCommits["commits"].([]any)
	if len(commitRows) != 3 {
		t.Fatalf("project commits len = %d, want 3 (working copy + 2 commits)", len(commitRows))
	}
	workingCopyRow := commitRows[0].(map[string]any)
	if got := workingCopyRow["subject"].(string); got != "Working Copy" {
		t.Fatalf("first row subject = %q, want Working Copy", got)
	}
	if got := workingCopyRow["workingCopy"].(bool); !got {
		t.Fatalf("first row workingCopy = %v, want true", got)
	}
	if got := int(workingCopyRow["linesTotal"].(float64)); got < 1 {
		t.Fatalf("working copy linesTotal = %d, want >= 1", got)
	}
	if got := workingCopyRow["commitHash"].(string); got != "working-copy" {
		t.Fatalf("working copy commitHash = %q, want %q", got, "working-copy")
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
		RawJSON:        `{"source":"derived_diff"}`,
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
	if len(commits) != 1 {
		t.Fatalf("commits = %d, want 1", len(commits))
	}
	commit := commits[0].(map[string]any)
	if got := int(commit["linesTotal"].(float64)); got != 1 {
		t.Fatalf("linesTotal = %d, want 1 after ignoring AGENTS.md", got)
	}
	if got := int(commit["linesFromAgent"].(float64)); got != 1 {
		t.Fatalf("linesFromAgent = %d, want 1", got)
	}
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
