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

	"github.com/davidcann/zrate/web/server/internal/db"
)

func TestStripBinaryDiffsRemovesBinaryBlocks(t *testing.T) {
	raw := strings.Join([]string{
		"diff --git a/app.txt b/app.txt",
		"index 1111111..2222222 100644",
		"--- a/app.txt",
		"+++ b/app.txt",
		"@@ -1 +1,2 @@",
		" hello",
		"+world",
		"diff --git a/image.png b/image.png",
		"new file mode 100644",
		"index 0000000..abc1234",
		"Binary files /dev/null and b/image.png differ",
		"diff --git a/readme.md b/readme.md",
		"index 3333333..4444444 100644",
		"--- a/readme.md",
		"+++ b/readme.md",
		"@@ -1 +1,2 @@",
		" title",
		"+more",
		"",
	}, "\n")

	clean := stripBinaryDiffs(raw)

	if strings.Contains(clean, "Binary files /dev/null and b/image.png differ") {
		t.Fatalf("binary marker should be removed, got: %q", clean)
	}
	if strings.Contains(clean, "diff --git a/image.png b/image.png") {
		t.Fatalf("binary diff block should be removed, got: %q", clean)
	}
	if !strings.Contains(clean, "diff --git a/app.txt b/app.txt") {
		t.Fatalf("expected text diff for app.txt to remain, got: %q", clean)
	}
	if !strings.Contains(clean, "diff --git a/readme.md b/readme.md") {
		t.Fatalf("expected text diff for readme.md to remain, got: %q", clean)
	}
}

func TestIngestEndpointsAndStatus(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	repo := t.TempDir()
	gitRun(t, repo, nil, "init", "-b", "main")
	gitRun(t, repo, nil, "config", "user.name", "Test User")
	gitRun(t, repo, nil, "config", "user.email", "test@example.com")

	appPath := filepath.Join(repo, "app.txt")
	mustWriteFile(t, appPath, "hello\n")
	gitRun(t, repo, nil, "add", "app.txt")
	gitRun(t, repo, nil, "commit", "-m", "text commit")

	root := strings.TrimSpace(gitRun(t, repo, nil, "rev-list", "--max-parents=0", "HEAD"))

	binPath := filepath.Join(repo, "blob.bin")
	if err := os.WriteFile(binPath, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatalf("write binary file: %v", err)
	}
	gitRun(t, repo, nil, "add", "blob.bin")
	gitRun(t, repo, nil, "commit", "-m", "binary commit")
	binaryHash := strings.TrimSpace(gitRun(t, repo, nil, "rev-parse", "HEAD"))

	projectID, err := db.EnsureProject(ctx, s.DB, repo)
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.UpdateProjectGitID(ctx, s.DB, projectID, root); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}

	postJSON := func(path string, body any) *httptest.ResponseRecorder {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	parseData := func(rec *httptest.ResponseRecorder) map[string]any {
		var env jsonEnvelope
		if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		if !env.OK {
			t.Fatalf("expected ok=true, got error: %s", env.Error)
		}
		data, ok := env.Data.(map[string]any)
		if !ok {
			t.Fatalf("response data type = %T, want object", env.Data)
		}
		return data
	}

	rec := postJSON("/api/v1/projects/"+projectID+"/ingest-commits", map[string]any{"count": 1})
	if rec.Code != http.StatusOK {
		t.Fatalf("ingest status = %d, want %d", rec.Code, http.StatusOK)
	}
	ingest1 := parseData(rec)
	if got := int(ingest1["ingested"].(float64)); got != 1 {
		t.Fatalf("ingested = %d, want 1", got)
	}
	if got := ingest1["reachedRoot"].(bool); got {
		t.Fatalf("reachedRoot = %v, want false", got)
	}

	c, err := db.GetCommitByHash(ctx, s.DB, projectID, "main", binaryHash)
	if err != nil {
		t.Fatalf("GetCommitByHash: %v", err)
	}
	if c == nil {
		t.Fatalf("expected ingested commit %s to exist", binaryHash)
	}
	if strings.Contains(c.DiffContent, "Binary files ") || strings.Contains(c.DiffContent, "GIT binary patch") {
		t.Fatalf("binary diff markers should be stripped, got diff: %q", c.DiffContent)
	}

	rec = get("/api/v1/projects/" + projectID + "/commit-ingestion-status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status endpoint code = %d, want %d", rec.Code, http.StatusOK)
	}
	status1 := parseData(rec)
	if got := int(status1["ingestedCount"].(float64)); got != 1 {
		t.Fatalf("ingestedCount = %d, want 1", got)
	}
	if got := int(status1["totalGitCommits"].(float64)); got != 2 {
		t.Fatalf("totalGitCommits = %d, want 2", got)
	}
	if got := status1["reachedRoot"].(bool); got {
		t.Fatalf("status reachedRoot = %v, want false", got)
	}

	rec = postJSON("/api/v1/projects/"+projectID+"/ingest-commits", map[string]any{"count": 10})
	if rec.Code != http.StatusOK {
		t.Fatalf("second ingest status = %d, want %d", rec.Code, http.StatusOK)
	}
	ingest2 := parseData(rec)
	if got := int(ingest2["ingested"].(float64)); got != 1 {
		t.Fatalf("second ingested = %d, want 1", got)
	}
	if got := ingest2["reachedRoot"].(bool); !got {
		t.Fatalf("second reachedRoot = %v, want true", got)
	}

	rec = get("/api/v1/projects/" + projectID + "/commit-ingestion-status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status endpoint code after second ingest = %d, want %d", rec.Code, http.StatusOK)
	}
	status2 := parseData(rec)
	if got := int(status2["ingestedCount"].(float64)); got != 2 {
		t.Fatalf("ingestedCount after second ingest = %d, want 2", got)
	}
	if got := status2["reachedRoot"].(bool); !got {
		t.Fatalf("status reachedRoot after second ingest = %v, want true", got)
	}
}
