package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type historyEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"`
	SessionID string `json:"sessionId"`
	Project   string `json:"project"`
}

// resolveSessionID polls history.jsonl for up to 5 seconds looking for a
// /zrate entry that matches the given rating and note. Returns the real
// Claude Code sessionId if found, otherwise returns fallbackID.
func resolveSessionID(rating int, note string, fallbackID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return fallbackID
	}
	histPath := filepath.Join(home, ".claude", "history.jsonl")

	expectedDisplay := fmt.Sprintf("/zrate %d", rating)
	if note != "" {
		expectedDisplay += " " + note
	}

	const (
		pollInterval = 500 * time.Millisecond
		maxWait      = 5 * time.Second
		tailBytes    = int64(64 * 1024)
		maxAge       = 30 * time.Second
	)

	deadline := time.Now().Add(maxWait)
	for {
		if sid, ok := searchHistory(histPath, expectedDisplay, tailBytes, maxAge); ok {
			log.Printf("history: matched entry with sessionId=%s", sid)
			return sid
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}

	log.Printf("history: no match found for %q, using fallback", expectedDisplay)
	return fallbackID
}

// searchHistory reads the last tailBytes of the history file and searches
// lines in reverse for an entry whose display field matches expectedDisplay
// and whose timestamp is within maxAge of now.
func searchHistory(path, expectedDisplay string, tailBytes int64, maxAge time.Duration) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", false
	}

	offset := int64(0)
	readSize := info.Size()
	if readSize > tailBytes {
		offset = readSize - tailBytes
		readSize = tailBytes
	}

	buf := make([]byte, readSize)
	if _, err := f.ReadAt(buf, offset); err != nil && err != io.EOF {
		return "", false
	}

	lines := strings.Split(string(buf), "\n")
	now := time.Now()

	// iterate in reverse (most recent first)
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		var entry historyEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		entryTime := time.UnixMilli(entry.Timestamp)
		if now.Sub(entryTime) > maxAge {
			// entries are chronological; older ones won't match either
			break
		}

		if entry.Display == expectedDisplay && entry.SessionID != "" {
			return entry.SessionID, true
		}
	}

	return "", false
}
