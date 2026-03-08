package claude

import (
	"encoding/json"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

type claudeQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type claudeQuestionSpec struct {
	ID       string                 `json:"id"`
	Header   string                 `json:"header"`
	Question string                 `json:"question"`
	Options  []claudeQuestionOption `json:"options"`
}

// ClassifyClaudeMessage classifies a message into role, type, and content.
func ClassifyClaudeMessage(role, content, rawJSON, stopReason string) (string, string, string) {
	if questions, ok := extractAskUserQuestionsFromRaw(rawJSON); ok {
		return "agent", "question", formatClaudeQuestionsMarkdown(questions)
	}
	if questions, answers, ok := extractAskUserAnswersFromRaw(rawJSON); ok {
		return "user", "answer", formatClaudeAnswersMarkdown(questions, answers)
	}
	if role == "agent" && strings.TrimSpace(stopReason) == "end_turn" && strings.TrimSpace(content) != "" {
		return role, db.MessageTypeFinalAnswer, content
	}
	return role, inferClaudeMessageType(role, content), content
}

func inferClaudeMessageType(role, content string) string {
	if strings.TrimSpace(role) != "user" {
		return "log"
	}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "log"
	}
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "$bb") {
		return "log"
	}
	return "prompt"
}

func extractAskUserQuestionsFromRaw(rawJSON string) ([]claudeQuestionSpec, bool) {
	var entry conversationEntry
	if err := json.Unmarshal([]byte(rawJSON), &entry); err != nil {
		return nil, false
	}

	var blocks []map[string]any
	if err := json.Unmarshal(entry.Message.Content, &blocks); err != nil {
		return nil, false
	}
	for _, block := range blocks {
		if strings.TrimSpace(asString(block["type"])) != "tool_use" {
			continue
		}
		if strings.TrimSpace(asString(block["name"])) != "AskUserQuestion" {
			continue
		}
		input, _ := block["input"].(map[string]any)
		questions := parseClaudeQuestionsFromAny(input["questions"])
		if len(questions) == 0 {
			return nil, false
		}
		return questions, true
	}
	return nil, false
}

func extractAskUserAnswersFromRaw(rawJSON string) ([]claudeQuestionSpec, map[string]any, bool) {
	var entry conversationEntry
	if err := json.Unmarshal([]byte(rawJSON), &entry); err != nil {
		return nil, nil, false
	}
	if len(entry.ToolUseResult.Questions) == 0 || len(entry.ToolUseResult.Answers) == 0 {
		return nil, nil, false
	}
	return entry.ToolUseResult.Questions, entry.ToolUseResult.Answers, true
}

func parseClaudeQuestionsFromAny(v any) []claudeQuestionSpec {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]claudeQuestionSpec, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		q := claudeQuestionSpec{
			ID:       strings.TrimSpace(asString(m["id"])),
			Header:   strings.TrimSpace(asString(m["header"])),
			Question: strings.TrimSpace(asString(m["question"])),
			Options:  parseClaudeOptionsFromAny(m["options"]),
		}
		if q.Question == "" {
			continue
		}
		out = append(out, q)
	}
	return out
}

func parseClaudeOptionsFromAny(v any) []claudeQuestionOption {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]claudeQuestionOption, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		opt := claudeQuestionOption{
			Label:       strings.TrimSpace(asString(m["label"])),
			Description: strings.TrimSpace(asString(m["description"])),
		}
		if opt.Label == "" && opt.Description == "" {
			continue
		}
		out = append(out, opt)
	}
	return out
}

func formatClaudeQuestionsMarkdown(questions []claudeQuestionSpec) string {
	var b strings.Builder
	for i, q := range questions {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if q.Header != "" {
			b.WriteString("### ")
			b.WriteString(q.Header)
			b.WriteString("\n")
		}
		b.WriteString(q.Question)
		if len(q.Options) > 0 {
			b.WriteString("\n\nOptions:\n")
			for _, opt := range q.Options {
				b.WriteString("- ")
				if opt.Label != "" {
					b.WriteString("**")
					b.WriteString(opt.Label)
					b.WriteString("**")
				}
				if opt.Description != "" {
					if opt.Label != "" {
						b.WriteString(": ")
					}
					b.WriteString(opt.Description)
				}
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func formatClaudeAnswersMarkdown(questions []claudeQuestionSpec, answers map[string]any) string {
	var b strings.Builder
	for i, q := range questions {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if q.Header != "" {
			b.WriteString("### ")
			b.WriteString(q.Header)
			b.WriteString("\n")
		}
		b.WriteString("Question: ")
		b.WriteString(q.Question)

		selected, custom := extractClaudeAnswerValues(answers[q.ID])
		optionLines := formatClaudeOptionsWithSelection(q.Options, selected)
		if len(optionLines) > 0 {
			b.WriteString("\n\n")
			for _, line := range optionLines {
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		if len(custom) > 0 {
			b.WriteString("\nCustom:\n")
			for _, value := range custom {
				b.WriteString("- ")
				b.WriteString(value)
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func extractClaudeAnswerValues(v any) ([]string, []string) {
	values := flattenClaudeAnswerValues(v)
	selected := make([]string, 0, len(values))
	custom := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if customValue, ok := stripUserNotePrefix(value); ok {
			if customValue != "" {
				custom = append(custom, customValue)
			}
			continue
		}
		selected = append(selected, value)
	}
	return selected, custom
}

func flattenClaudeAnswerValues(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			out = append(out, flattenClaudeAnswerValues(item)...)
		}
		return out
	default:
		return nil
	}
}

func formatClaudeOptionsWithSelection(options []claudeQuestionOption, selected []string) []string {
	selectedByKey := make(map[string]struct{}, len(selected))
	matchedByKey := make(map[string]struct{}, len(selected))
	selectedInOrder := make([]string, 0, len(selected))
	for _, value := range selected {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		selectedByKey[key] = struct{}{}
		selectedInOrder = append(selectedInOrder, trimmed)
	}

	lines := make([]string, 0, len(options)+len(selectedInOrder))
	for _, opt := range options {
		label := strings.TrimSpace(opt.Label)
		desc := strings.TrimSpace(opt.Description)
		if label == "" && desc == "" {
			continue
		}
		_, isSelected := selectedByKey[strings.ToLower(label)]
		if isSelected && label != "" {
			matchedByKey[strings.ToLower(label)] = struct{}{}
			line := "- **" + "\u2713 " + label + "**"
			if desc != "" {
				line += ": " + desc
			}
			lines = append(lines, line)
			continue
		}
		line := "- "
		if label != "" {
			line += "**" + label + "**"
		}
		if desc != "" {
			if label != "" {
				line += ": "
			}
			line += desc
		}
		lines = append(lines, line)
	}

	for _, value := range selectedInOrder {
		key := strings.ToLower(value)
		if _, ok := matchedByKey[key]; ok {
			continue
		}
		lines = append(lines, "- **"+"\u2713 "+value+"**")
	}

	return lines
}

func stripUserNotePrefix(value string) (string, bool) {
	const prefix = "user_note:"
	if len(value) < len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(value[len(prefix):]), true
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
