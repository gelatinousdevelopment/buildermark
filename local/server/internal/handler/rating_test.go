package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

type stubResolverAgent struct {
	name   string
	result *agent.SessionResult
}

func (s *stubResolverAgent) Name() string { return s.name }

func (s *stubResolverAgent) ResolveSession(rating int, note string, fallbackID string) *agent.SessionResult {
	return s.result
}

// setupTestServer creates a Server backed by a temporary SQLite database.
func setupTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	database, err := db.InitDB(dir + "/test.db")
	if err != nil {
		t.Fatalf("init test db: %v", err)
	}
	srv := &Server{
		DB:                database,
		refreshJobs:       newJobTracker(),
		coverageJobs:      newJobTracker(),
		visibilityJobs:    newJobTracker(),
		commitIngestJobs:  newJobTracker(),
		commitDetailCache: newCommitDetailCacheStore(),
		branchCache:       newBranchCacheStore(),
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if srv.refreshJobs.isIdle() && srv.coverageJobs.isIdle() && srv.visibilityJobs.isIdle() && srv.commitIngestJobs.isIdle() {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if srv.RepoMonitor != nil {
			srv.RepoMonitor.Close()
		}
		database.Close()
	})
	return srv
}

func TestCreateRating(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        any
		wantStatus  int
		wantOK      bool
	}{
		{
			name:        "valid request",
			contentType: "application/json",
			body:        map[string]any{"conversationId": "conv-1", "rating": 4, "note": "great"},
			wantStatus:  http.StatusCreated,
			wantOK:      true,
		},
		{
			name:        "valid request with agent",
			contentType: "application/json",
			body:        map[string]any{"conversationId": "conv-1", "rating": 4, "note": "great", "agent": "claude"},
			wantStatus:  http.StatusCreated,
			wantOK:      true,
		},
		{
			name:        "valid request with unknown agent",
			contentType: "application/json",
			body:        map[string]any{"conversationId": "conv-1", "rating": 3, "agent": "codex"},
			wantStatus:  http.StatusCreated,
			wantOK:      true,
		},
		{
			name:        "valid request with temp conversation id only",
			contentType: "application/json",
			body:        map[string]any{"tempConversationId": "temp-1", "rating": 4},
			wantStatus:  http.StatusCreated,
			wantOK:      true,
		},
		{
			name:        "missing conversationId",
			contentType: "application/json",
			body:        map[string]any{"rating": 3},
			wantStatus:  http.StatusBadRequest,
			wantOK:      false,
		},
		{
			name:        "invalid rating",
			contentType: "application/json",
			body:        map[string]any{"conversationId": "conv-1", "rating": 10},
			wantStatus:  http.StatusBadRequest,
			wantOK:      false,
		},
		{
			name:        "wrong content-type",
			contentType: "text/plain",
			body:        map[string]any{"conversationId": "conv-1", "rating": 3},
			wantStatus:  http.StatusUnsupportedMediaType,
			wantOK:      false,
		},
		{
			name:        "invalid JSON",
			contentType: "application/json",
			body:        "not json{{{",
			wantStatus:  http.StatusBadRequest,
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := setupTestServer(t)
			handler := s.Routes()

			var body []byte
			switch v := tt.body.(type) {
			case string:
				body = []byte(v)
			default:
				body, _ = json.Marshal(v)
			}

			req := httptest.NewRequest("POST", "/api/v1/rating", bytes.NewReader(body))
			req.Header.Set("Content-Type", tt.contentType)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			var env jsonEnvelope
			if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if env.OK != tt.wantOK {
				t.Errorf("ok = %v, want %v", env.OK, tt.wantOK)
			}
		})
	}
}

func TestListRatings(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		seedCount  int
		wantStatus int
		wantCount  int
	}{
		{
			name:       "default limit",
			query:      "",
			seedCount:  3,
			wantStatus: http.StatusOK,
			wantCount:  3,
		},
		{
			name:       "custom limit",
			query:      "?limit=2",
			seedCount:  3,
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "empty database",
			query:      "",
			seedCount:  0,
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := setupTestServer(t)
			handler := s.Routes()

			// Seed data.
			for i := 0; i < tt.seedCount; i++ {
				body, _ := json.Marshal(map[string]any{
					"conversationId": "conv",
					"rating":         3,
				})
				req := httptest.NewRequest("POST", "/api/v1/rating", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
			}

			req := httptest.NewRequest("GET", "/api/v1/ratings"+tt.query, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			var env jsonEnvelope
			if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			data, ok := env.Data.([]any)
			if !ok {
				t.Fatalf("data is not an array: %T", env.Data)
			}
			if len(data) != tt.wantCount {
				t.Errorf("got %d ratings, want %d", len(data), tt.wantCount)
			}
		})
	}
}

func TestCreateRatingResolvedSessionDoesNotChangeConversationBounds(t *testing.T) {
	s := setupTestServer(t)
	registry := agent.NewRegistry()
	registry.Register(&stubResolverAgent{
		name: "codex",
		result: &agent.SessionResult{
			SessionID: "conv-1",
			Project:   "/test/project",
			Entries: []agent.Entry{
				{Timestamp: 5000, SessionID: "conv-1", Project: "/test/project", Role: "user", Display: "$rate-buildermark 5 still great", RawJSON: `{}`},
				{Timestamp: 6000, SessionID: "conv-1", Project: "/test/project", Role: "agent", Display: "Base directory for this skill: /tmp/skills/rate-buildermark\n\nThe user wants to rate this conversation.", RawJSON: `{}`},
			},
		},
	})
	s.Agents = registry

	ctx := context.Background()
	projectID, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-1", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	if err := db.InsertMessages(ctx, s.DB, []db.Message{
		{Timestamp: 1000, ProjectID: projectID, ConversationID: "conv-1", Role: "agent", Content: "first assistant reply", RawJSON: `{}`},
		{Timestamp: 2000, ProjectID: projectID, ConversationID: "conv-1", Role: "user", Content: "real prompt", RawJSON: `{}`},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"conversationId": "temp-1",
		"rating":         5,
		"note":           "still great",
		"agent":          "codex",
	})
	req := httptest.NewRequest("POST", "/api/v1/rating", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	insertedResolvedMessages := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := s.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ?", "conv-1").Scan(&count); err == nil && count >= 4 {
			insertedResolvedMessages = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !insertedResolvedMessages {
		t.Fatal("timed out waiting for resolved rating messages to be inserted")
	}

	var startedAt, endedAt int64
	if err := s.DB.QueryRow("SELECT started_at, ended_at FROM conversations WHERE id = ?", "conv-1").Scan(&startedAt, &endedAt); err != nil {
		t.Fatalf("query bounds: %v", err)
	}
	if startedAt != 1000 {
		t.Errorf("started_at = %d, want 1000", startedAt)
	}
	if endedAt != 2000 {
		t.Errorf("ended_at = %d, want 2000", endedAt)
	}
}
