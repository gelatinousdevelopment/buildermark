package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
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

func TestPutLocalSettingsScopesRescanToNewHomes(t *testing.T) {
	t.Setenv("HOME", "/tmp/buildermark-home")
	oldWatcher := &mockWatcher{name: "claude", home: "/homes/old"}
	newWatcher := &mockWatcher{name: "claude", home: "/homes/new"}
	oldDiscoverer := &mockDiscoverer{name: "claude", home: "/homes/old", paths: []string{"/tmp/old-project"}}
	newDiscoverer := &mockDiscoverer{name: "claude", home: "/homes/new", paths: []string{"/tmp/new-project"}}
	s := setupTestServerWithAgents(t, oldWatcher, newWatcher, oldDiscoverer, newDiscoverer)
	configDir := t.TempDir()
	s.ConfigDir = configDir
	s.ReloadWatchers = func() []string { return []string{"/homes/new"} }
	handler := s.Routes()

	var gotPaths []string
	s.historyScanRecompute = func(ctx context.Context, since time.Time, paths []string, broadcast func(string, string)) {
		_ = ctx
		_ = since
		_ = broadcast
		gotPaths = append([]string(nil), paths...)
	}

	body, _ := json.Marshal(map[string]any{"extraAgentHomes": []string{"/homes/new"}})
	req := httptest.NewRequest("PUT", "/api/v1/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	waitForImportUnlock(s)

	oldScanCount, _, _, _ := oldWatcher.snapshot()
	newScanCount, _, _, _ := newWatcher.snapshot()
	if oldScanCount != 0 {
		t.Fatalf("old watcher scanCount = %d, want 0", oldScanCount)
	}
	if newScanCount != 1 {
		t.Fatalf("new watcher scanCount = %d, want 1", newScanCount)
	}

	if !reflect.DeepEqual(gotPaths, []string{"/tmp/new-project"}) {
		t.Fatalf("recompute paths = %#v, want [/tmp/new-project]", gotPaths)
	}
}

func TestPutLocalSettingsQueuesRescanWhenImportBusy(t *testing.T) {
	t.Setenv("HOME", "/tmp/buildermark-home")
	newWatcher := &mockWatcher{name: "claude", home: "/homes/new"}
	newDiscoverer := &mockDiscoverer{name: "claude", home: "/homes/new", paths: []string{"/tmp/new-project"}}
	s := setupTestServerWithAgents(t, newWatcher, newDiscoverer)
	configDir := t.TempDir()
	s.ConfigDir = configDir
	s.ReloadWatchers = func() []string { return []string{"/homes/new"} }
	handler := s.Routes()
	done := make(chan struct{}, 1)
	s.historyScanRecompute = func(ctx context.Context, since time.Time, paths []string, broadcast func(string, string)) {
		_ = ctx
		_ = since
		_ = paths
		_ = broadcast
		done <- struct{}{}
	}

	locked := true
	s.importMu.Lock()
	defer func() {
		if locked {
			s.importMu.Unlock()
		}
	}()

	body, _ := json.Marshal(map[string]any{"extraAgentHomes": []string{"/homes/new"}})
	req := httptest.NewRequest("PUT", "/api/v1/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if scanCount, _, _, _ := newWatcher.snapshot(); scanCount != 0 {
		t.Fatalf("scanCount while import lock held = %d, want 0", scanCount)
	}

	s.importMu.Unlock()
	locked = false
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for queued history scan")
	}
	waitForImportUnlock(s)

	scanCount, _, _, _ := newWatcher.snapshot()
	if scanCount != 1 {
		t.Fatalf("scanCount after releasing import lock = %d, want 1", scanCount)
	}
}

func TestPutLocalSettingsSkipsRescanWithoutNewHomes(t *testing.T) {
	t.Setenv("HOME", "/tmp/buildermark-home")
	w := &mockWatcher{name: "claude", home: "/homes/existing"}
	s := setupTestServerWithAgents(t, w)
	configDir := t.TempDir()
	s.ConfigDir = configDir
	s.ReloadWatchers = func() []string { return nil }
	handler := s.Routes()

	recomputeCalls := 0
	s.historyScanRecompute = func(ctx context.Context, since time.Time, paths []string, broadcast func(string, string)) {
		_ = ctx
		_ = since
		_ = paths
		_ = broadcast
		recomputeCalls++
	}

	body, _ := json.Marshal(map[string]any{"extraAgentHomes": []string{"/homes/existing"}})
	req := httptest.NewRequest("PUT", "/api/v1/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	scanCount, _, _, _ := w.snapshot()
	if scanCount != 0 {
		t.Fatalf("watcher scanCount = %d, want 0", scanCount)
	}
	if recomputeCalls != 0 {
		t.Fatalf("recomputeCalls = %d, want 0", recomputeCalls)
	}
}

func TestNotificationsEnabledDefault(t *testing.T) {
	s := setupTestServer(t)
	s.ConfigDir = t.TempDir()

	// No config file exists → default true.
	if !s.notificationsEnabled() {
		t.Fatal("notificationsEnabled() = false, want true (default)")
	}
}

func TestNotificationsEnabledExplicit(t *testing.T) {
	configDir := t.TempDir()

	// Explicitly false.
	f := false
	cfg := localConfigFile{NotificationsEnabled: &f}
	if err := saveLocalConfigFile(configDir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadLocalConfigFile(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if effectiveNotificationsEnabled(loaded) {
		t.Fatal("effectiveNotificationsEnabled = true, want false")
	}

	// Explicitly true.
	tr := true
	cfg.NotificationsEnabled = &tr
	if err := saveLocalConfigFile(configDir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadLocalConfigFile(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if !effectiveNotificationsEnabled(loaded) {
		t.Fatal("effectiveNotificationsEnabled = false, want true")
	}
}

func TestNotificationsEnabledConfigRoundTrip(t *testing.T) {
	configDir := t.TempDir()

	// Save with notificationsEnabled = false.
	f := false
	cfg := localConfigFile{NotificationsEnabled: &f}
	if err := saveLocalConfigFile(configDir, cfg); err != nil {
		t.Fatal(err)
	}

	// Load and verify.
	loaded, err := loadLocalConfigFile(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if effectiveNotificationsEnabled(loaded) {
		t.Fatal("expected false after round-trip, got true")
	}

	// Save with notificationsEnabled = true.
	tr := true
	loaded.NotificationsEnabled = &tr
	if err := saveLocalConfigFile(configDir, loaded); err != nil {
		t.Fatal(err)
	}

	loaded2, err := loadLocalConfigFile(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if !effectiveNotificationsEnabled(loaded2) {
		t.Fatal("expected true after round-trip, got false")
	}
}
