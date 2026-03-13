package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func TestResolveCommitUserFiltersExpandsLocalUserSentinel(t *testing.T) {
	got := resolveCommitUserFilters(
		[]string{localUserFilterSentinel, "other@example.com", "ALIAS@example.com"},
		gitIdentity{Email: "Me@Example.com"},
		[]string{"alias@example.com", "noreply@anthropic.com"},
	)

	want := []string{"Me@Example.com", "alias@example.com", "noreply@anthropic.com", "other@example.com"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("resolved filters = %q, want %q", strings.Join(got, ","), strings.Join(want, ","))
	}
}

func TestLocalUserAuthorFilterMatchesCaseInsensitively(t *testing.T) {
	filter := newLocalUserAuthorFilter(
		gitIdentity{Email: "me@example.com"},
		[]string{"Alias@example.com"},
	)

	commits := []db.Commit{
		{CommitHash: "a", UserEmail: "ME@example.com"},
		{CommitHash: "b", UserEmail: "alias@example.com"},
		{CommitHash: "c", UserEmail: "other@example.com"},
	}

	filtered := filter.FilterCommits(commits)
	if got := len(filtered); got != 2 {
		t.Fatalf("filtered commit count = %d, want 2", got)
	}
	if filtered[0].CommitHash != "a" || filtered[1].CommitHash != "b" {
		t.Fatalf("filtered commits = %#v, want hashes [a b]", filtered)
	}
}

func TestNotifyIngestedCommitsSkipsNonLocalCommits(t *testing.T) {
	s := setupTestServer(t)
	s.Routes()

	client := &wsClient{send: make(chan []byte, 1)}
	s.notifyWS.register(client)
	t.Cleanup(func() {
		s.notifyWS.unregister(client)
		close(client.send)
	})

	filtered := newLocalUserAuthorFilter(
		gitIdentity{Email: "me@example.com"},
		[]string{"alias@example.com"},
	).FilterCommits([]db.Commit{
		{CommitHash: "c", UserEmail: "other@example.com"},
	})

	s.notifyIngestedCommits(filtered, "repo")

	select {
	case msg := <-client.send:
		t.Fatalf("unexpected notification message: %s", string(msg))
	default:
	}
}

func TestNotifyIngestedCommitsUsesOnlyLocalCommitSubset(t *testing.T) {
	s := setupTestServer(t)
	s.Routes()

	client := &wsClient{send: make(chan []byte, 1)}
	s.notifyWS.register(client)
	t.Cleanup(func() {
		s.notifyWS.unregister(client)
		close(client.send)
	})

	filtered := newLocalUserAuthorFilter(
		gitIdentity{Email: "me@example.com"},
		[]string{"alias@example.com"},
	).FilterCommits([]db.Commit{
		{
			ProjectID:      "proj-1",
			BranchName:     "main",
			CommitHash:     "abc123",
			Subject:        "local commit",
			UserEmail:      "ME@example.com",
			LinesTotal:     10,
			LinesFromAgent: 4,
			AuthoredAt:     1,
		},
		{
			ProjectID:      "proj-1",
			BranchName:     "main",
			CommitHash:     "def456",
			Subject:        "extra local commit",
			UserEmail:      "alias@example.com",
			LinesTotal:     30,
			LinesFromAgent: 6,
			AuthoredAt:     2,
		},
		{
			ProjectID:      "proj-1",
			BranchName:     "main",
			CommitHash:     "zzz999",
			Subject:        "other commit",
			UserEmail:      "other@example.com",
			LinesTotal:     100,
			LinesFromAgent: 100,
			AuthoredAt:     3,
		},
	})

	s.notifyIngestedCommits(filtered, "repo")

	select {
	case msg := <-client.send:
		var envelope wsMessage
		if err := json.Unmarshal(msg, &envelope); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if envelope.Type != "notification" {
			t.Fatalf("message type = %q, want %q", envelope.Type, "notification")
		}

		var event notificationEvent
		if err := json.Unmarshal(envelope.Data, &event); err != nil {
			t.Fatalf("unmarshal notification event: %v", err)
		}
		if event.Kind != "commit_ingested" {
			t.Fatalf("event kind = %q, want %q", event.Kind, "commit_ingested")
		}
		if event.Title != "2 commits 25% by agents" {
			t.Fatalf("event title = %q, want %q", event.Title, "2 commits 25% by agents")
		}
		if event.Body != "repo" {
			t.Fatalf("event body = %q, want %q", event.Body, "repo")
		}
		if event.URL != "/projects/proj-1" {
			t.Fatalf("event url = %q, want %q", event.URL, "/projects/proj-1")
		}
	default:
		t.Fatal("expected notification message")
	}
}

