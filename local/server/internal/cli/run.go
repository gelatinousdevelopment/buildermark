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
	cfg, err := LoadConfig(configDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	registry := agent.NewRegistry()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("determine home directory: %w", err)
	}
	homes := []string{home}
	seen := map[string]struct{}{home: {}}
	for _, candidate := range cfg.ExtraAgentHomes {
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
		homes = append(homes, clean)
	}

	for _, watchHome := range homes {
		registry.Register(claude.NewForHome(database, watchHome))
		registry.Register(codex.NewForHome(database, watchHome))
		registry.Register(gemini.NewForHome(database, watchHome))
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	for _, w := range registry.Watchers() {
		go w.Run(watchCtx)
	}

	srv := &http.Server{
		Addr:         opts.Addr,
		Handler:      (&handler.Server{DB: database, Agents: registry, DBPath: opts.DBPath, ListenAddr: opts.Addr, ReadOnly: readOnly, ConfigDir: configDir}).Routes(),
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
