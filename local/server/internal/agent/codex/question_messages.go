package codex

import (
	"encoding/json"
	"strings"
)

type codexQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type codexQuestionSpec struct {
	ID       string                `json:"id"`
	Header   string                `json:"header"`
	Question string                `json:"question"`
	Options  []codexQuestionOption `json:"options"`
}

type codexAnswerPayload struct {
	Answers map[string]struct {
		Answers []string `json:"answers"`
	} `json:"answers"`
}

func parseRequestUserInputQuestions(arguments string) []codexQuestionSpec {
	var payload struct {
		Questions []codexQuestionSpec `json:"questions"`
	}
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return nil
	}
	out := make([]codexQuestionSpec, 0, len(payload.Questions))
	for _, q := range payload.Questions {
		if strings.TrimSpace(q.Question) == "" {
			continue
		}
		out = append(out, q)
	}
	return out
}

func parseRequestUserInputAnswers(output string) map[string]any {
	var payload codexAnswerPayload
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return nil
	}
	if len(payload.Answers) == 0 {
		return nil
	}
	out := make(map[string]any, len(payload.Answers))
	for id, value := range payload.Answers {
		items := make([]any, 0, len(value.Answers))
		for _, answer := range value.Answers {
			items = append(items, answer)
		}
		out[id] = items
	}
	return out
}

func formatCodexQuestionsMarkdown(questions []codexQuestionSpec) string {
	var b strings.Builder
	for i, q := range questions {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if h := strings.TrimSpace(q.Header); h != "" {
			b.WriteString("### ")
			b.WriteString(h)
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimSpace(q.Question))
		if len(q.Options) > 0 {
			b.WriteString("\n\nOptions:\n")
			for _, opt := range q.Options {
				label := strings.TrimSpace(opt.Label)
				desc := strings.TrimSpace(opt.Description)
				if label == "" && desc == "" {
					continue
				}
				b.WriteString("- ")
				if label != "" {
					b.WriteString("**")
					b.WriteString(label)
					b.WriteString("**")
				}
				if desc != "" {
					if label != "" {
						b.WriteString(": ")
					}
					b.WriteString(desc)
				}
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func formatCodexAnswersMarkdown(questions []codexQuestionSpec, answers map[string]any) string {
	var b strings.Builder
	for i, q := range questions {
		if i > 0 {
			b.WriteString("\n\n")
		}
		if h := strings.TrimSpace(q.Header); h != "" {
			b.WriteString("### ")
			b.WriteString(h)
			b.WriteString("\n")
		}
		b.WriteString("Question: ")
		b.WriteString(strings.TrimSpace(q.Question))

		selected, custom := extractCodexAnswerValues(answers[q.ID])
		optionLines := formatCodexOptionsWithSelection(q.Options, selected)
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

func extractCodexAnswerValues(v any) ([]string, []string) {
	values := flattenCodexAnswerValues(v)
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

func flattenCodexAnswerValues(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			out = append(out, flattenCodexAnswerValues(item)...)
		}
		return out
	default:
		return nil
	}
}

func formatCodexOptionsWithSelection(options []codexQuestionOption, selected []string) []string {
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
