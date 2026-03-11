package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFindProjectByCwd_OldPathBasename(t *testing.T) {
	s := setupTestServer(t)
	ctx := context.Background()

	// Insert a project with old_paths containing a renamed path.
	_, err := s.DB.ExecContext(ctx,
		"INSERT INTO projects (id, path, label, old_paths) VALUES (?, ?, ?, ?)",
		"proj-bm", "/Users/user/github/buildermark", "buildermark", "/Users/user/github/zrate",
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{
			name: "exact path match",
			cwd:  "/Users/user/github/buildermark",
			want: "proj-bm",
		},
		{
			name: "old path exact match",
			cwd:  "/Users/user/github/zrate",
			want: "proj-bm",
		},
		{
			name: "old path subdirectory match",
			cwd:  "/Users/user/github/zrate/subdir",
			want: "proj-bm",
		},
		{
			name: "sandbox cwd with old path basename",
			cwd:  "/home/user/zrate",
			want: "proj-bm",
		},
		{
			name: "no match",
			cwd:  "/home/user/unrelated",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findProjectByCwd(ctx, s.DB, tt.cwd)
			if err != nil {
				t.Fatalf("findProjectByCwd(%q): %v", tt.cwd, err)
			}
			if got != tt.want {
				t.Errorf("findProjectByCwd(%q) = %q, want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

func TestImportWebConversationLogsRawRequest(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	body := `{"url":"https://example.com/conversations/1","agent":"codex","title":"Test import","messages":[{"role":"user","content":"hello","timestamp":1},{"role":"agent","content":"world","timestamp":2}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/import-web", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	logRow := fetchLatestImportLog(t, s.DB)
	if !logRow.logType.Valid || logRow.logType.String != "web" {
		t.Fatalf("type = %#v, want web", logRow.logType)
	}
	if logRow.source.Valid {
		t.Fatalf("source = %#v, want NULL", logRow.source)
	}
	if logRow.sourceID != "https://example.com/conversations/1" {
		t.Fatalf("source_id = %q, want %q", logRow.sourceID, "https://example.com/conversations/1")
	}
	if logRow.timestamp <= 0 {
		t.Fatalf("timestamp = %d, want > 0", logRow.timestamp)
	}
	if logRow.content != body {
		t.Fatalf("content = %q, want %q", logRow.content, body)
	}
}

func TestImportWebConversationLogsCloudSource(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	body := `{"url":"https://example.com/conversations/cloud-1","agent":"claude_cloud","events":[{"type":"result","result":"Final answer","created_at":"2025-01-01T00:00:00Z"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/import-web", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	logRow := fetchLatestImportLog(t, s.DB)
	if !logRow.source.Valid || logRow.source.String != "claude_cloud" {
		t.Fatalf("source = %#v, want claude_cloud", logRow.source)
	}
	if logRow.content != body {
		t.Fatalf("content = %q, want %q", logRow.content, body)
	}
}

func TestImportWebConversationDuplicateStillLogs(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	body := `{"url":"https://example.com/conversations/dup-1","agent":"codex","messages":[{"role":"user","content":"hello","timestamp":1}]}`

	for i, wantStatus := range []int{http.StatusCreated, http.StatusOK} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/import-web", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != wantStatus {
			t.Fatalf("request %d status = %d, want %d", i+1, rec.Code, wantStatus)
		}
	}

	if got := countImportLogs(t, s.DB); got != 2 {
		t.Fatalf("import log count = %d, want 2", got)
	}
}

func TestImportWebConversationInvalidRequestsDoNotLog(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "malformed json",
			body: `{"url":`,
		},
		{
			name: "missing url",
			body: `{"agent":"codex","messages":[{"role":"user","content":"hello"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := setupTestServer(t)
			handler := s.Routes()

			req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/import-web", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if got := countImportLogs(t, s.DB); got != 0 {
				t.Fatalf("import log count = %d, want 0", got)
			}
		})
	}
}

func TestNormalizeRemoteToRepoKey_GitProtocol(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "git://github.com/owner/repo.git",
			want:  "github.com/owner/repo",
		},
		{
			input: "git://github.com/owner/repo",
			want:  "github.com/owner/repo",
		},
		{
			input: "https://github.com/owner/repo.git",
			want:  "github.com/owner/repo",
		},
		{
			input: "git@github.com:owner/repo.git",
			want:  "github.com/owner/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeRemoteToRepoKey(tt.input)
			if got != tt.want {
				t.Errorf("normalizeRemoteToRepoKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestImportCodexCloudTask_EndToEnd(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	cloudData := `{
		"current_turn_id": "task_e_abc~assttrn_e_def",
		"turn_mapping": {
			"task_e_abc~assttrn_e_def": {
				"id": "task_e_abc~assttrn_e_def",
				"parent": "task_e_abc~usertrn_e_ghi",
				"turn": {
					"created_at": 1700000001.0,
					"role": "assistant",
					"type": "assistant",
					"model_version": "gpt-5",
					"output_items": [
						{
							"content": [{"content_type": "text", "text": "Done!"}],
							"role": "assistant",
							"type": "message"
						},
						{
							"type": "pr",
							"output_diff": {
								"type": "output_diff",
								"diff": "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1,2 @@\n old\n+new"
							}
						}
					]
				}
			},
			"task_e_abc~usertrn_e_ghi": {
				"id": "task_e_abc~usertrn_e_ghi",
				"parent": null,
				"turn": {
					"created_at": 1700000000.0,
					"role": "user",
					"type": "user",
					"input_items": [
						{
							"content": [{"content_type": "text", "text": "Fix the bug"}],
							"role": "user",
							"type": "message"
						}
					]
				}
			}
		}
	}`

	body, _ := json.Marshal(map[string]any{
		"url":       "https://chatgpt.com/codex/s/task_e_abc",
		"agent":     "codex_cloud",
		"cloudData": json.RawMessage(cloudData),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/import-web", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Imported       bool   `json:"imported"`
			ConversationID string `json:"conversationId"`
			MessageCount   int    `json:"messageCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.Ok || !resp.Data.Imported {
		t.Fatalf("expected ok=true imported=true, got ok=%v imported=%v", resp.Ok, resp.Data.Imported)
	}
	if resp.Data.MessageCount != 3 {
		t.Errorf("messageCount = %d, want 3 (user + assistant + diff)", resp.Data.MessageCount)
	}
}

func TestImportCodexCloudTask_ProjectMatching(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	// Insert a project with a matching remote.
	_, err := s.DB.ExecContext(ctx,
		"INSERT INTO projects (id, path, label, remote) VALUES (?, ?, ?, ?)",
		"proj-codex", "/tmp/myrepo", "myrepo", "https://github.com/owner/myrepo.git",
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	cloudData := `{
		"current_turn_id": "t~a1",
		"turn_mapping": {
			"t~a1": {
				"id": "t~a1",
				"parent": "t~u1",
				"turn": {
					"created_at": 1700000001.0,
					"role": "assistant",
					"type": "assistant",
					"output_items": [
						{
							"content": [{"content_type": "text", "text": "Done"}],
							"role": "assistant",
							"type": "message"
						}
					],
					"environment": {
						"repo_map": {
							"repo-1": {
								"git_url": "git://github.com/owner/myrepo.git",
								"name": "myrepo"
							}
						}
					}
				}
			},
			"t~u1": {
				"id": "t~u1",
				"parent": null,
				"turn": {
					"created_at": 1700000000.0,
					"role": "user",
					"type": "user",
					"input_items": [
						{
							"content": [{"content_type": "text", "text": "Hello"}],
							"role": "user",
							"type": "message"
						}
					]
				}
			}
		}
	}`

	body, _ := json.Marshal(map[string]any{
		"url":       "https://chatgpt.com/codex/s/task_e_proj",
		"agent":     "codex_cloud",
		"cloudData": json.RawMessage(cloudData),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/import-web", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	// Verify conversation was placed in the correct project.
	var resp struct {
		Data struct {
			ConversationID string `json:"conversationId"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	var projectID string
	s.DB.QueryRowContext(ctx, "SELECT project_id FROM conversations WHERE id = ?", resp.Data.ConversationID).Scan(&projectID)
	if projectID != "proj-codex" {
		t.Errorf("project_id = %q, want proj-codex", projectID)
	}
}

func TestImportCodexCloudTask_Deduplication(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	cloudData := `{
		"current_turn_id": "t~u1",
		"turn_mapping": {
			"t~u1": {
				"id": "t~u1",
				"parent": null,
				"turn": {
					"created_at": 1700000000.0,
					"role": "user",
					"type": "user",
					"input_items": [
						{
							"content": [{"content_type": "text", "text": "Hello"}],
							"role": "user",
							"type": "message"
						}
					]
				}
			}
		}
	}`

	body, _ := json.Marshal(map[string]any{
		"url":       "https://chatgpt.com/codex/s/task_e_dup",
		"agent":     "codex_cloud",
		"cloudData": json.RawMessage(cloudData),
	})

	// First request.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/import-web", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first request status = %d, want %d", rec.Code, http.StatusCreated)
	}

	// Second request — same URL.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/conversations/import-web", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second request status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Data struct {
			AlreadyExisted bool `json:"alreadyExisted"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.Data.AlreadyExisted {
		t.Error("expected alreadyExisted=true on second import")
	}
}

func TestImportCodexCloudTask_LogsSource(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	cloudData := `{
		"current_turn_id": "t~u1",
		"turn_mapping": {
			"t~u1": {
				"id": "t~u1",
				"parent": null,
				"turn": {
					"created_at": 1700000000.0,
					"role": "user",
					"type": "user",
					"input_items": [
						{
							"content": [{"content_type": "text", "text": "Hello"}],
							"role": "user",
							"type": "message"
						}
					]
				}
			}
		}
	}`

	body, _ := json.Marshal(map[string]any{
		"url":       "https://chatgpt.com/codex/s/task_e_log",
		"agent":     "codex_cloud",
		"cloudData": json.RawMessage(cloudData),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/import-web", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	logRow := fetchLatestImportLog(t, s.DB)
	if !logRow.source.Valid || logRow.source.String != "codex_cloud" {
		t.Fatalf("source = %#v, want codex_cloud", logRow.source)
	}
}

func TestImportClaudeCloudViaCloudData(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	// cloudData is a {data: [...]} envelope with one result event.
	cloudData := `{"data":[{"type":"result","result":"Final answer from cloud.","created_at":"2025-01-01T00:00:00Z"}]}`

	body, _ := json.Marshal(map[string]any{
		"url":       "https://claude.ai/code/session_abc123",
		"agent":     "claude_cloud",
		"cloudData": json.RawMessage(cloudData),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/import-web", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Imported     bool `json:"imported"`
			MessageCount int  `json:"messageCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Ok || !resp.Data.Imported {
		t.Fatalf("expected ok and imported")
	}
	if resp.Data.MessageCount < 1 {
		t.Errorf("messageCount = %d, want >= 1", resp.Data.MessageCount)
	}
}

type importLogRow struct {
	logType   sql.NullString
	source    sql.NullString
	sourceID  string
	timestamp int64
	content   string
}

func fetchLatestImportLog(t *testing.T, database *sql.DB) importLogRow {
	t.Helper()

	var row importLogRow
	err := database.QueryRow(`
		SELECT type, source, source_id, timestamp, content
		FROM import_logs
		ORDER BY timestamp DESC, id DESC
		LIMIT 1
	`).Scan(&row.logType, &row.source, &row.sourceID, &row.timestamp, &row.content)
	if err != nil {
		t.Fatalf("query import log: %v", err)
	}

	return row
}

func countImportLogs(t *testing.T, database *sql.DB) int {
	t.Helper()

	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM import_logs").Scan(&count); err != nil {
		t.Fatalf("count import logs: %v", err)
	}
	return count
}
