package codex

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/davidcann/zrate/web/server/internal/agent"
)

// ResolveSession attempts to resolve a rating to a real Codex session.
// If the fallbackID matches a known thread ID (e.g. from CODEX_THREAD_ID),
// it uses that directly. Otherwise, it scans recent session files for a
// $bb entry matching the given rating and note.
func (a *Agent) ResolveSession(rating int, note string, fallbackID string) *agent.SessionResult {
	// First, try using fallbackID as a thread ID (it may be CODEX_THREAD_ID).
	if path := findSessionFile(a.sessionsDir, fallbackID); path != "" {
		log.Printf("codex session: resolved thread %s from file %s", fallbackID, filepath.Base(path))
		entries, project := collectSessionEntries(path)
		return &agent.SessionResult{
			SessionID: fallbackID,
			Project:   project,
			Entries:   entries,
		}
	}

	const (
		pollInterval = 500 * time.Millisecond
		maxWait      = 5 * time.Second
		maxAge       = 30 * time.Second
	)

	var matchPath string
	var matchThreadID string
	deadline := time.Now().Add(maxWait)
	for {
		matchPath, matchThreadID = a.searchRecentFiles(rating, note, maxAge)
		if matchPath != "" {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}

	if matchPath == "" {
		log.Printf("codex session: no match found for $bb %d %q, using fallback", rating, note)
		return &agent.SessionResult{SessionID: fallbackID}
	}

	log.Printf("codex session: matched entry in %s with threadId=%s", filepath.Base(matchPath), matchThreadID)

	entries, project := collectSessionEntries(matchPath)
	sessionID := matchThreadID
	if sessionID == "" {
		sessionID = fallbackID
	}

	return &agent.SessionResult{
		SessionID: sessionID,
		Project:   project,
		Entries:   entries,
	}
}

// searchRecentFiles scans session files modified within maxAge for a $bb
// command matching the given rating and note.
func (a *Agent) searchRecentFiles(rating int, note string, maxAge time.Duration) (string, string) {
	cutoff := time.Now().Add(-maxAge)

	var matchPath string
	var matchThreadID string

	filepath.Walk(a.sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || matchPath != "" {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		var threadID string
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var event codexSessionLine
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}

			switch event.Type {
			case "session_meta":
				var meta codexSessionMetaPayload
				if err := json.Unmarshal(event.Payload, &meta); err == nil && meta.ID != "" && threadID == "" {
					threadID = meta.ID
				}

			case "response_item":
				var item codexResponseItemPayload
				if err := json.Unmarshal(event.Payload, &item); err != nil || item.Type != "message" || item.Role != "user" {
					continue
				}
				if r, n := parseZrateDisplay(extractResponseItemText(item.Content)); r == rating && strings.TrimSpace(n) == strings.TrimSpace(note) {
					matchPath = path
					matchThreadID = threadID
					return filepath.SkipAll
				}

			case "event_msg":
				var msg codexEventMsgPayload
				if err := json.Unmarshal(event.Payload, &msg); err != nil || msg.Type != "user_message" {
					continue
				}
				if r, n := parseZrateDisplay(msg.Message); r == rating && strings.TrimSpace(n) == strings.TrimSpace(note) {
					matchPath = path
					matchThreadID = threadID
					return filepath.SkipAll
				}

			case "input":
				// Legacy schema.
				if event.ThreadID != "" && threadID == "" {
					threadID = event.ThreadID
				}
				if r, n := parseZrateDisplay(event.Content); r == rating && strings.TrimSpace(n) == strings.TrimSpace(note) {
					matchPath = path
					matchThreadID = threadID
					return filepath.SkipAll
				}

			case "item.completed":
				// Legacy schema.
				if event.ThreadID != "" && threadID == "" {
					threadID = event.ThreadID
				}
				for _, c := range event.Item.Content {
					if (c.Type == "text" || c.Type == "input_text") && c.Text != "" {
						if r, n := parseZrateDisplay(c.Text); r == rating && strings.TrimSpace(n) == strings.TrimSpace(note) {
							matchPath = path
							matchThreadID = threadID
							return filepath.SkipAll
						}
					}
				}
			}
		}

		return nil
	})

	return matchPath, matchThreadID
}