func TestListProjectCommitsForProjectResolvesLocalUserSentinel(t *testing.T) {
	s := setupTestServer(t)
	configDir := t.TempDir()
	s.ConfigDir = configDir
	if err := saveLocalConfigFile(configDir, localConfigFile{
		ExtraLocalUserEmails: []string{"alias@example.com"},
	}); err != nil {
		t.Fatalf("saveLocalConfigFile: %v", err)
	}
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	appPath := filepath.Join(repo, "app.txt")
	mustWriteFile(t, appPath, "one\n")
	gitRun(t, repo, nil, "add", "app.txt")
	gitRun(t, repo, []string{
		"GIT_AUTHOR_NAME=Other User",
		"GIT_AUTHOR_EMAIL=other@example.com",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	}, "commit", "-m", "other commit")
	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	mustWriteFile(t, appPath, "one\ntwo\n")
	gitRun(t, repo, nil, "add", "app.txt")
	gitRun(t, repo, []string{
		"GIT_AUTHOR_NAME=Alias User",
		"GIT_AUTHOR_EMAIL=alias@example.com",
		"GIT_AUTHOR_DATE=2026-01-01T01:00:00Z",
		"GIT_COMMITTER_DATE=2026-01-01T01:00:00Z",
	}, "commit", "-m", "alias commit")

	mustWriteFile(t, appPath, "one\ntwo\nthree\n")
	gitRun(t, repo, nil, "add", "app.txt")
	gitRun(t, repo, []string{
		"GIT_AUTHOR_DATE=2026-01-01T02:00:00Z",
		"GIT_COMMITTER_DATE=2026-01-01T02:00:00Z",
	}, "commit", "-m", "local commit")

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}
	groups, err := listAllProjectGroups(ctx, s.DB)
	if err != nil {
		t.Fatalf("listAllProjectGroups: %v", err)
	}
	group, ok := findProjectGroupByProjectID(groups, projectID)
	if !ok {
		t.Fatalf("project group not found for %s", projectID)
	}
	repoProject, err := resolveRepoProject(ctx, group)
	if err != nil {
		t.Fatalf("resolveRepoProject: %v", err)
	}
	identity, err := resolveGitIdentity(ctx, repoProject.Path)
	if err != nil {
		t.Fatalf("resolveGitIdentity: %v", err)
	}
	if err := IngestDefaultCommits(ctx, s.DB, repoProject, group, identity, s.loadExtraLocalUserEmails(), "main", nil); err != nil {
		t.Fatalf("IngestDefaultCommits: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/commits?branch=main&user="+url.QueryEscape(localUserFilterSentinel), nil)
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
	if got := data["userFilter"].(string); got != "test@example.com,alias@example.com" {
		t.Fatalf("userFilter = %q, want %q", got, "test@example.com,alias@example.com")
	}

	commits := data["commits"].([]any)
	if got := len(commits); got != 2 {
		t.Fatalf("commit count = %d, want 2", got)
	}

	subjects := make([]string, 0, len(commits))
	for _, raw := range commits {
		subjects = append(subjects, raw.(map[string]any)["subject"].(string))
	}
	if strings.Join(subjects, ",") != "local commit,alias commit" {
		t.Fatalf("subjects = %q, want %q", strings.Join(subjects, ","), "local commit,alias commit")
	}
}
