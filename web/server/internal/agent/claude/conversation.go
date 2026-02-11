package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
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
