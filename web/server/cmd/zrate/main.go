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

	"github.com/davidcann/zrate/web/server/internal/agent"
	"github.com/davidcann/zrate/web/server/internal/agent/claude"
	"github.com/davidcann/zrate/web/server/internal/db"
	"github.com/davidcann/zrate/web/server/internal/handler"
)

func main() {
	defaultDB := "../../.data/zrate.db"
	if env := os.Getenv("ZRATE_DB_PATH"); env != "" {
		defaultDB = env
	}

	dbPath := flag.String("db", defaultDB, "path to SQLite database file")
	addr := flag.String("addr", ":7022", "listen address")
	flag.Parse()

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

	watchCtx, watchCancel := context.WithCancel(context.Background())
	defer watchCancel()

	for _, w := range registry.Watchers() {
		go w.Run(watchCtx)
	}

	srv := &http.Server{
		Addr:         *addr,
		Handler:      (&handler.Server{DB: database, Agents: registry}).Routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("zrate server listening on %s\n", *addr)
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
