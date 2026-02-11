package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// conversationEntry represents a single entry in a Claude conversation JSONL file.
type conversationEntry struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// readFirstPrompt reads the Claude conversation JSONL file for the given session
// and returns the first substantive user prompt and its timestamp in unix millis.
// Claude Code stores full conversation transcripts at
// ~/.claude/projects/{project-dir}/{sessionId}.jsonl but history.jsonl sometimes
// omits the initial prompt (e.g. plan-mode auto-submissions). This function
// extracts that missing first prompt.
func readFirstPrompt(home, projectPath, sessionID string) (string, int64) {
	dirName := strings.ReplaceAll(projectPath, "/", "-")
	convPath := filepath.Join(home, ".claude", "projects", dirName, sessionID+".jsonl")

	f, err := os.Open(convPath)
	if err != nil {
		return "", 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry conversationEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "user" {
			continue
		}

		text := extractUserText(entry.Message.Content)
		if text == "" || isSystemMessage(text) {
			continue
		}

		ts, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil {
			continue
		}

		return text, ts.UnixMilli()
	}

	return "", 0
}

// extractUserText extracts text from a conversation entry's content field,
// which can be either a JSON string or an array of content blocks.
func extractUserText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try as string first (plan mode prompts use a plain string).
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}

	// Try as array of content blocks.
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		for _, b := range blocks {
			if b.Type == "text" {
				text := strings.TrimSpace(b.Text)
				if text != "" {
					return text
				}
			}
		}
	}

	return ""
}

// sessionsIndex represents the top-level structure of Claude's sessions-index.json.
type sessionsIndex struct {
	Entries []sessionsIndexEntry `json:"entries"`
}

// sessionsIndexEntry represents a single entry in Claude's sessions-index.json.
type sessionsIndexEntry struct {
	SessionID string `json:"sessionId"`
	Summary   string `json:"summary"`
}

// maxTitleLen is the maximum character length for a title derived from the first prompt.
const maxTitleLen = 100

// readSessionTitle returns a title for the given session. It first checks
// Claude's sessions-index.json for a summary. If the session is not indexed,
// it falls back to extracting the first user prompt from the conversation
// .jsonl file and truncating it.
func readSessionTitle(home, projectPath, sessionID string) string {
	dirName := strings.ReplaceAll(projectPath, "/", "-")

	// Try sessions-index.json first.
	indexPath := filepath.Join(home, ".claude", "projects", dirName, "sessions-index.json")
	if data, err := os.ReadFile(indexPath); err == nil {
		var idx sessionsIndex
		if err := json.Unmarshal(data, &idx); err == nil {
			for _, e := range idx.Entries {
				if e.SessionID == sessionID {
					if s := strings.TrimSpace(e.Summary); s != "" {
						return s
					}
				}
			}
		}
	}

	// Fallback: use the first user prompt from the conversation file.
	text, _ := readFirstPrompt(home, projectPath, sessionID)
	if text == "" {
		return ""
	}

	return titleFromPrompt(text)
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

	// Look for a first-level markdown heading in the first few lines.
	for _, line := range lines[:limit] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimSpace(trimmed[2:])
			if title != "" {
				return truncateTitle(title)
			}
		}
	}

	// No heading found; use the first non-empty line.
	first := strings.TrimSpace(lines[0])
	return truncateTitle(first)
}

func truncateTitle(s string) string {
	if utf8.RuneCountInString(s) > maxTitleLen {
		return string([]rune(s)[:maxTitleLen]) + "..."
	}
	return s
}

// isSystemMessage returns true for system/meta messages that should be skipped
// when looking for the first substantive user prompt.
func isSystemMessage(text string) bool {
	if text == "[]" {
		return true
	}
	skipPrefixes := []string{
		"<local-command",
		"<command-name>",
		"<system-reminder>",
		"<user-prompt-submit-hook>",
		"[Request interrupted",
	}
	for _, p := range skipPrefixes {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}
