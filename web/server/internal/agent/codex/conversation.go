package codex

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// maxTitleLen is the maximum character length for a title derived from the first prompt.
const maxTitleLen = 100

// readSessionTitle returns a title for the given session by finding the rollout
// file and extracting the first user prompt. Codex doesn't have a
// sessions-index.json equivalent, so we derive titles from the first user prompt only.
func readSessionTitle(sessionsDir, threadID string) string {
	path := findSessionFile(sessionsDir, threadID)
	if path == "" {
		return ""
	}

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	firstResponseItemUser := ""

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
		case "event_msg":
			var msg codexEventMsgPayload
			if err := json.Unmarshal(event.Payload, &msg); err != nil || msg.Type != "user_message" {
				continue
			}
			if text := strings.TrimSpace(msg.Message); text != "" {
				return titleFromPrompt(text)
			}

		case "response_item":
			var item codexResponseItemPayload
			if err := json.Unmarshal(event.Payload, &item); err != nil || item.Type != "message" || item.Role != "user" {
				continue
			}
			if text := extractResponseItemText(item.Content); text != "" {
				if firstResponseItemUser == "" {
					firstResponseItemUser = text
				}
			}

		case "input":
			// Legacy schema.
			if event.Role == "user" {
				text := strings.TrimSpace(event.Content)
				if text != "" {
					return titleFromPrompt(text)
				}
			}

		case "item.completed":
			// Legacy schema.
			if event.Item.Role == "user" {
				for _, c := range event.Item.Content {
					if c.Type == "text" || c.Type == "input_text" {
						text := strings.TrimSpace(c.Text)
						if text != "" {
							return titleFromPrompt(text)
						}
					}
				}
			}
		}
	}

	if firstResponseItemUser != "" {
		return titleFromPrompt(firstResponseItemUser)
	}

	return ""
}

// maxHeadingScanLines is how many lines into the prompt we look for a markdown heading.
const maxHeadingScanLines = 10

// titleFromPrompt extracts a title from a user prompt. If a first-level
// markdown heading (# Heading) appears in the first few lines, that heading
// text is used. Otherwise the first line is used. The result is truncated to
// maxTitleLen characters.
func titleFromPrompt(text string) string {
	lines := strings.SplitN(text, "\n", maxHeadingScanLines+1)
	limit := len(lines)
	if limit > maxHeadingScanLines {
		limit = maxHeadingScanLines
	}

	for _, line := range lines[:limit] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimSpace(trimmed[2:])
			if title != "" {
				return truncateTitle(title)
			}
		}
	}

	first := strings.TrimSpace(lines[0])
	return truncateTitle(first)
}

func truncateTitle(s string) string {
	if utf8.RuneCountInString(s) > maxTitleLen {
		return string([]rune(s)[:maxTitleLen]) + "..."
	}
	return s
}

// findSessionFile searches the sessions directory for a rollout file containing
// the given thread ID. It checks the filename suffix first, then falls back to
// parsing the file for a thread.started event.
func findSessionFile(sessionsDir, threadID string) string {
	if threadID == "" {
		return ""
	}

	// Walk the sessions directory looking for a matching filename.
	var match string
	filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || match != "" {
			return nil
		}
		base := info.Name()
		if !strings.HasSuffix(base, ".jsonl") {
			return nil
		}
		// Check if the thread ID appears in the filename.
		if strings.Contains(base, threadID) {
			match = path
			return filepath.SkipAll
		}
		return nil
	})
	if match != "" {
		return match
	}

	// Fallback: parse files for thread.started events.
	filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || match != "" {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}
		if tid := parseThreadID(path); tid == threadID {
			match = path
			return filepath.SkipAll
		}
		return nil
	})

	return match
}

// parseThreadID reads a rollout file and returns the thread ID from the
// thread.started event, or from the first event's thread_id field.
func parseThreadID(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

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
			if err := json.Unmarshal(event.Payload, &meta); err == nil && meta.ID != "" {
				return meta.ID
			}

		default:
			// Legacy schema.
			if event.ThreadID != "" {
				return event.ThreadID
			}
		}
	}
	return ""
}
