package gemini

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
)

func (a *Agent) ResolveSession(rating int, note string, fallbackID string) *agent.SessionResult {
	if path := findSessionFile(a.tmpDir, fallbackID, ""); path != "" {
		entries, project, sessionID := a.collectSessionEntries(path)
		if sessionID == "" {
			sessionID = fallbackID
		}
		return &agent.SessionResult{SessionID: sessionID, Project: project, Entries: entries}
	}

	const (
		pollInterval = 500 * time.Millisecond
		maxWait      = 5 * time.Second
		maxAge       = 30 * time.Second
	)

	var sessionID string
	var projectHash string
	deadline := time.Now().Add(maxWait)
	for {
		sid, hash, ok := searchRecentLogs(a.tmpDir, rating, note, maxAge)
		if ok {
			sessionID = sid
			projectHash = hash
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}

	if sessionID == "" {
		log.Printf("gemini session: no match found for /bb %d %q, using fallback", rating, note)
		return &agent.SessionResult{SessionID: fallbackID}
	}

	path := findSessionFile(a.tmpDir, sessionID, projectHash)
	if path == "" {
		log.Printf("gemini session: matched sessionId=%s from logs, but chat file not found", sessionID)
		return &agent.SessionResult{SessionID: sessionID}
	}

	entries, project, resolvedSessionID := a.collectSessionEntries(path)
	if resolvedSessionID != "" {
		sessionID = resolvedSessionID
	}

	return &agent.SessionResult{SessionID: sessionID, Project: project, Entries: entries}
}

func searchRecentLogs(tmpDir string, rating int, note string, maxAge time.Duration) (string, string, bool) {
	cutoff := time.Now().Add(-maxAge)
	wantedNote := strings.TrimSpace(note)

	var matchSID string
	var matchHash string

	filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || matchSID != "" {
			return nil
		}
		if info.Name() != "logs.json" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var entries []geminiLogEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil
		}

		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			if e.Type != "user" {
				continue
			}
			ts := parseGeminiTimestamp(e.Timestamp)
			if ts <= 0 {
				continue
			}
			entryTime := time.UnixMilli(ts)
			if entryTime.Before(cutoff) {
				break
			}

			r, n := parseRatingDisplay(e.Message)
			if r == rating && strings.TrimSpace(n) == wantedNote && e.SessionID != "" {
				matchSID = e.SessionID
				matchHash = filepath.Base(filepath.Dir(path))
				return filepath.SkipAll
			}
		}

		return nil
	})

	if matchSID == "" {
		return "", "", false
	}
	return matchSID, matchHash, true
}

func findSessionFile(tmpDir, sessionID, projectHash string) string {
	if sessionID == "" {
		return ""
	}

	candidates := make([]string, 0, 2)
	if projectHash != "" {
		candidates = append(candidates, filepath.Join(tmpDir, projectHash, "chats"))
	}
	candidates = append(candidates, tmpDir)

	for _, root := range candidates {
		var match string
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || match != "" {
				return nil
			}
			if !strings.HasSuffix(info.Name(), ".json") || !strings.Contains(path, string(filepath.Separator)+"chats"+string(filepath.Separator)) {
				return nil
			}
			if !strings.Contains(info.Name(), sessionID[:min(len(sessionID), 8)]) {
				return nil
			}

			conv, err := readConversation(path)
			if err != nil {
				return nil
			}
			if conv.SessionID == sessionID {
				match = path
				return filepath.SkipAll
			}
			return nil
		})
		if match != "" {
			return match
		}
	}

	return ""
}

func (a *Agent) collectSessionEntries(path string) ([]agent.Entry, string, string) {
	conv, err := readConversation(path)
	if err != nil {
		log.Printf("gemini session: error reading file %s: %v", path, err)
		return nil, "", ""
	}

	project := a.resolveProjectPath(conv)
	entries := make([]agent.Entry, 0, len(conv.Messages))

	for _, m := range conv.Messages {
		display := extractMessageText(m)
		if strings.TrimSpace(display) == "" {
			continue
		}

		role := "agent"
		if m.Type == "user" {
			role = "user"
		}
		model := ""
		if role == "agent" {
			model = agent.FirstNonEmpty(strings.TrimSpace(m.Model), strings.TrimSpace(m.ModelName), strings.TrimSpace(conv.Model))
		}

		rawJSON, _ := json.Marshal(m)

		ts := parseGeminiTimestamp(m.Timestamp)
		if ts <= 0 {
			continue
		}

		entries = append(entries, agent.Entry{
			Timestamp: ts,
			SessionID: conv.SessionID,
			Project:   project,
			Role:      role,
			Model:     model,
			Display:   display,
			RawJSON:   string(rawJSON),
		})
	}

	return agent.AppendDiffEntries(entries), project, conv.SessionID
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
