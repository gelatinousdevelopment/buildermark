package cli

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestRunServer_ContextCancel(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/test.db"

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunServer(ctx, RunOptions{
			DBPath: dbPath,
			Addr:   "127.0.0.1:0",
		})
	}()

	// Give server a moment to start.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunServer() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunServer() did not stop within timeout")
	}
}

func TestRunServer_InvalidDBPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := RunServer(ctx, RunOptions{
		DBPath: "/nonexistent/path/to/db",
		Addr:   "127.0.0.1:0",
	})
	if err == nil {
		t.Error("RunServer() expected error for invalid DB path, got nil")
	}
}

func TestRunServer_ServesRequests(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/test.db"

	// Use a specific port for testing.
	addr := "127.0.0.1:17199"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunServer(ctx, RunOptions{
			DBPath: dbPath,
			Addr:   addr,
		})
	}()

	// Wait for server to be ready.
	client := &http.Client{Timeout: time.Second}
	var ready bool
	for i := 0; i < 20; i++ {
		resp, err := client.Get("http://" + addr + "/api/v1/settings")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatal("server did not become ready")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunServer() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunServer() did not stop within timeout")
	}
}
