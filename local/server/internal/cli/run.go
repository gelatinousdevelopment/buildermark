package cli

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent/claude"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent/codex"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent/cursor"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent/gemini"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/gitmonitor"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/handler"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/updater"
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

	if n, err := db.ResetStuckCommitSyncStates(ctx, database); err != nil {
		log.Printf("warning: failed to reset stuck sync states: %v", err)
	} else if n > 0 {
		log.Printf("reset %d stuck commit sync state(s)", n)
	}

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
	registerHome := func(h string, importAllClaude bool) {
		if importAllClaude {
			registry.Register(claude.NewForHomeImportAll(database, h))
		} else {
			registry.Register(claude.NewForHome(database, h))
		}
		registry.Register(codex.NewForHome(database, h))
		registry.Register(gemini.NewForHome(database, h))
		registry.Register(cursor.NewForHome(database, h))
	}

	// Register the primary home and extra homes from config.
	registerHome(home, false)
	for _, candidate := range cfg.ExtraAgentHomes {
		clean := normalizeHomePath(candidate)
		if clean == "" {
			continue
		}
		if _, ok := watchedHomes[clean]; ok {
			continue
		}
		watchedHomes[clean] = struct{}{}
		registerHome(clean, true)
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	for i, w := range registry.Watchers() {
		delay := time.Duration(i) * 5 * time.Second
		go func(w agent.Watcher, d time.Duration) {
			if d > 0 {
				select {
				case <-time.After(d):
				case <-watchCtx.Done():
					return
				}
			}
			w.Run(watchCtx)
		}(w, delay)
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
	hsrv.SetVersion(Version)

	// Detect "just installed" update from marker file.
	detectPostUpdate(hsrv, configDir)

	hsrv.RepoMonitor = gitmonitor.New(ctx, gitmonitor.Options{
		OnBranchChange: hsrv.HandleGitBranchChange,
	})
	hsrv.ReconcileGitRepoMonitor(ctx)

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
			registerHome(clean, true)
			added = append(added, clean)
		}
		// Start the newly registered watchers.
		watchers := registry.Watchers()
		newWatchers := watchers[len(watchers)-len(added)*4:]
		for i, w := range newWatchers {
			delay := time.Duration(i) * 5 * time.Second
			go func(w agent.Watcher, d time.Duration) {
				if d > 0 {
					select {
					case <-time.After(d):
					case <-watchCtx.Done():
						return
					}
				}
				w.Run(watchCtx)
			}(w, delay)
		}
		if len(added) > 0 {
			log.Printf("reload watchers: started watchers for %d new home(s)", len(added))
		}
		return added
	}

	mux := hsrv.Routes()

	go func() {
		select {
		case <-time.After(3 * time.Second):
		case <-ctx.Done():
			return
		}
		hsrv.RefreshStaleProjects(ctx)
	}()

	// Periodic update check for Linux CLI.
	if cfg.UpdateMode != "off" {
		go runPeriodicUpdateCheck(ctx, hsrv, cfg)
	}

	srv := &http.Server{
		Addr:         opts.Addr,
		Handler:      mux,
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
	if hsrv.RepoMonitor != nil {
		hsrv.RepoMonitor.Close()
	}

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
	if filepath.Base(clean) == ".claude" || filepath.Base(clean) == ".codex" || filepath.Base(clean) == ".gemini" || filepath.Base(clean) == ".cursor" {
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

// detectPostUpdate checks for a pre-update marker file and sets the "installed"
// update status if the version changed since the marker was written.
func detectPostUpdate(hsrv *handler.Server, configDir string) {
	markerPath := filepath.Join(configDir, "pre-update-version")
	data, err := os.ReadFile(markerPath)
	if err != nil {
		return
	}
	os.Remove(markerPath)

	previousVersion := strings.TrimSpace(string(data))
	if previousVersion != "" && previousVersion != Version {
		hsrv.SetUpdateStatus(handler.UpdateStatusEvent{
			State:           "installed",
			Version:         Version,
			PreviousVersion: previousVersion,
			Platform:        runtime.GOOS,
		})
		log.Printf("update detected: %s -> %s", previousVersion, Version)
	}
}

// runPeriodicUpdateCheck checks for updates periodically on Linux CLI.
func runPeriodicUpdateCheck(ctx context.Context, hsrv *handler.Server, cfg Config) {
	// Wait 30 seconds after startup before first check.
	select {
	case <-time.After(30 * time.Second):
	case <-ctx.Done():
		return
	}

	check := func() {
		u := updater.GetUpdater(Version)
		result, err := u.Check()
		if err != nil {
			log.Printf("periodic update check failed: %v", err)
			return
		}
		if !result.HasUpdate {
			return
		}

		if cfg.UpdateMode == "auto" {
			log.Printf("auto-applying update: %s -> %s", result.CurrentVersion, result.LatestVersion)
			if err := u.Apply(result); err != nil {
				log.Printf("auto-update apply failed: %v", err)
				return
			}
			// Signal that the update was installed; systemd will restart the binary.
			hsrv.SetUpdateStatus(handler.UpdateStatusEvent{
				State:           "installed",
				Version:         result.LatestVersion,
				PreviousVersion: result.CurrentVersion,
				Platform:        runtime.GOOS,
			})
			return
		}

		// "check" mode: just notify that an update is available.
		hsrv.SetUpdateStatus(handler.UpdateStatusEvent{
			State:    "available",
			Version:  result.LatestVersion,
			Platform: runtime.GOOS,
		})
	}

	check()

	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			check()
		case <-ctx.Done():
			return
		}
	}
}

func pluginBundleExists(dir string) bool {
	required := []string{
		"claudecode/skills/rate-buildermark/SKILL.md",
		"codex/skills/rate-buildermark/SKILL.md",
		"gemini/commands/rate-buildermark.toml",
		"cursor/skills/rate-buildermark/SKILL.md",
	}
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			return false
		}
	}
	return true
}
