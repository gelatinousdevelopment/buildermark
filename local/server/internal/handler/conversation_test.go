package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gelatinousdevelopment/buildermark/web/server/internal/db"
)

func TestListConversations(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	// Seed data: project + conversations.
	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	for _, id := range []string{"conv-a", "conv-b"} {
		if err := db.EnsureConversation(ctx, s.DB, id, pid, "claude"); err != nil {
			t.Fatalf("EnsureConversation: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "/api/v1/conversations", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Errorf("ok = false, want true")
	}

	data, ok := env.Data.([]any)
	if !ok {
		t.Fatalf("data is not an array: %T", env.Data)
	}
	if len(data) != 2 {
		t.Errorf("got %d conversations, want 2", len(data))
	}
}

func TestListConversationsHiddenFilter(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	for _, id := range []string{"conv-a", "conv-b"} {
		if err := db.EnsureConversation(ctx, s.DB, id, pid, "claude"); err != nil {
			t.Fatalf("EnsureConversation: %v", err)
		}
	}
	if err := db.SetConversationHidden(ctx, s.DB, "conv-b", true); err != nil {
		t.Fatalf("SetConversationHidden: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/conversations", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode visible response: %v", err)
	}
	data, ok := env.Data.([]any)
	if !ok {
		t.Fatalf("visible data is not array: %T", env.Data)
	}
	if len(data) != 1 {
		t.Fatalf("visible count = %d, want 1", len(data))
	}

	req = httptest.NewRequest("GET", "/api/v1/conversations?hidden=true", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("hidden status = %d, want %d", rec.Code, http.StatusOK)
	}
	env = jsonEnvelope{}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode hidden response: %v", err)
	}
	data, ok = env.Data.([]any)
	if !ok {
		t.Fatalf("hidden data is not array: %T", env.Data)
	}
	if len(data) != 1 {
		t.Fatalf("hidden count = %d, want 1", len(data))
	}
}

func TestListConversationsEmpty(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/v1/conversations", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	data, ok := env.Data.([]any)
	if !ok {
		t.Fatalf("data is not an array: %T", env.Data)
	}
	if len(data) != 0 {
		t.Errorf("got %d conversations, want 0", len(data))
	}
}

func TestGetConversation(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-1", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Add messages and a rating.
	messages := []db.Message{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "conv-1", Role: "user", Content: "hello"},
		{Timestamp: 2000, ProjectID: pid, ConversationID: "conv-1", Role: "agent", Content: "hi"},
	}
	if err := db.InsertMessages(ctx, s.DB, messages); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}
	if _, err := db.InsertRating(ctx, s.DB, "conv-1", 4, "nice", ""); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/conversations/conv-1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Error("ok = false, want true")
	}

	// Verify the response has the expected structure.
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	if data["id"] != "conv-1" {
		t.Errorf("id = %v, want %q", data["id"], "conv-1")
	}
	if got := int(data["startedAt"].(float64)); got != 1000 {
		t.Errorf("startedAt = %d, want %d", got, 1000)
	}
	if got := int(data["endedAt"].(float64)); got != 2000 {
		t.Errorf("endedAt = %d, want %d", got, 2000)
	}

	messagesData, ok := data["messages"].([]any)
	if !ok {
		t.Fatalf("messages is not an array: %T", data["messages"])
	}
	if len(messagesData) != 2 {
		t.Errorf("got %d messages, want 2", len(messagesData))
	}

	ratingsData, ok := data["ratings"].([]any)
	if !ok {
		t.Fatalf("ratings is not an array: %T", data["ratings"])
	}
	if len(ratingsData) != 1 {
		t.Errorf("got %d ratings, want 1", len(ratingsData))
	}
}

func TestGetConversationNotFound(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/v1/conversations/nonexistent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if env.OK {
		t.Error("ok = true, want false")
	}
}

func TestGetConversationByTempConversationID(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-1", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	if err := db.InsertMessages(ctx, s.DB, []db.Message{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "conv-1", Role: "user", Content: "hello"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}
	if _, err := db.InsertRatingWithTemp(ctx, s.DB, "conv-1", "temp-1", 4, "nice", ""); err != nil {
		t.Fatalf("InsertRatingWithTemp: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/conversations/temp-1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatal("ok = false, want true")
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	if data["id"] != "conv-1" {
		t.Errorf("id = %v, want %q", data["id"], "conv-1")
	}
}

func TestSetConversationHidden(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()
	ctx := context.Background()

	pid, err := db.EnsureProject(ctx, s.DB, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := db.EnsureConversation(ctx, s.DB, "conv-1", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	body := []byte(`{"hidden":true}`)
	req := httptest.NewRequest("POST", "/api/v1/conversations/conv-1/hidden", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var env jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok = false, want true")
	}

	conv, err := db.GetConversation(ctx, s.DB, "conv-1")
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if conv == nil || !conv.Hidden {
		t.Fatalf("conversation hidden = false, want true")
	}
}
