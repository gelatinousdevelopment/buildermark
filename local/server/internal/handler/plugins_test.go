package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetPluginsIncludesPrimaryAndExtraHomes(t *testing.T) {
	primaryHome := t.TempDir()
	extraHome := t.TempDir()
	t.Setenv("HOME", primaryHome)

	s := setupTestServer(t)
	s.ConfigDir = t.TempDir()
	if err := saveLocalConfigFile(s.ConfigDir, localConfigFile{
		ExtraAgentHomes: []string{
			filepath.Join(extraHome, ".codex"),
			primaryHome,
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(primaryHome, ".codex", "skills", "bbrate"), 0o755); err != nil {
		t.Fatalf("mkdir primary codex skill dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(primaryHome, ".codex", "skills", "bbrate", "SKILL.md"),
		[]byte("installed"),
		0o644,
	); err != nil {
		t.Fatalf("write primary codex skill: %v", err)
	}

	inventory, err := s.buildPluginInventory()
	if err != nil {
		t.Fatalf("build inventory: %v", err)
	}

	if len(inventory.Homes) != 2 {
		t.Fatalf("homes len = %d, want 2", len(inventory.Homes))
	}

	if got := inventory.Homes[0]; got.HomePath != primaryHome || !got.IsPrimary {
		t.Fatalf("first home = %+v, want primary %s", got, primaryHome)
	}
	if got := inventory.Homes[1]; got.HomePath != extraHome || got.IsPrimary {
		t.Fatalf("second home = %+v, want extra %s", got, extraHome)
	}

	codexPrimary := findPluginInfo(t, inventory.Homes[0], "codex")
	if codexPrimary.Status != "partial" {
		t.Fatalf("primary codex status = %q, want partial", codexPrimary.Status)
	}
	if !strings.HasSuffix(codexPrimary.Paths[0], filepath.Join(".codex", "skills", "bbrate")) {
		t.Fatalf("primary codex path = %q", codexPrimary.Paths[0])
	}

	claudeExtra := findPluginInfo(t, inventory.Homes[1], "claude")
	if claudeExtra.Status != "missing" {
		t.Fatalf("extra claude status = %q, want missing", claudeExtra.Status)
	}
}

func TestPostPluginsInstallAndUninstall(t *testing.T) {
	sourceDir := createTestPluginBundle(t)

	tests := []struct {
		name             string
		agent            string
		verifyPath       string
		replacedContains string
		removedPath      string
	}{
		{
			name:             "claude",
			agent:            "claude",
			verifyPath:       filepath.Join(".claude", "plugins", "buildermark", "skills", "bbrate", "SKILL.md"),
			replacedContains: `"$HOME/.claude/plugins/buildermark/skills/bbrate/scripts/submit-rating.sh"`,
			removedPath:      filepath.Join(".claude", "plugins", "buildermark"),
		},
		{
			name:             "codex",
			agent:            "codex",
			verifyPath:       filepath.Join(".codex", "skills", "bbrate", "SKILL.md"),
			replacedContains: `bash "$HOME/.codex/skills/bbrate/scripts/submit-rating.sh"`,
			removedPath:      filepath.Join(".codex", "skills", "bbrate"),
		},
		{
			name:             "gemini",
			agent:            "gemini",
			verifyPath:       filepath.Join(".gemini", "commands", "bbrate.toml"),
			replacedContains: `bash \"$HOME/.gemini/scripts/submit-rating.sh\"`,
			removedPath:      filepath.Join(".gemini", "commands", "bbrate.toml"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)

			s := setupTestServer(t)
			s.PluginSourceDir = sourceDir

			reqBody, _ := json.Marshal(map[string]any{
				"homePath": homeDir,
				"agent":    tt.agent,
				"install":  true,
			})
			req := newJSONRequest(t, "POST", "/api/v1/plugins", reqBody)
			rec := serveRequest(t, s, req)
			if rec.Code != 200 {
				t.Fatalf("install status = %d, want 200", rec.Code)
			}

			verifyFile := filepath.Join(homeDir, tt.verifyPath)
			content, err := os.ReadFile(verifyFile)
			if err != nil {
				t.Fatalf("read installed file %s: %v", verifyFile, err)
			}
			if !strings.Contains(string(content), tt.replacedContains) {
				t.Fatalf("installed file %s missing replacement %q", verifyFile, tt.replacedContains)
			}

			scriptPath := installedScriptPath(tt.agent, homeDir)
			info, err := os.Stat(scriptPath)
			if err != nil {
				t.Fatalf("stat installed script %s: %v", scriptPath, err)
			}
			if info.Mode().Perm() != 0o755 {
				t.Fatalf("script mode = %o, want 755", info.Mode().Perm())
			}

			reqBody, _ = json.Marshal(map[string]any{
				"homePath": homeDir,
				"agent":    tt.agent,
				"install":  false,
			})
			req = newJSONRequest(t, "POST", "/api/v1/plugins", reqBody)
			rec = serveRequest(t, s, req)
			if rec.Code != 200 {
				t.Fatalf("uninstall status = %d, want 200", rec.Code)
			}

			if _, err := os.Stat(filepath.Join(homeDir, tt.removedPath)); !os.IsNotExist(err) {
				t.Fatalf("removed path %s still exists or unexpected err: %v", tt.removedPath, err)
			}
		})
	}
}

func createTestPluginBundle(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"claudecode/.claude-plugin/plugin.json":             `{"name":"Buildermark"}`,
		"claudecode/skills/bbrate/SKILL.md":                 `"$(git rev-parse --show-toplevel)/plugins/claudecode/skills/bbrate/scripts/submit-rating.sh"`,
		"claudecode/skills/bbrate/scripts/submit-rating.sh": "#!/bin/sh\nexit 0\n",
		"codex/skills/bbrate/SKILL.md":                      "bash plugins/codex/skills/bbrate/scripts/submit-rating.sh\n",
		"codex/skills/bbrate/scripts/submit-rating.sh":      "#!/bin/sh\nexit 0\n",
		"gemini/commands/bbrate.toml":                       "bash plugins/gemini/scripts/submit-rating.sh\n",
		"gemini/scripts/submit-rating.sh":                   "#!/bin/sh\nexit 0\n",
	}
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	return root
}

func installedScriptPath(agent, home string) string {
	switch agent {
	case "claude":
		return filepath.Join(home, ".claude", "plugins", "buildermark", "skills", "bbrate", "scripts", "submit-rating.sh")
	case "codex":
		return filepath.Join(home, ".codex", "skills", "bbrate", "scripts", "submit-rating.sh")
	case "gemini":
		return filepath.Join(home, ".gemini", "scripts", "submit-rating.sh")
	default:
		return ""
	}
}

func findPluginInfo(t *testing.T, home pluginHomeInfo, agent string) pluginFileInfo {
	t.Helper()
	for _, plugin := range home.Plugins {
		if plugin.Agent == agent {
			return plugin
		}
	}
	t.Fatalf("plugin %s not found in %+v", agent, home)
	return pluginFileInfo{}
}

func newJSONRequest(t *testing.T, method, path string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func serveRequest(t *testing.T, s *Server, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}
