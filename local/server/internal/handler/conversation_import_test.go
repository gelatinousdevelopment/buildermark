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

func TestIsContentArray(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "tool_result array",
			content: `[{"type":"tool_result","tool_use_id":"abc","content":"ok"}]`,
			want:    true,
		},
		{
			name:    "text block array",
			content: `[{"type":"text","text":"hello"}]`,
			want:    true,
		},
		{
			name:    "tool_use array",
			content: `[{"type":"tool_use","name":"Bash","id":"abc"}]`,
			want:    true,
		},
		{
			name:    "plain string (real user message)",
			content: `"hello world"`,
			want:    false,
		},
		{
			name:    "empty",
			content: ``,
			want:    false,
		},
		{
			name:    "object not array",
			content: `{"type":"tool_result"}`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isContentArray(json.RawMessage(tt.content))
			if got != tt.want {
				t.Errorf("isContentArray(%s) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestClaudeCloudToEntry_ArrayContentSetsUUID(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "tool_result array",
			content: `[{"type":"tool_result","tool_use_id":"abc","content":"file written"}]`,
		},
		{
			name:    "text block array (subagent prompt)",
			content: `[{"type":"text","text":"You are a helpful assistant"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := cloudEvent{
				Type:      "user",
				CreatedAt: "2025-01-01T00:00:00Z",
				Message: &struct {
					Role       string          `json:"role"`
					Model      string          `json:"model"`
					Content    json.RawMessage `json:"content"`
					StopReason string          `json:"stop_reason"`
				}{
					Role:    "user",
					Content: json.RawMessage(tt.content),
				},
			}
			raw := json.RawMessage(`{}`)

			entry, _ := claudeCloudToEntry(ev, raw)

			if entry.SourceToolAssistantUUID != "cloud" {
				t.Errorf("expected SourceToolAssistantUUID = %q, got %q", "cloud", entry.SourceToolAssistantUUID)
			}
		})
	}
}

func TestCloudEventResultParsing(t *testing.T) {
	raw := `{"type":"result","result":"Here is the summary.","created_at":"2025-01-01T00:00:00Z","subtype":"success"}`
	var ev cloudEvent
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("failed to unmarshal result event: %v", err)
	}
	if ev.Type != "result" {
		t.Errorf("expected type %q, got %q", "result", ev.Type)
	}
	if ev.Result != "Here is the summary." {
		t.Errorf("expected result %q, got %q", "Here is the summary.", ev.Result)
	}
	if ev.Message != nil {
		t.Error("expected nil message for result event")
	}
}

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

func TestClaudeCloudToEntry_RealUserNoUUID(t *testing.T) {
	ev := cloudEvent{
		Type:      "user",
		CreatedAt: "2025-01-01T00:00:00Z",
		Message: &struct {
			Role       string          `json:"role"`
			Model      string          `json:"model"`
			Content    json.RawMessage `json:"content"`
			StopReason string          `json:"stop_reason"`
		}{
			Role:    "user",
			Content: json.RawMessage(`"Fix the bug in main.go"`),
		},
	}
	raw := json.RawMessage(`{"type":"user","message":{"role":"user","content":"Fix the bug in main.go"}}`)

	entry, _ := claudeCloudToEntry(ev, raw)

	if entry.SourceToolAssistantUUID != "" {
		t.Errorf("expected empty SourceToolAssistantUUID for real user message, got %q", entry.SourceToolAssistantUUID)
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
