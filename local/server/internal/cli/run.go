package cli

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent/claude"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent/codex"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent/gemini"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/handler"
)

// RunOptions configures the server startup.
type RunOptions struct {
	DBPath string
	Addr   string
}

// RunServer starts the buildermark server and blocks until ctx is cancelled.
func RunServer(ctx context.Context, opts RunOptions) error {
	readOnly, _ := strconv.ParseBool(os.Getenv("READ_ONLY"))

	database, err := db.InitDB(opts.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	configDir, err := DefaultConfigDir()
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	cfg := DefaultConfig()
	if loaded, err := LoadConfig(configDir); err != nil {
		log.Printf("load config: %v; continuing with defaults", err)
	} else {
		cfg = loaded
	}

	registry := agent.NewRegistry()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("determine home directory: %w", err)
	}

	watchedHomes := map[string]struct{}{home: {}}
	registerHome := func(h string) {
		registry.Register(claude.NewForHome(database, h))
		registry.Register(codex.NewForHome(database, h))
		registry.Register(gemini.NewForHome(database, h))
	}

	// Register the primary home and extra homes from config.
	registerHome(home)
	for _, candidate := range cfg.ExtraAgentHomes {
		clean := normalizeHomePath(candidate)
		if clean == "" {
			continue
		}
		if _, ok := watchedHomes[clean]; ok {
			continue
		}
		watchedHomes[clean] = struct{}{}
		registerHome(clean)
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	for _, w := range registry.Watchers() {
		go w.Run(watchCtx)
	}

	hsrv := &handler.Server{
		DB:              database,
		Agents:          registry,
		DBPath:          opts.DBPath,
		ListenAddr:      opts.Addr,
		ReadOnly:        readOnly,
		ConfigDir:       configDir,
		PluginSourceDir: resolvePluginSourceDir(),
	}

	// ReloadWatchers reads the current config and starts watchers for any
	// homes that aren't already being watched. Returns the new home paths.
	hsrv.ReloadWatchers = func() []string {
		latestCfg, err := LoadConfig(configDir)
		if err != nil {
			log.Printf("reload watchers: failed to load config: %v", err)
			return nil
		}
		var added []string
		for _, candidate := range latestCfg.ExtraAgentHomes {
			clean := normalizeHomePath(candidate)
			if clean == "" {
				continue
			}
			if _, ok := watchedHomes[clean]; ok {
				continue
			}
			watchedHomes[clean] = struct{}{}
			registerHome(clean)
			added = append(added, clean)
		}
		// Start the newly registered watchers.
		watchers := registry.Watchers()
		for _, w := range watchers[len(watchers)-len(added)*3:] {
			go w.Run(watchCtx)
		}
		if len(added) > 0 {
			log.Printf("reload watchers: started watchers for %d new home(s)", len(added))
		}
		return added
	}

	srv := &http.Server{
		Addr:         opts.Addr,
		Handler:      hsrv.Routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("Buildermark Local server listening on %s\n", opts.Addr)
		if readOnly {
			fmt.Println("READ_ONLY mode enabled: mutating API endpoints are disabled")
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
	}

	watchCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	log.Println("server stopped")
	return nil
}

// normalizeHomePath cleans a candidate home path, stripping any trailing
// agent-specific directory (e.g. ".claude"). Returns "" for empty input.
func normalizeHomePath(candidate string) string {
	if candidate == "" {
		return ""
	}
	clean := filepath.Clean(candidate)
	if filepath.Base(clean) == ".claude" || filepath.Base(clean) == ".codex" || filepath.Base(clean) == ".gemini" {
		clean = filepath.Dir(clean)
	}
	return clean
}

func resolvePluginSourceDir() string {
	candidates := make([]string, 0, 2)
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}

	for _, candidate := range candidates {
		dir := filepath.Clean(candidate)
		for {
			pluginDir := filepath.Join(dir, "plugins")
			if pluginBundleExists(pluginDir) {
				return pluginDir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return ""
}

func pluginBundleExists(dir string) bool {
	required := []string{
		"claudecode/.claude-plugin/plugin.json",
		"claudecode/skills/bbrate/SKILL.md",
		"codex/skills/bbrate/SKILL.md",
		"gemini/commands/bbrate.toml",
	}
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			return false
		}
	}
	return true
}
