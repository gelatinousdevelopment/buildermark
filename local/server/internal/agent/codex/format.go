package codex

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type codexSessionLine struct {
	Type string `json:"type"`

	// Current Codex session schema uses payload envelopes.
	Payload json.RawMessage `json:"payload"`

	// Legacy rollout schema fields.
	ThreadID   string      `json:"thread_id"`
	Role       string      `json:"role"`
	Content    string      `json:"content"`
	Item       rolloutItem `json:"item"`
	WorkingDir string      `json:"working_dir"`

	// Timestamp can be either Unix milliseconds (number) or RFC3339 string.
	Timestamp json.RawMessage `json:"timestamp"`
}

type codexSessionMetaPayload struct {
	ID        string `json:"id"`
	Cwd       string `json:"cwd"`
	Model     string `json:"model"`
	ModelSlug string `json:"model_slug"`
}

type codexTurnContextPayload struct {
	Cwd       string `json:"cwd"`
	Model     string `json:"model"`
	ModelSlug string `json:"model_slug"`
}

type codexResponseContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexResponseSummaryBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexResponseItemPayload struct {
	Type      string                      `json:"type"`
	Role      string                      `json:"role"`
	Content   []codexResponseContentBlock `json:"content"`
	Summary   []codexResponseSummaryBlock `json:"summary"`
	Model     string                      `json:"model"`
	ModelSlug string                      `json:"model_slug"`
	Name      string                      `json:"name"`
	Arguments string                      `json:"arguments"`
	CallID    string                      `json:"call_id"`
	Output    string                      `json:"output"`
}

type codexEventMsgPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Phase   string `json:"phase"`
}

func parseCodexTimestamp(raw json.RawMessage) int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}

	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	if ts, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return ts.UnixMilli()
	}
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return ts.UnixMilli()
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return ms
	}

	return 0
}

func extractResponseItemText(blocks []codexResponseContentBlock) string {
	var text strings.Builder
	for _, c := range blocks {
		if c.Type != "text" && c.Type != "input_text" && c.Type != "output_text" {
			continue
		}
		if c.Text == "" {
			continue
		}
		if text.Len() > 0 {
			text.WriteString("\n")
		}
		text.WriteString(c.Text)
	}
	return strings.TrimSpace(text.String())
}

func extractResponseItemSummaryText(blocks []codexResponseSummaryBlock) string {
	var text strings.Builder
	for _, c := range blocks {
		if c.Type != "summary_text" && c.Type != "text" {
			continue
		}
		if c.Text == "" {
			continue
		}
		if text.Len() > 0 {
			text.WriteString("\n")
		}
		text.WriteString(c.Text)
	}
	return strings.TrimSpace(text.String())
}