// collectSessionEntries parses a rollout JSONL file into agent.Entry slices.
// Returns the entries and the project path (working directory).
func collectSessionEntries(path string) ([]agent.Entry, string) {
	f, err := os.Open(path)
	if err != nil {
		log.Printf("codex session: error reading file %s: %v", path, err)
		return nil, ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var entries []agent.Entry
	var threadID string
	var project string
	var currentModel string
	var responseItemUserIdx []int
	hasEventMsgUser := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event codexSessionLine
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if m := extractCodexModelFromRawLine(line); m != "" {
			currentModel = m
		}

		ts := parseCodexTimestamp(event.Timestamp)
		if ts <= 0 {
			continue
		}

		switch event.Type {
		case "session_meta":
			var meta codexSessionMetaPayload
			if err := json.Unmarshal(event.Payload, &meta); err != nil {
				continue
			}
			if meta.ID != "" && threadID == "" {
				threadID = meta.ID
			}
			if meta.Cwd != "" && project == "" {
				project = meta.Cwd
			}

		case "turn_context":
			var ctx codexTurnContextPayload
			if err := json.Unmarshal(event.Payload, &ctx); err != nil {
				continue
			}
			if ctx.Cwd != "" && project == "" {
				project = ctx.Cwd
			}

		case "response_item":
			var item codexResponseItemPayload
			if err := json.Unmarshal(event.Payload, &item); err != nil || item.Type != "message" {
				continue
			}

			role := "user"
			if item.Role == "assistant" {
				role = "agent"
			} else if item.Role != "user" {
				continue
			}

			content := extractResponseItemText(item.Content)
			if content == "" {
				continue
			}

			entries = append(entries, agent.Entry{
				Timestamp: ts,
				SessionID: threadID,
				Project:   project,
				Role:      role,
				Model:     currentModel,
				Display:   content,
				RawJSON:   line,
			})
			if item.Role == "user" {
				responseItemUserIdx = append(responseItemUserIdx, len(entries)-1)
			}

		case "event_msg":
			var msg codexEventMsgPayload
			if err := json.Unmarshal(event.Payload, &msg); err != nil || msg.Type != "user_message" {
				continue
			}
			content := strings.TrimSpace(msg.Message)
			if content == "" {
				continue
			}
			hasEventMsgUser = true
			entries = append(entries, agent.Entry{
				Timestamp: ts,
				SessionID: threadID,
				Project:   project,
				Role:      "user",
				Model:     currentModel,
				Display:   content,
				RawJSON:   line,
			})

		case "input":
			// Legacy schema.
			if event.ThreadID != "" && threadID == "" {
				threadID = event.ThreadID
			}
			if event.WorkingDir != "" && project == "" {
				project = event.WorkingDir
			}

			content := strings.TrimSpace(event.Content)
			if content == "" {
				continue
			}
			entries = append(entries, agent.Entry{
				Timestamp: ts,
				SessionID: threadID,
				Project:   project,
				Role:      "user",
				Model:     currentModel,
				Display:   content,
				RawJSON:   line,
			})

		case "item.completed":
			// Legacy schema.
			if event.ThreadID != "" && threadID == "" {
				threadID = event.ThreadID
			}
			if event.WorkingDir != "" && project == "" {
				project = event.WorkingDir
			}

			item := event.Item
			role := "user"
			if item.Role == "assistant" || item.Type == "agent_message" {
				role = "agent"
			}

			var text strings.Builder
			for _, c := range item.Content {
				if c.Type == "text" || c.Type == "output_text" || c.Type == "input_text" {
					if text.Len() > 0 {
						text.WriteString("\n")
					}
					text.WriteString(c.Text)
				}
			}

			content := strings.TrimSpace(text.String())
			if content == "" {
				continue
			}

			entries = append(entries, agent.Entry{
				Timestamp: ts,
				SessionID: threadID,
				Project:   project,
				Role:      role,
				Model:     currentModel,
				Display:   content,
				RawJSON:   line,
			})
		}
	}
	if hasEventMsgUser {
		for _, i := range responseItemUserIdx {
			entries[i].Role = "agent"
		}
	}
	entries = appendDiffEntries(entries)

	return entries, project
}
