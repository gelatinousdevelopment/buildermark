package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// conversationEntry represents a single entry in a Claude conversation JSONL file.
type conversationEntry struct {
	Type                    string `json:"type"`
	Timestamp               string `json:"timestamp"`
	SessionID               string `json:"sessionId"`
	Cwd                     string `json:"cwd"`
	Summary                 string `json:"summary"`
	SourceToolAssistantUUID string `json:"sourceToolAssistantUUID"`
	PlanContent             string `json:"planContent"`
	ToolUseResult           struct {
		Content any `json:"content"`
	} `json:"toolUseResult"`
	Message struct {
		Role    string          `json:"role"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type conversationLogEntry struct {
	Type      string
	Timestamp int64
	Role      string
	Content   string
	RawJSON   string
}

func conversationPath(home, projectPath, sessionID string) string {
	dirName := strings.ReplaceAll(projectPath, "/", "-")
	return filepath.Join(home, ".claude", "projects", dirName, sessionID+".jsonl")
}

func scanConversationFile(path string, fn func(line string, entry conversationEntry)) {
	f, err := os.Open(path)
	if err != nil {
		return
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

		fn(line, entry)
	}
}

// readFirstPrompt reads the Claude conversation JSONL file for the given session
// and returns the first substantive user prompt and its timestamp in unix millis.
// Claude Code stores full conversation transcripts at
// ~/.claude/projects/{project-dir}/{sessionId}.jsonl but history.jsonl sometimes
// omits the initial prompt (e.g. plan-mode auto-submissions). This function
// extracts that missing first prompt.
func readFirstPrompt(home, projectPath, sessionID string) (string, int64) {
	convPath := conversationPath(home, projectPath, sessionID)
	var firstText string
	var firstTS int64

	scanConversationFile(convPath, func(_ string, entry conversationEntry) {
		if firstText != "" {
			return
		}
		if entry.Type != "user" {
			return
		}
		if strings.TrimSpace(entry.SourceToolAssistantUUID) != "" {
			// Tool results are logged as "user" entries but are not user prompts.
			return
		}

		text := extractUserText(entry.Message.Content)
		if text == "" || isSystemMessage(text) {
			return
		}

		ts, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil {
			return
		}

		firstText = text
		firstTS = ts.UnixMilli()
	})

	return firstText, firstTS
}

const maxExtractedContentLen = 64 * 1024

// extractUserText extracts text from a conversation entry's content field,
// which can be either a JSON string or an array of content blocks.
func extractUserText(raw json.RawMessage) string {
	return joinTextParts(extractContentTextParts(raw))
}

func extractContentTextParts(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}

	// Try as string first (plan mode prompts use a plain string).
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []string{strings.TrimSpace(s)}
	}

	// Try as array of content blocks.
	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var out []string
		for _, b := range blocks {
			typ, _ := b["type"].(string)
			switch typ {
			case "text":
				if text, ok := b["text"].(string); ok {
					out = append(out, text)
				}
			case "tool_result":
				appendPreferredText(&out, b["content"])
			case "tool_use":
				appendToolInputText(&out, b["input"])
			}
		}
		return uniqueNonEmpty(out)
	}

	return nil
}

func appendPreferredText(out *[]string, v any) {
	switch x := v.(type) {
	case string:
		*out = append(*out, x)
	case []any:
		for _, item := range x {
			appendPreferredText(out, item)
		}
	case map[string]any:
		if content, ok := x["content"]; ok {
			appendPreferredText(out, content)
		}
		if text, ok := x["text"]; ok {
			appendPreferredText(out, text)
		}
	}
}

func appendToolInputText(out *[]string, v any) {
	switch x := v.(type) {
	case string:
		*out = append(*out, x)
	case []any:
		for _, item := range x {
			appendToolInputText(out, item)
		}
	case map[string]any:
		preferredKeys := []string{"plan", "content", "text", "prompt"}
		for _, k := range preferredKeys {
			if val, ok := x[k]; ok {
				appendToolInputText(out, val)
			}
		}
	}
}

func uniqueNonEmpty(parts []string) []string {
	if len(parts) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func joinTextParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	joined := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if joined == "" {
		return ""
	}
	if len(joined) <= maxExtractedContentLen {
		return joined
	}
	return joined[:maxExtractedContentLen] + "\n...[truncated]"
}

func contentFromConversationEntry(entry conversationEntry) string {
	parts := extractContentTextParts(entry.Message.Content)
	appendPreferredText(&parts, entry.ToolUseResult.Content)
	if s := strings.TrimSpace(entry.PlanContent); s != "" {
		parts = append(parts, s)
	}
	return joinTextParts(uniqueNonEmpty(parts))
}

func parseProjectConversationLine(line string) (historyEntry, bool) {
	var entry conversationEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return historyEntry{}, false
	}

	sessionID := strings.TrimSpace(entry.SessionID)
	project := strings.TrimSpace(entry.Cwd)
	if sessionID == "" || project == "" {
		return historyEntry{}, false
	}

	parsed, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
	if err != nil {
		return historyEntry{}, false
	}
	ts := parsed.UnixMilli()

	typ := strings.TrimSpace(entry.Type)
	if typ == "" {
		typ = strings.TrimSpace(entry.Message.Role)
	}
	if typ == "" {
		typ = "user"
	}

	display := contentFromConversationEntry(entry)
	if typ == "summary" && strings.TrimSpace(entry.Summary) != "" {
		display = strings.TrimSpace(entry.Summary)
	}
	if display == "" {
		display = fmt.Sprintf("[%s]", typ)
	}

	return historyEntry{
		Display:                 display,
		Timestamp:               ts,
		SessionID:               sessionID,
		Project:                 project,
		Type:                    typ,
		Model:                   strings.TrimSpace(entry.Message.Model),
		Summary:                 strings.TrimSpace(entry.Summary),
		SourceToolAssistantUUID: strings.TrimSpace(entry.SourceToolAssistantUUID),
		RawJSON:                 line,
	}, true
}

func readConversationLogEntries(home, projectPath, sessionID string) []conversationLogEntry {
	convPath := conversationPath(home, projectPath, sessionID)
	result := make([]conversationLogEntry, 0, 64)
	lastTS := int64(0)

	scanConversationFile(convPath, func(line string, entry conversationEntry) {
		content := contentFromConversationEntry(entry)
		if entry.Type == "summary" && strings.TrimSpace(entry.Summary) != "" {
			content = strings.TrimSpace(entry.Summary)
		}
		if content == "" {
			content = fmt.Sprintf("[%s]", strings.TrimSpace(entry.Type))
		}
		if strings.TrimSpace(content) == "" {
			content = "[entry]"
		}

		ts := int64(0)
		if parsed, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
			ts = parsed.UnixMilli()
		} else if entry.Type != "summary" {
			return
		}
		if ts == 0 && lastTS > 0 {
			ts = lastTS + 1
		}
		if ts > 0 {
			lastTS = ts
		}

		role := "agent"
		if entry.Type == "user" {
			role = "user"
			if strings.TrimSpace(entry.SourceToolAssistantUUID) != "" {
				// Claude logs tool_result as type=user; this is assistant-produced output.
				role = "agent"
			}
		}

		result = append(result, conversationLogEntry{
			Type:      entry.Type,
			Timestamp: ts,
			Role:      role,
			Content:   content,
			RawJSON:   line,
		})
	})

	return result
}

func listProjectConversationFiles(home string) []string {
	root := filepath.Join(home, ".claude", "projects")
	files := make([]string, 0, 128)

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(d.Name()) != ".jsonl" {
			return nil
		}
		files = append(files, path)
		return nil
	})

	sort.Strings(files)
	return files
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
const maxTitleLen = 1000

// readSessionTitle returns a title for the given session. Priority:
// 1. Inline summary from conversation JSONL (type="summary" entries)
// 2. Claude's sessions-index.json
// 3. First user prompt from the conversation file
func readSessionTitle(home, projectPath, sessionID string) string {
	if s := readSummaryFromConversationFile(home, projectPath, sessionID); s != "" {
		return s
	}

	if s := readSessionSummaryFromIndex(home, projectPath, sessionID); s != "" {
		return s
	}

	// Fallback: use the first user prompt from the conversation file.
	text, _ := readFirstPrompt(home, projectPath, sessionID)
	if text == "" {
		return ""
	}

	return titleFromPrompt(text)
}

// readSummaryFromConversationFile scans the conversation JSONL for
// type="summary" entries and returns the last non-empty summary found.
func readSummaryFromConversationFile(home, projectPath, sessionID string) string {
	convPath := conversationPath(home, projectPath, sessionID)
	var lastSummary string

	scanConversationFile(convPath, func(_ string, entry conversationEntry) {
		if entry.Type == "summary" {
			if s := strings.TrimSpace(entry.Summary); s != "" {
				lastSummary = s
			}
		}
	})

	return lastSummary
}

// readSessionSummaryFromIndex reads Claude's sessions-index.json and returns
// the summary for a session if available.
func readSessionSummaryFromIndex(home, projectPath, sessionID string) string {
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
	return ""
}

// titleFromPrompt extracts a title from a user prompt by taking the first
// maxTitleLen characters from the prompt (including new lines), appending an
// ellipsis when truncated.
func titleFromPrompt(text string) string {
	return truncateTitle(strings.TrimSpace(text))
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
		if strings.HasPrefix(strings.TrimSpace(text), p) {
			return true
		}
	}
	return false
}
