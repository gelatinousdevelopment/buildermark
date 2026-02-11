package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeHistoryFile(t *testing.T, path string, entries []historyEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create history file: %v", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("encode entry: %v", err)
		}
	}
}

func TestSearchHistoryMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()
	entries := []historyEntry{
		{Display: "some other command", Timestamp: now.Add(-10 * time.Second).UnixMilli(), SessionID: "sess-old", Type: "user"},
		{Display: "/zrate 4 good work", Timestamp: now.Add(-2 * time.Second).UnixMilli(), SessionID: "sess-123", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	sid, ok := searchHistory(path, "/zrate 4 good work", 64*1024, 30*time.Second)
	if !ok {
		t.Fatal("expected match, got none")
	}
	if sid != "sess-123" {
		t.Errorf("sessionID = %q, want %q", sid, "sess-123")
	}
}

func TestSearchHistoryNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()
	entries := []historyEntry{
		{Display: "/zrate 3 different", Timestamp: now.Add(-2 * time.Second).UnixMilli(), SessionID: "sess-1", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	_, ok := searchHistory(path, "/zrate 5 good work", 64*1024, 30*time.Second)
	if ok {
		t.Error("expected no match")
	}
}

func TestSearchHistoryTooOld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	entries := []historyEntry{
		{Display: "/zrate 4 good work", Timestamp: time.Now().Add(-5 * time.Minute).UnixMilli(), SessionID: "sess-old", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	_, ok := searchHistory(path, "/zrate 4 good work", 64*1024, 30*time.Second)
	if ok {
		t.Error("expected no match for entry older than maxAge")
	}
}

func TestSearchHistoryMissingFile(t *testing.T) {
	_, ok := searchHistory("/nonexistent/path/history.jsonl", "/zrate 3", 64*1024, 30*time.Second)
	if ok {
		t.Error("expected no match for missing file")
	}
}

func TestSearchHistoryEmptySessionID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()
	entries := []historyEntry{
		{Display: "/zrate 4 test", Timestamp: now.Add(-2 * time.Second).UnixMilli(), SessionID: "", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	_, ok := searchHistory(path, "/zrate 4 test", 64*1024, 30*time.Second)
	if ok {
		t.Error("expected no match when sessionID is empty")
	}
}

func TestCollectSessionEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	entries := []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-1", Project: "/my/project", Type: "user"},
		{Display: "hi there", Timestamp: 2000, SessionID: "sess-1", Project: "/my/project", Type: "assistant"},
		{Display: "unrelated", Timestamp: 3000, SessionID: "sess-other", Project: "/other", Type: "user"},
		{Display: "thanks", Timestamp: 4000, SessionID: "sess-1", Project: "/my/project", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	result := collectSessionEntries(dir, path, "sess-1")

	if len(result) != 3 {
		t.Fatalf("got %d entries, want 3", len(result))
	}

	// Verify chronological order.
	if result[0].Timestamp != 1000 {
		t.Errorf("first entry timestamp = %d, want 1000", result[0].Timestamp)
	}
	if result[2].Timestamp != 4000 {
		t.Errorf("last entry timestamp = %d, want 4000", result[2].Timestamp)
	}

	// Verify role mapping.
	if result[0].Role != "user" {
		t.Errorf("first entry role = %q, want %q", result[0].Role, "user")
	}
	if result[1].Role != "agent" {
		t.Errorf("second entry role = %q, want %q", result[1].Role, "agent")
	}

	// Verify project is captured.
	if result[0].Project != "/my/project" {
		t.Errorf("project = %q, want %q", result[0].Project, "/my/project")
	}
}

func TestCollectSessionEntriesNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	entries := []historyEntry{
		{Display: "hello", Timestamp: 1000, SessionID: "sess-other", Type: "user"},
	}
	writeHistoryFile(t, path, entries)

	result := collectSessionEntries(dir, path, "sess-nonexistent")
	if len(result) != 0 {
		t.Errorf("got %d entries, want 0", len(result))
	}
}

func TestCollectSessionEntriesMissingFile(t *testing.T) {
	result := collectSessionEntries("/tmp", "/nonexistent/history.jsonl", "sess-1")
	if result != nil {
		t.Errorf("expected nil for missing file, got %v", result)
	}
}

func TestResolvePastedContents(t *testing.T) {
	dir := t.TempDir()

	// Set up the paste-cache directory structure.
	cacheDir := filepath.Join(dir, ".claude", "paste-cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a cached paste file.
	hash := "abc123"
	if err := os.WriteFile(filepath.Join(cacheDir, hash+".txt"), []byte("pasted content here"), 0644); err != nil {
		t.Fatalf("write paste cache: %v", err)
	}

	display := "Before [Pasted text #1] after"
	pasted := map[string]pastedContent{
		"1": {ID: 1, Type: "text", ContentHash: hash},
	}

	result := resolvePastedContents(dir, display, pasted)
	expected := "Before pasted content here after"
	if result != expected {
		t.Errorf("result = %q, want %q", result, expected)
	}
}

func TestResolvePastedContentsNonTextType(t *testing.T) {
	dir := t.TempDir()

	display := "Before [Pasted text #1] after"
	pasted := map[string]pastedContent{
		"1": {ID: 1, Type: "image", ContentHash: "abc123"},
	}

	// Non-text types should not be replaced.
	result := resolvePastedContents(dir, display, pasted)
	if result != display {
		t.Errorf("result = %q, want %q (unchanged)", result, display)
	}
}

func TestResolvePastedContentsMissingCache(t *testing.T) {
	dir := t.TempDir()

	display := "Before [Pasted text #1] after"
	pasted := map[string]pastedContent{
		"1": {ID: 1, Type: "text", ContentHash: "nonexistent"},
	}

	// Missing cache file should leave the placeholder.
	result := resolvePastedContents(dir, display, pasted)
	if result != display {
		t.Errorf("result = %q, want %q (unchanged)", result, display)
	}
}

func TestSearchHistoryTailBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Now()

	// Write many old entries to pad the file, then a matching entry at the end.
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	enc := json.NewEncoder(f)

	// Write enough filler to exceed 1KB.
	for i := 0; i < 50; i++ {
		enc.Encode(historyEntry{
			Display:   fmt.Sprintf("filler entry %d with extra padding text to take up space", i),
			Timestamp: now.Add(-1 * time.Minute).UnixMilli(),
			SessionID: "sess-filler",
			Type:      "user",
		})
	}

	// Write the entry we want to find.
	enc.Encode(historyEntry{
		Display:   "/zrate 5 found it",
		Timestamp: now.Add(-1 * time.Second).UnixMilli(),
		SessionID: "sess-target",
		Type:      "user",
	})
	f.Close()

	// Use a small tailBytes to only read the end of file.
	sid, ok := searchHistory(path, "/zrate 5 found it", 1024, 30*time.Second)
	if !ok {
		t.Fatal("expected match in tail of file")
	}
	if sid != "sess-target" {
		t.Errorf("sessionID = %q, want %q", sid, "sess-target")
	}
}
