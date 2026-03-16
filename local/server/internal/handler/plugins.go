package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type pluginInventoryResponse struct {
	Agents []pluginAgentInfo `json:"agents"`
	Homes  []pluginHomeInfo  `json:"homes"`
}

type pluginAgentInfo struct {
	Agent  string `json:"agent"`
	Name   string `json:"name"`
	Syntax string `json:"syntax"`
}

type pluginHomeInfo struct {
	HomePath  string           `json:"homePath"`
	IsPrimary bool             `json:"isPrimary"`
	Plugins   []pluginFileInfo `json:"plugins"`
}

type pluginFileInfo struct {
	Agent         string   `json:"agent"`
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Installed     bool     `json:"installed"`
	RelativePaths []string `json:"relativePaths"`
	Paths         []string `json:"paths"`
}

type pluginReplacement struct {
	old string
	new string
}

type pluginFileDefinition struct {
	sourcePath   string
	installPath  string
	executable   bool
	replacements []pluginReplacement
}

type pluginDefinition struct {
	agent        string
	name         string
	syntax       string
	files        []pluginFileDefinition
	cleanupPaths []string
}

type pluginHomeDescriptor struct {
	path      string
	isPrimary bool
}

func (s *Server) handleGetPlugins(w http.ResponseWriter, r *http.Request) {
	inventory, err := s.buildPluginInventory()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeSuccess(w, http.StatusOK, inventory)
}

func (s *Server) handlePostPlugins(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}

	var req struct {
		HomePath string `json:"homePath"`
		Agent    string `json:"agent"`
		Install  bool   `json:"install"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	homes, err := s.listPluginHomes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	normalizedHomes := normalizeHomeEntries([]string{req.HomePath})
	if len(normalizedHomes) != 1 {
		writeError(w, http.StatusBadRequest, "homePath is required")
		return
	}
	homePath := normalizedHomes[0]

	allowedHomes := make(map[string]struct{}, len(homes))
	for _, home := range homes {
		allowedHomes[home.path] = struct{}{}
	}
	if _, ok := allowedHomes[homePath]; !ok {
		writeError(w, http.StatusBadRequest, "homePath is not a managed agent home")
		return
	}

	def, ok := pluginDefinitionByAgent(req.Agent)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown agent")
		return
	}

	if req.Install {
		if err := s.installPlugin(homePath, def); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		if err := uninstallPlugin(homePath, def); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	inventory, err := s.buildPluginInventory()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeSuccess(w, http.StatusOK, inventory)
}

func (s *Server) buildPluginInventory() (pluginInventoryResponse, error) {
	homes, err := s.listPluginHomes()
	if err != nil {
		return pluginInventoryResponse{}, err
	}

	defs := pluginDefinitions()
	resp := pluginInventoryResponse{
		Agents: make([]pluginAgentInfo, 0, len(defs)),
		Homes:  make([]pluginHomeInfo, 0, len(homes)),
	}
	for _, def := range defs {
		resp.Agents = append(resp.Agents, pluginAgentInfo{
			Agent:  def.agent,
			Name:   def.name,
			Syntax: def.syntax,
		})
	}
	for _, home := range homes {
		row := pluginHomeInfo{
			HomePath:  home.path,
			IsPrimary: home.isPrimary,
			Plugins:   make([]pluginFileInfo, 0, len(defs)),
		}
		for _, def := range defs {
			row.Plugins = append(row.Plugins, pluginStatusForHome(home.path, def))
		}
		resp.Homes = append(resp.Homes, row)
	}
	return resp, nil
}

func (s *Server) listPluginHomes() ([]pluginHomeDescriptor, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine home directory")
	}

	homes := []pluginHomeDescriptor{{path: home, isPrimary: true}}
	if s.ConfigDir == "" {
		return homes, nil
	}

	cfg, err := loadLocalConfigFile(s.ConfigDir)
	if err != nil {
		log.Printf("plugins: failed to load config: %v", err)
		return homes, nil
	}

	for _, extra := range normalizeHomeEntries(cfg.ExtraAgentHomes) {
		if extra == home {
			continue
		}
		homes = append(homes, pluginHomeDescriptor{path: extra})
	}
	return homes, nil
}

func pluginDefinitions() []pluginDefinition {
	return []pluginDefinition{
		{
			agent:  "claude",
			name:   "Claude Code CLI",
			syntax: "/rate-buildermark",
			files: []pluginFileDefinition{
				{
					sourcePath:  "claudecode/skills/rate-buildermark/SKILL.md",
					installPath: ".claude/skills/rate-buildermark/SKILL.md",
					replacements: []pluginReplacement{
						{
							old: `"$(git rev-parse --show-toplevel)/plugins/claudecode/skills/rate-buildermark/scripts/submit-rating.sh"`,
							new: `"$HOME/.claude/skills/rate-buildermark/scripts/submit-rating.sh"`,
						},
					},
				},
				{
					sourcePath:  "claudecode/skills/rate-buildermark/scripts/submit-rating.sh",
					installPath: ".claude/skills/rate-buildermark/scripts/submit-rating.sh",
					executable:  true,
				},
			},
			cleanupPaths: []string{
				".claude/skills/rate-buildermark",
			},
		},
		{
			agent:  "codex",
			name:   "Codex CLI",
			syntax: "$rate-buildermark",
			files: []pluginFileDefinition{
				{
					sourcePath:  "codex/skills/rate-buildermark/SKILL.md",
					installPath: ".codex/skills/rate-buildermark/SKILL.md",
					replacements: []pluginReplacement{
						{
							old: "bash plugins/codex/skills/rate-buildermark/scripts/submit-rating.sh",
							new: `bash "$HOME/.codex/skills/rate-buildermark/scripts/submit-rating.sh"`,
						},
					},
				},
				{
					sourcePath:  "codex/skills/rate-buildermark/scripts/submit-rating.sh",
					installPath: ".codex/skills/rate-buildermark/scripts/submit-rating.sh",
					executable:  true,
				},
			},
			cleanupPaths: []string{
				".codex/skills/rate-buildermark",
			},
		},
		{
			agent:  "gemini",
			name:   "Gemini CLI",
			syntax: "/rate-buildermark",
			files: []pluginFileDefinition{
				{
					sourcePath:  "gemini/commands/rate-buildermark.toml",
					installPath: ".gemini/commands/rate-buildermark.toml",
					replacements: []pluginReplacement{
						{
							old: `bash plugins/gemini/scripts/submit-rating.sh`,
							new: `bash \"$HOME/.gemini/scripts/submit-rating.sh\"`,
						},
					},
				},
				{
					sourcePath:  "gemini/scripts/submit-rating.sh",
					installPath: ".gemini/scripts/submit-rating.sh",
					executable:  true,
				},
			},
			cleanupPaths: []string{
				".gemini/commands/rate-buildermark.toml",
				".gemini/scripts/submit-rating.sh",
			},
		},
		{
			agent:  "cursor",
			name:   "Cursor IDE",
			syntax: "/rate-buildermark",
			files: []pluginFileDefinition{
				{
					sourcePath:  "cursor/skills/rate-buildermark/SKILL.md",
					installPath: ".cursor/skills/rate-buildermark/SKILL.md",
					replacements: []pluginReplacement{
						{
							old: `"$(git rev-parse --show-toplevel)/plugins/cursor/skills/rate-buildermark/scripts/submit-rating.sh"`,
							new: `"$HOME/.cursor/skills/rate-buildermark/scripts/submit-rating.sh"`,
						},
					},
				},
				{
					sourcePath:  "cursor/skills/rate-buildermark/scripts/submit-rating.sh",
					installPath: ".cursor/skills/rate-buildermark/scripts/submit-rating.sh",
					executable:  true,
				},
			},
			cleanupPaths: []string{
				".cursor/skills/rate-buildermark",
			},
		},
	}
}

