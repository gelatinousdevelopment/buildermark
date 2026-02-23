package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidcann/zrate/web/server/internal/agent"
	"github.com/davidcann/zrate/web/server/internal/db"
)

type stubDiscoverer struct {
	name  string
	paths []string
}

func (s *stubDiscoverer) Name() string { return s.name }

func (s *stubDiscoverer) DiscoverProjectPathsSince(ctx context.Context, since time.Time) []string {
	_ = ctx
	_ = since
	return append([]string(nil), s.paths...)
}

func TestDiscoverImportableProjects(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	nested := filepath.Join(repo, "nested", "path")
	mustMkdirAll(t, nested)

	reg := agent.NewRegistry()
	reg.Register(&stubDiscoverer{name: "discoverer", paths: []string{nested}})
	s.Agents = reg

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/discover-importable?days=30", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Projects []struct {
				Path    string `json:"path"`
				Tracked bool   `json:"tracked"`
			} `json:"projects"`
			Since string `json:"since"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok=false error=%q", env.Error)
	}
	if len(env.Data.Projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(env.Data.Projects))
	}
	if env.Data.Projects[0].Path != repo {
		t.Fatalf("project path = %q, want %q", env.Data.Projects[0].Path, repo)
	}
	if env.Data.Projects[0].Tracked {
		t.Fatalf("tracked = true, want false")
	}
	if strings.TrimSpace(env.Data.Since) == "" {
		t.Fatal("since should not be empty")
	}
}

func TestImportProjectsImportsHistoryAndCommits(t *testing.T) {
	w := &mockWatcher{name: "claude"}
	s := setupTestServerWithWatcher(t, w)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")
	file := filepath.Join(repo, "app.txt")
	mustWriteFile(t, file, "hello\n")
	gitRun(t, repo, nil, "add", "app.txt")
	gitRun(t, repo, nil, "commit", "-m", "initial")
	headHash := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.SetProjectIgnored(ctx, s.DB, projectID, true); err != nil {
		t.Fatalf("SetProjectIgnored(true): %v", err)
	}

	body, _ := json.Marshal(importProjectsRequest{
		Paths:       []string{repo},
		HistoryDays: "90",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Import now returns 202 Accepted and runs asynchronously.
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Started bool `json:"started"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok=false error=%q", env.Error)
	}
	if !env.Data.Started {
		t.Fatal("started=false, want true")
	}

	// Wait for the background import goroutine to finish.
	// The import holds s.importMu; acquiring it confirms completion.
	s.importMu.Lock()
	s.importMu.Unlock()

	project, err := getProjectByID(ctx, s.DB, projectID)
	if err != nil || project == nil {
		t.Fatalf("getProjectByID err=%v project=%v", err, project)
	}
	if project.Ignored {
		t.Fatal("project should be tracked after import")
	}

	commit, err := db.GetCommitByHash(ctx, s.DB, projectID, "main", headHash)
	if err != nil {
		t.Fatalf("GetCommitByHash: %v", err)
	}
	if commit == nil {
		t.Fatalf("expected commit %s to be ingested", headHash)
	}

	_, scanPathsCount, _, lastPaths := w.snapshot()
	if scanPathsCount != 1 {
		t.Fatalf("scanPathsCount = %d, want 1", scanPathsCount)
	}
	if len(lastPaths) != 1 || lastPaths[0] != repo {
		t.Fatalf("lastPaths = %#v, want [%q]", lastPaths, repo)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
}
