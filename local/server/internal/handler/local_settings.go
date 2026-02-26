package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

type localSettingsResponse struct {
	HomePath                string            `json:"homePath"`
	DBPath                  string            `json:"dbPath"`
	ListenAddr              string            `json:"listenAddr"`
	ConversationSearchPaths []agentSearchPath `json:"conversationSearchPaths"`
}

type agentSearchPath struct {
	Agent string `json:"agent"`
	Path  string `json:"path"`
}

func (s *Server) handleGetLocalSettings(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to determine home directory")
		return
	}

	paths := make([]agentSearchPath, 0)
	if s.Agents != nil {
		seen := make(map[string]struct{})
		for _, watcher := range s.Agents.Watchers() {
			agentName := watcher.Name()
			path := conversationSearchPathForAgent(home, agentName)
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, agentSearchPath{
				Agent: agentName,
				Path:  path,
			})
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		return paths[i].Agent < paths[j].Agent
	})

	dbPath := s.DBPath
	if abs, err := filepath.Abs(dbPath); err == nil {
		dbPath = abs
	}

	writeSuccess(w, http.StatusOK, localSettingsResponse{
		HomePath:                home,
		DBPath:                  dbPath,
		ListenAddr:              s.ListenAddr,
		ConversationSearchPaths: paths,
	})
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
