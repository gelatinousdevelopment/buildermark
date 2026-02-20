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

	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/commits?page=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first call status = %d, want %d", rec.Code, http.StatusOK)
	}

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

	req = httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/commits?page=1", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second call status = %d, want %d", rec.Code, http.StatusOK)
	}

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

	files, _, _ := summarizeDiffFiles(diffText, nil, commitTokens, fileAgent, remainingNorms)
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

	_, _, _, fileAgent, normCounts := attributeCommitToMessages(commitTokens, messages, 0, 2000)
	files, _, _ := summarizeDiffFiles(diffText, nil, commitTokens, fileAgent, normCounts)

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

	contrib, lines, chars, fileAgent, _ := attributeCommitToMessages(commitTokens, messages, 0, 2000)
	if lines != 3 {
		t.Fatalf("matched lines = %d, want 3", lines)
	}
	if chars != 48 {
		t.Fatalf("matched chars = %d, want 48", chars)
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

	_, _, _, fileAgent, remainingNorms := attributeCommitToMessages(commitTokens, messages, 0, 2000)
	files, _, _ := summarizeDiffFiles(diffText, nil, commitTokens, fileAgent, remainingNorms)
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

	contrib, lines, _, _, _ := attributeCommitToMessages(commitTokens, messages, 0, 2000)
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

	contrib, lines, _, _, _ := attributeCommitToMessages(commitTokens, messages, 0, 10000)
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

	contrib, lines, _, _, _ := attributeCommitToMessages(commitTokens, messages, 0, 10000)
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

func testToken(path string, sign byte, norm string, chars int) diffToken {
	return diffToken{
		Path:         path,
		Sign:         sign,
		Norm:         norm,
		Key:          path + "\x1f" + string(sign) + "\x1f" + norm,
		Chars:        chars,
		Attributable: true,
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

	req := httptest.NewRequest("GET", "/api/v1/projects/"+pid+"/commits?page=1&branch=feature/demo", nil)
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

	files, fbLines, fbChars := summarizeDiffFiles(diffText, nil, commitTokens, fileAgent, remainingNorms)
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
	if fbChars != 26 {
		t.Fatalf("fallback chars = %d, want 26", fbChars)
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

	files, fbLines, _ := summarizeDiffFiles(diffText, nil, commitTokens, fileAgent, remainingNorms)
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
	files, fbLines, fbChars := summarizeDiffFiles(diffText, nil, commitTokens, fileAgent, remainingNorms)

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
	expectedChars := 0
	for _, n := range norms {
		expectedChars += len(n)
	}
	if fbChars != expectedChars {
		t.Fatalf("fallback chars = %d, want %d", fbChars, expectedChars)
	}
}
