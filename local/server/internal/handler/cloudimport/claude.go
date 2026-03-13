package cloudimport

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent/claude"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// CloudProcessResult holds the output of processing cloud events or tasks.
type CloudProcessResult struct {
	Messages []db.Message
	Title    string
	Model    string
	RepoURL  string // raw git remote URL (handler normalizes)
	Cwd      string // working directory (Claude Cloud only)
}

// CloudEvent represents a single event from the Claude Cloud API.
type CloudEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	CreatedAt string `json:"created_at"`
	Model     string `json:"model"`
	Cwd       string `json:"cwd"`
	Result    string `json:"result"`
	Message   *struct {
		Role       string          `json:"role"`
		Model      string          `json:"model"`
		Content    json.RawMessage `json:"content"`
		StopReason string          `json:"stop_reason"`
	} `json:"message,omitempty"`
}

// ProcessClaudeCloudEvents converts raw Claude Cloud events into messages.
func ProcessClaudeCloudEvents(rawEvents []json.RawMessage) (CloudProcessResult, error) {
	// Parse events.
	events := make([]CloudEvent, 0, len(rawEvents))
	parseErrors := 0
	for _, raw := range rawEvents {
		var ev CloudEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			parseErrors++
			continue
		}
		events = append(events, ev)
	}

	log.Printf("[import-cloud] parsed %d/%d events (%d parse errors)", len(events), len(rawEvents), parseErrors)

	if len(events) == 0 {
		return CloudProcessResult{}, nil
	}

	// Log event type breakdown.
	typeCounts := map[string]int{}
	for _, ev := range events {
		key := ev.Type
		if ev.Subtype != "" {
			key += ":" + ev.Subtype
		}
		typeCounts[key]++
	}
	log.Printf("[import-cloud] event types: %v", typeCounts)

	// Extract metadata from init event.
	var model, cwd, repoURL string
	for _, ev := range events {
		if ev.Type == "system" && ev.Subtype == "init" {
			if ev.Model != "" {
				model = ev.Model
			}
			if ev.Cwd != "" {
				cwd = ev.Cwd
			}
			break
		}
	}
	log.Printf("[import-cloud] init metadata: model=%q cwd=%q", model, cwd)

	// Look for repo clone info in env_manager_log entries.
	for _, rawEv := range rawEvents {
		var logEv struct {
			Type    string `json:"type"`
			Subtype string `json:"subtype"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(rawEv, &logEv); err != nil {
			continue
		}
		if logEv.Type != "system" || logEv.Subtype != "env_manager_log" {
			continue
		}
		if strings.Contains(logEv.Content, "Cloning repository") {
			parts := strings.SplitN(logEv.Content, "Cloning repository ", 2)
			if len(parts) == 2 {
				candidate := strings.TrimSpace(strings.Fields(parts[1])[0])
				if candidate != "" {
					repoURL = candidate
				}
			}
		}
	}

	// Convert events to messages.
	messages := make([]db.Message, 0, len(events))
	now := time.Now().UnixMilli()

	skippedSystem, skippedNoMsg, skippedEmpty, skippedSysMsg := 0, 0, 0, 0
	roleCounts := map[string]int{}

	for i, ev := range events {
		if ev.Type == "system" {
			skippedSystem++
			continue
		}

		// "result" events are the final answer displayed to the user.
		if ev.Type == "result" && strings.TrimSpace(ev.Result) != "" {
			ts := int64(0)
			if parsed, err := time.Parse(time.RFC3339Nano, ev.CreatedAt); err == nil {
				ts = parsed.UnixMilli()
			}
			if ts == 0 {
				ts = now
			}
			roleCounts["agent"]++
			messages = append(messages, db.Message{
				Timestamp:   ts,
				Role:        "agent",
				MessageType: db.MessageTypeFinalAnswer,
				Content:     strings.TrimSpace(ev.Result),
				Model:       model,
				RawJSON:     string(rawEvents[i]),
			})
			continue
		}

		if ev.Message == nil {
			skippedNoMsg++
			continue
		}

		entry, rawJSON := ClaudeCloudToEntry(ev, rawEvents[i])

		content := claude.ContentFromConversationEntry(entry)

		// If content is empty but the event has tool_use blocks, generate a
		// summary so the message isn't skipped.
		if content == "" {
			summary := ToolUseSummary(ev.Message.Content)
			if summary != "" {
				content = summary
			}
		}

		if content == "" {
			skippedEmpty++
			continue
		}
		if claude.IsSystemMessage(content) {
			skippedSysMsg++
			continue
		}

		role := "agent"
		if entry.Type == "user" {
			role = "user"
			if strings.TrimSpace(entry.SourceToolAssistantUUID) != "" || claude.IsAssistantAuthoredConversationEntry(entry) || claude.IsSkillExpansion(content) {
				role = "agent"
			}
		}

		stopReason := strings.TrimSpace(ev.Message.StopReason)
		var messageType string
		role, messageType, content = claude.ClassifyClaudeMessage(role, content, rawJSON, stopReason)
		roleCounts[role]++

		ts := int64(0)
		if parsed, err := time.Parse(time.RFC3339Nano, ev.CreatedAt); err == nil {
			ts = parsed.UnixMilli()
		}
		if ts == 0 {
			ts = now
		}

		msgModel := strings.TrimSpace(ev.Message.Model)
		if msgModel == "" {
			msgModel = model
		}

		messages = append(messages, db.Message{
			Timestamp:   ts,
			Role:        role,
			MessageType: messageType,
			Content:     content,
			Model:       msgModel,
			RawJSON:     rawJSON,
		})
	}

	log.Printf("[import-cloud] message conversion: %d messages from %d events (skipped: system=%d noMsg=%d empty=%d sysMsg=%d) roles: %v",
		len(messages), len(events), skippedSystem, skippedNoMsg, skippedEmpty, skippedSysMsg, roleCounts)

	// Extract title from summary events or first user prompt.
	title := ExtractCloudTitle(events, messages)

	return CloudProcessResult{
		Messages: messages,
		Title:    title,
		Model:    model,
		RepoURL:  repoURL,
		Cwd:      cwd,
	}, nil
}

// ClaudeCloudToEntry translates a cloud event into a local ConversationEntry
// so the existing Claude classification pipeline works correctly.
func ClaudeCloudToEntry(ev CloudEvent, raw json.RawMessage) (claude.ConversationEntry, string) {
	entry := claude.ConversationEntry{}
	entry.Type = ev.Type
	if entry.Type == "" {
		entry.Type = ev.Message.Role
	}
	entry.Timestamp = ev.CreatedAt
	entry.Message.Role = ev.Message.Role
	entry.Message.Model = ev.Message.Model
	entry.Message.Content = ev.Message.Content
	entry.Message.StopReason = ev.Message.StopReason

	// Cloud events with type "user" whose content is a JSON array are
	// agent-authored (subagent prompts, tool results, etc). Real user messages
	// have string content. Set SourceToolAssistantUUID so the existing pipeline
	// classifies them as role "agent" instead of "user".
	if entry.Type == "user" && IsContentArray(ev.Message.Content) {
		entry.SourceToolAssistantUUID = "cloud"
	}

	return entry, string(raw)
}

// ToolUseSummary generates a short placeholder for assistant messages that
// contain only tool_use blocks.
func ToolUseSummary(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	var names []string
	for _, b := range blocks {
		typ, _ := b["type"].(string)
		if typ != "tool_use" {
			continue
		}
		name, _ := b["name"].(string)
		if name == "" {
			name = "tool"
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}

	// Deduplicate while preserving order.
	seen := make(map[string]struct{}, len(names))
	unique := make([]string, 0, len(names))
	for _, n := range names {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		unique = append(unique, n)
	}

	return "[" + strings.Join(unique, ", ") + "]"
}

// ExtractCloudTitle extracts a title from cloud events or messages.
func ExtractCloudTitle(events []CloudEvent, messages []db.Message) string {
	for _, ev := range events {
		if ev.Type == "summary" || (ev.Type == "system" && ev.Subtype == "summary") {
			if ev.Message != nil {
				text := agent.NormalizeTitleCandidate(claude.ExtractUserText(ev.Message.Content))
				if text != "" {
					return text
				}
			}
		}
	}

	for _, m := range messages {
		if m.Role != "user" {
			continue
		}
		if text := agent.NormalizeTitleCandidate(m.Content); text != "" {
			return agent.TitleFromPrompt(text)
		}
	}

	return ""
}

// IsContentArray returns true if content is a JSON array (starts with '[').
func IsContentArray(content json.RawMessage) bool {
	return len(content) > 0 && content[0] == '['
}
