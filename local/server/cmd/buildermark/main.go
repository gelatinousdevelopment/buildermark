package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gelatinousdevelopment/buildermark/web/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/web/server/internal/agent/claude"
	"github.com/gelatinousdevelopment/buildermark/web/server/internal/agent/codex"
	"github.com/gelatinousdevelopment/buildermark/web/server/internal/agent/gemini"
	"github.com/gelatinousdevelopment/buildermark/web/server/internal/db"
	"github.com/gelatinousdevelopment/buildermark/web/server/internal/handler"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")

	defaultDB := "../../.data/local.db"
	if env := os.Getenv("BUILDERMARK_LOCAL_DB_PATH"); env != "" {
		defaultDB = env
	}

	dbPath := flag.String("db", defaultDB, "path to SQLite database file")
	addr := flag.String("addr", ":7022", "listen address")
	flag.Parse()

	if *showVersion {
		fmt.Printf("buildermark %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	database, err := db.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	registry := agent.NewRegistry()

	claudeAgent, err := claude.New(database)
	if err != nil {
		log.Printf("warning: claude agent disabled: %v", err)
	} else {
		registry.Register(claudeAgent)
	}

	codexAgent, err := codex.New(database)
	if err != nil {
		log.Printf("warning: codex agent disabled: %v", err)
	} else {
		registry.Register(codexAgent)
	}

	geminiAgent, err := gemini.New(database)
	if err != nil {
		log.Printf("warning: gemini agent disabled: %v", err)
	} else {
		registry.Register(geminiAgent)
	}

	watchCtx, watchCancel := context.WithCancel(context.Background())
	defer watchCancel()

	for _, w := range registry.Watchers() {
		go w.Run(watchCtx)
	}

	srv := &http.Server{
		Addr:         *addr,
		Handler:      (&handler.Server{DB: database, Agents: registry, DBPath: *dbPath, ListenAddr: *addr}).Routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("Buildermark Local server listening on %s\n", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-done
	log.Println("shutting down...")
	watchCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("server stopped")
}
