package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
				Agent  string `json:"agent"`
				Path   string `json:"path"`
				Exists bool   `json:"exists"`
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
	if len(env.Data.ConversationSearchPaths) != 3 {
		t.Fatalf("conversationSearchPaths len = %d, want 3", len(env.Data.ConversationSearchPaths))
	}

	if got := env.Data.ConversationSearchPaths[0]; got.Agent != "claude" || got.Path != "/tmp/buildermark-home/.claude" || got.Exists {
		t.Fatalf("first path = %+v, want agent=claude path=/tmp/buildermark-home/.claude exists=false", got)
	}
	if got := env.Data.ConversationSearchPaths[1]; got.Agent != "codex" || got.Path != "/tmp/buildermark-home/.codex" || got.Exists {
		t.Fatalf("second path = %+v, want agent=codex path=/tmp/buildermark-home/.codex exists=false", got)
	}
}

func TestPutLocalSettings(t *testing.T) {
	t.Setenv("HOME", "/tmp/buildermark-home")
	s := setupTestServer(t)
	configDir := t.TempDir()
	s.ConfigDir = configDir
	handler := s.Routes()

	body, _ := json.Marshal(map[string]any{"extraAgentHomes": []string{"/mnt/vm/user", "/mnt/vm/user/.claude", ""}})
	req := httptest.NewRequest("PUT", "/api/v1/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	cfgPath := filepath.Join(configDir, "config.json")
	var cfg struct {
		ExtraAgentHomes []string `json:"extraAgentHomes"`
	}
	content, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := json.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if len(cfg.ExtraAgentHomes) != 1 || cfg.ExtraAgentHomes[0] != "/mnt/vm/user" {
		t.Fatalf("saved extraAgentHomes = %#v, want [/mnt/vm/user]", cfg.ExtraAgentHomes)
	}
}
