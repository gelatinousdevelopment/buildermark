package cloudimport

import (
	"encoding/json"
	"testing"
)

func TestIsContentArray(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "tool_result array",
			content: `[{"type":"tool_result","tool_use_id":"abc","content":"ok"}]`,
			want:    true,
		},
		{
			name:    "text block array",
			content: `[{"type":"text","text":"hello"}]`,
			want:    true,
		},
		{
			name:    "tool_use array",
			content: `[{"type":"tool_use","name":"Bash","id":"abc"}]`,
			want:    true,
		},
		{
			name:    "plain string (real user message)",
			content: `"hello world"`,
			want:    false,
		},
		{
			name:    "empty",
			content: ``,
			want:    false,
		},
		{
			name:    "object not array",
			content: `{"type":"tool_result"}`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsContentArray(json.RawMessage(tt.content))
			if got != tt.want {
				t.Errorf("IsContentArray(%s) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestClaudeCloudToEntry_ArrayContentSetsUUID(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "tool_result array",
			content: `[{"type":"tool_result","tool_use_id":"abc","content":"file written"}]`,
		},
		{
			name:    "text block array (subagent prompt)",
			content: `[{"type":"text","text":"You are a helpful assistant"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := CloudEvent{
				Type:      "user",
				CreatedAt: "2025-01-01T00:00:00Z",
				Message: &struct {
					Role       string          `json:"role"`
					Model      string          `json:"model"`
					Content    json.RawMessage `json:"content"`
					StopReason string          `json:"stop_reason"`
				}{
					Role:    "user",
					Content: json.RawMessage(tt.content),
				},
			}
			raw := json.RawMessage(`{}`)

			entry, _ := ClaudeCloudToEntry(ev, raw)

			if entry.SourceToolAssistantUUID != "cloud" {
				t.Errorf("expected SourceToolAssistantUUID = %q, got %q", "cloud", entry.SourceToolAssistantUUID)
			}
		})
	}
}

func TestCloudEventResultParsing(t *testing.T) {
	raw := `{"type":"result","result":"Here is the summary.","created_at":"2025-01-01T00:00:00Z","subtype":"success"}`
	var ev CloudEvent
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("failed to unmarshal result event: %v", err)
	}
	if ev.Type != "result" {
		t.Errorf("expected type %q, got %q", "result", ev.Type)
	}
	if ev.Result != "Here is the summary." {
		t.Errorf("expected result %q, got %q", "Here is the summary.", ev.Result)
	}
	if ev.Message != nil {
		t.Error("expected nil message for result event")
	}
}

func TestClaudeCloudToEntry_RealUserNoUUID(t *testing.T) {
	ev := CloudEvent{
		Type:      "user",
		CreatedAt: "2025-01-01T00:00:00Z",
		Message: &struct {
			Role       string          `json:"role"`
			Model      string          `json:"model"`
			Content    json.RawMessage `json:"content"`
			StopReason string          `json:"stop_reason"`
		}{
			Role:    "user",
			Content: json.RawMessage(`"Fix the bug in main.go"`),
		},
	}
	raw := json.RawMessage(`{"type":"user","message":{"role":"user","content":"Fix the bug in main.go"}}`)

	entry, _ := ClaudeCloudToEntry(ev, raw)

	if entry.SourceToolAssistantUUID != "" {
		t.Errorf("expected empty SourceToolAssistantUUID for real user message, got %q", entry.SourceToolAssistantUUID)
	}
}