func pluginDefinitionByAgent(agent string) (pluginDefinition, bool) {
	for _, def := range pluginDefinitions() {
		if def.agent == agent {
			return def, true
		}
	}
	return pluginDefinition{}, false
}

func pluginStatusForHome(homePath string, def pluginDefinition) pluginFileInfo {
	existingFiles := 0
	for _, file := range def.files {
		if fileExists(filepath.Join(homePath, file.installPath)) {
			existingFiles++
		}
	}

	status := "missing"
	installed := false
	switch {
	case existingFiles == 0:
	case existingFiles == len(def.files):
		status = "installed"
		installed = true
	default:
		status = "partial"
	}

	relativePaths := slices.Clone(def.cleanupPaths)
	paths := make([]string, 0, len(relativePaths))
	for _, rel := range relativePaths {
		paths = append(paths, filepath.Join(homePath, rel))
	}

	return pluginFileInfo{
		Agent:         def.agent,
		Name:          def.name,
		Status:        status,
		Installed:     installed,
		RelativePaths: relativePaths,
		Paths:         paths,
	}
}

func (s *Server) installPlugin(homePath string, def pluginDefinition) error {
	sourceDir, err := s.resolvePluginSourceDir()
	if err != nil {
		return err
	}

	if err := uninstallPlugin(homePath, def); err != nil {
		return err
	}

	for _, file := range def.files {
		sourcePath := filepath.Join(sourceDir, file.sourcePath)
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to read plugin bundle %s", file.sourcePath)
		}

		text := string(content)
		for _, replacement := range file.replacements {
			text = strings.ReplaceAll(text, replacement.old, replacement.new)
		}

		targetPath := filepath.Join(homePath, file.installPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("failed to create plugin directory %s", filepath.Dir(targetPath))
		}

		mode := os.FileMode(0o644)
		if file.executable {
			mode = 0o755
		}
		if err := os.WriteFile(targetPath, []byte(text), mode); err != nil {
			return fmt.Errorf("failed to write plugin file %s", targetPath)
		}
	}

	return nil
}

func uninstallPlugin(homePath string, def pluginDefinition) error {
	for _, rel := range def.cleanupPaths {
		target := filepath.Join(homePath, rel)
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("failed to remove plugin path %s", target)
		}
	}
	return nil
}

func (s *Server) resolvePluginSourceDir() (string, error) {
	if isPluginSourceDir(s.PluginSourceDir) {
		return s.PluginSourceDir, nil
	}

	candidates := make([]string, 0, 2)
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}

	for _, candidate := range candidates {
		if dir, ok := findPluginSourceDir(candidate); ok {
			return dir, nil
		}
	}

	return "", fmt.Errorf("plugin source directory is unavailable")
}

func findPluginSourceDir(start string) (string, bool) {
	current := filepath.Clean(start)
	for {
		candidate := filepath.Join(current, "plugins")
		if isPluginSourceDir(candidate) {
			return candidate, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func isPluginSourceDir(dir string) bool {
	if dir == "" {
		return false
	}
	required := []string{
		"claudecode/skills/rate-buildermark/SKILL.md",
		"codex/skills/rate-buildermark/SKILL.md",
		"gemini/commands/rate-buildermark.toml",
		"cursor/skills/rate-buildermark/SKILL.md",
	}
	for _, rel := range required {
		if !fileExists(filepath.Join(dir, rel)) {
			return false
		}
	}
	return true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
