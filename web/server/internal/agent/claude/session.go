package claude

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/davidcann/zrate/web/server/internal/agent"
)

type pastedContent struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	ContentHash string `json:"contentHash"`
}

type historyEntry struct {
	Display        string                   `json:"display"`
	Timestamp      int64                    `json:"timestamp"`
	SessionID      string                   `json:"sessionId"`
	Project        string                   `json:"project"`
	Type           string                   `json:"type"`
	PastedContents map[string]pastedContent `json:"pastedContents"`
}

// ResolveSession polls history.jsonl for up to 5 seconds looking for a
// /zrate entry that matches the given rating and note. When found it
// collects every entry with the same sessionId and returns them all.
// If no match is found the fallbackID is returned with no entries.
func (a *Agent) ResolveSession(rating int, note string, fallbackID string) *agent.SessionResult {
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

	var sessionID string
	deadline := time.Now().Add(maxWait)
	for {
		if sid, ok := searchHistory(a.path, expectedDisplay, tailBytes, maxAge); ok {
			sessionID = sid
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}

	if sessionID == "" {
		log.Printf("claude session: no match found for %q, using fallback", expectedDisplay)
		return &agent.SessionResult{SessionID: fallbackID}
	}

	log.Printf("claude session: matched entry with sessionId=%s", sessionID)

	entries := collectSessionEntries(a.home, a.path, sessionID)

	project := ""
	if len(entries) > 0 {
		project = entries[0].Project
	}

	return &agent.SessionResult{
		SessionID: sessionID,
		Project:   project,
		Entries:   entries,
	}
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
			break
		}

		if entry.Display == expectedDisplay && entry.SessionID != "" {
			return entry.SessionID, true
		}
	}

	return "", false
}

// collectSessionEntries reads the full history file and returns every entry
// whose sessionId matches the given id, ordered chronologically.
func collectSessionEntries(home, path, sessionID string) []agent.Entry {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("claude session: error reading file for session collection: %v", err)
		return nil
	}

	lines := strings.Split(string(data), "\n")
	var entries []agent.Entry

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry historyEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.SessionID != sessionID {
			continue
		}

		role := "user"
		if entry.Type == "assistant" {
			role = "agent"
		}

		display := entry.Display
		if len(entry.PastedContents) > 0 {
			display = resolvePastedContents(home, display, entry.PastedContents)
		}

		entries = append(entries, agent.Entry{
			Timestamp: entry.Timestamp,
			SessionID: entry.SessionID,
			Project:   entry.Project,
			Role:      role,
			Display:   display,
			RawJSON:   line,
		})
	}

	return entries
}

// resolvePastedContents replaces [Pasted text #N] placeholders in display
// with the actual content from ~/.claude/paste-cache/{contentHash}.txt.
func resolvePastedContents(home, display string, pasted map[string]pastedContent) string {
	for _, pc := range pasted {
		if pc.Type != "text" || pc.ContentHash == "" {
			continue
		}

		placeholder := fmt.Sprintf("[Pasted text #%d]", pc.ID)
		cachePath := filepath.Join(home, ".claude", "paste-cache", pc.ContentHash+".txt")

		content, err := os.ReadFile(cachePath)
		if err != nil {
			log.Printf("claude session: failed to read paste cache %s: %v", cachePath, err)
			continue
		}

		display = strings.Replace(display, placeholder, string(content), 1)
	}
	return display
}
