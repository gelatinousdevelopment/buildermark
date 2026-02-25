package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetLocalSettings(t *testing.T) {
	t.Setenv("HOME", "/tmp/buildermark-home")

	w1 := &mockWatcher{name: "codex"}
	w2 := &mockWatcher{name: "claude"}
	s := setupTestServerWithWatcher(t, w1, w2)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/v1/settings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			HomePath                string `json:"homePath"`
			ConversationSearchPaths []struct {
				Agent string `json:"agent"`
				Path  string `json:"path"`
			} `json:"conversationSearchPaths"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !env.OK {
		t.Fatal("ok = false, want true")
	}
	if env.Data.HomePath != "/tmp/buildermark-home" {
		t.Fatalf("homePath = %q, want %q", env.Data.HomePath, "/tmp/buildermark-home")
	}
	if len(env.Data.ConversationSearchPaths) != 2 {
		t.Fatalf("conversationSearchPaths len = %d, want 2", len(env.Data.ConversationSearchPaths))
	}

	if got := env.Data.ConversationSearchPaths[0]; got.Agent != "claude" || got.Path != "/tmp/buildermark-home/.claude" {
		t.Fatalf("first path = %+v, want agent=claude path=/tmp/buildermark-home/.claude", got)
	}
	if got := env.Data.ConversationSearchPaths[1]; got.Agent != "codex" || got.Path != "/tmp/buildermark-home/.codex" {
		t.Fatalf("second path = %+v, want agent=codex path=/tmp/buildermark-home/.codex", got)
	}
}

func TestGetLocalSettingsWithoutAgents(t *testing.T) {
	t.Setenv("HOME", "/tmp/buildermark-home")

	s := setupTestServer(t)
	handler := s.Routes()

	req := httptest.NewRequest("GET", "/api/v1/settings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			HomePath                string `json:"homePath"`
			ConversationSearchPaths []struct {
				Agent string `json:"agent"`
				Path  string `json:"path"`
			} `json:"conversationSearchPaths"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !env.OK {
		t.Fatal("ok = false, want true")
	}
	if env.Data.HomePath != "/tmp/buildermark-home" {
		t.Fatalf("homePath = %q, want %q", env.Data.HomePath, "/tmp/buildermark-home")
	}
	if len(env.Data.ConversationSearchPaths) != 0 {
		t.Fatalf("conversationSearchPaths len = %d, want 0", len(env.Data.ConversationSearchPaths))
	}
}
