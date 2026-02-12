package gemini

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type geminiConversation struct {
	SessionID   string          `json:"sessionId"`
	ProjectHash string          `json:"projectHash"`
	StartTime   string          `json:"startTime"`
	LastUpdated string          `json:"lastUpdated"`
	Messages    []geminiMessage `json:"messages"`
	Directories []string        `json:"directories"`
}

type geminiMessage struct {
	ID             string           `json:"id"`
	Timestamp      string           `json:"timestamp"`
	Type           string           `json:"type"`
	Content        json.RawMessage  `json:"content"`
	DisplayContent json.RawMessage  `json:"displayContent"`
	ToolCalls      []geminiToolCall `json:"toolCalls"`
}

type geminiToolCall struct {
	Args map[string]any `json:"args"`
}

type geminiLogEntry struct {
	SessionID string `json:"sessionId"`
	Type      string `json:"type"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

func parseGeminiTimestamp(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Now().UnixMilli()
	}

	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UnixMilli()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixMilli()
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1_000_000_000_000 {
			return n
		}
		return n * 1000
	}

	return time.Now().UnixMilli()
}

func extractMessageText(msg geminiMessage) string {
	if t := extractContentText(msg.DisplayContent); t != "" {
		return t
	}
	return extractContentText(msg.Content)
}

func extractContentText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}

	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err == nil {
		parts := make([]string, 0, len(blocks))
		for _, b := range blocks {
			if text, ok := b["text"].(string); ok {
				text = strings.TrimSpace(text)
				if text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err == nil {
		if text, ok := m["text"].(string); ok {
			return strings.TrimSpace(text)
		}
	}

	return ""
}

func parseZrateDisplay(display string) (int, string) {
	display = strings.TrimSpace(display)

	if strings.HasPrefix(display, "[/zrate](") {
		if i := strings.Index(display, ")"); i >= 0 {
			display = "/zrate" + display[i+1:]
		}
	}

	if !strings.HasPrefix(display, "/zrate ") {
		if i := strings.Index(display, "/zrate "); i >= 0 {
			display = display[i:]
		}
	}
	if !strings.HasPrefix(display, "/zrate ") {
		return -1, ""
	}

	rest := strings.TrimSpace(strings.TrimPrefix(display, "/zrate"))
	if rest == "" {
		return -1, ""
	}
	parts := strings.SplitN(rest, " ", 2)
	rating, err := strconv.Atoi(parts[0])
	if err != nil || rating < 0 || rating > 5 {
		return -1, ""
	}
	note := ""
	if len(parts) > 1 {
		note = parts[1]
	}
	return rating, note
}
