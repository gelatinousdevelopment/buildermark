package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
)

type localSettingsResponse struct {
	HomePath                string            `json:"homePath"`
	DBPath                  string            `json:"dbPath"`
	ListenAddr              string            `json:"listenAddr"`
	ConversationSearchPaths []agentSearchPath `json:"conversationSearchPaths"`
	ExtraAgentHomes         []string          `json:"extraAgentHomes"`
}

type localConfigFile struct {
	UpdateMode      string   `json:"updateMode"`
	ExtraAgentHomes []string `json:"extraAgentHomes,omitempty"`
}

type agentSearchPath struct {
	Agent  string `json:"agent"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

func (s *Server) handleGetLocalSettings(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to determine home directory")
		return
	}

	cfg := localConfigFile{}
	if s.ConfigDir != "" {
		if loaded, err := loadLocalConfigFile(s.ConfigDir); err == nil {
			cfg = loaded
		}
	}
	extraHomes := normalizeHomeEntries(cfg.ExtraAgentHomes)
	paths := collectConversationSearchPaths(home, extraHomes)

	dbPath := s.DBPath
	if abs, err := filepath.Abs(dbPath); err == nil {
		dbPath = abs
	}

	writeSuccess(w, http.StatusOK, localSettingsResponse{
		HomePath:                home,
		DBPath:                  dbPath,
		ListenAddr:              s.ListenAddr,
		ConversationSearchPaths: paths,
		ExtraAgentHomes:         extraHomes,
	})
}

func (s *Server) handlePutLocalSettings(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var req struct {
		ExtraAgentHomes []string `json:"extraAgentHomes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if s.ConfigDir == "" {
		writeError(w, http.StatusInternalServerError, "settings config directory is unavailable")
		return
	}
	cfg, err := loadLocalConfigFile(s.ConfigDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}
	cfg.ExtraAgentHomes = normalizeHomeEntries(req.ExtraAgentHomes)
	if err := saveLocalConfigFile(s.ConfigDir, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	// Reload watchers for any newly added homes and trigger a scan.
	if s.ReloadWatchers != nil {
		if newHomes := s.ReloadWatchers(); len(newHomes) > 0 {
			if s.importMu.TryLock() {
				go s.runHistoryScanJob(time.Now().Add(-agent.DefaultScanWindow), "")
			}
		}
	}

	s.handleGetLocalSettings(w, r)
}

func loadLocalConfigFile(configDir string) (localConfigFile, error) {
	path := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return localConfigFile{}, nil
		}
		return localConfigFile{}, err
	}
	cfg := localConfigFile{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return localConfigFile{}, err
	}
	return cfg, nil
}

func saveLocalConfigFile(configDir string, cfg localConfigFile) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(configDir, "config.json"), data, 0o644)
}

func normalizeHomeEntries(raw []string) []string {
	result := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, candidate := range raw {
		if candidate == "" {
			continue
		}
		clean := filepath.Clean(candidate)
		if filepath.Base(clean) == ".claude" || filepath.Base(clean) == ".codex" || filepath.Base(clean) == ".gemini" {
			clean = filepath.Dir(clean)
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	sort.Strings(result)
	return result
}

func collectConversationSearchPaths(home string, extraHomes []string) []agentSearchPath {
	paths := make([]agentSearchPath, 0)
	homes := append([]string{home}, extraHomes...)
	seen := make(map[string]struct{})
	for _, root := range homes {
		for _, agentName := range []string{"claude", "codex", "gemini"} {
			path := conversationSearchPathForAgent(root, agentName)
			key := agentName + "|" + path
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			_, err := os.Stat(path)
			paths = append(paths, agentSearchPath{Agent: agentName, Path: path, Exists: err == nil})
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		if paths[i].Agent == paths[j].Agent {
			return paths[i].Path < paths[j].Path
		}
		return paths[i].Agent < paths[j].Agent
	})
	return paths
}

func conversationSearchPathForAgent(home, agentName string) string {
	switch agentName {
	case "claude":
		return filepath.Join(home, ".claude")
	case "codex":
		return filepath.Join(home, ".codex")
	case "gemini":
		return filepath.Join(home, ".gemini")
	default:
		return filepath.Join(home, "."+agentName)
	}
}
