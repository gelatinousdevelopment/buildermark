package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/cli"
)

func main() {
	defaultDB := "../../.data/local.db"
	if env := os.Getenv("BUILDERMARK_LOCAL_DB_PATH"); env != "" {
		defaultDB = env
	}

	dbPath := flag.String("db", defaultDB, "path to SQLite database file")
	addr := flag.String("addr", ":55022", "listen address")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-done
		fmt.Println()
		log.Println("shutting down...")
		cancel()
	}()

	if err := cli.RunServer(ctx, cli.RunOptions{DBPath: *dbPath, Addr: *addr}); err != nil {
		log.Fatalf("error: %v", err)
	}
}
