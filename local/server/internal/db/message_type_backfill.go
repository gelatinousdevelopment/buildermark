package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type questionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type questionSpec struct {
	ID       string           `json:"id"`
	Header   string           `json:"header"`
	Question string           `json:"question"`
	Options  []questionOption `json:"options"`
}

type codexAnswerValue struct {
	Answers []string `json:"answers"`
}

func backfillMessageTypes(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx,
		`SELECT m.id, m.conversation_id, m.role, m.message_type, m.content, m.raw_json, c.agent
		 FROM messages m
		 JOIN conversations c ON c.id = m.conversation_id
		 ORDER BY m.conversation_id, m.timestamp, m.id`,
	)
	if err != nil {
		return fmt.Errorf("query messages for backfill: %w", err)
	}
	defer rows.Close()

	type rowUpdate struct {
		id             string
		conversationID string
		role           string
		messageType    string
		content        string
	}
	var updates []rowUpdate
	conversationIDs := make(map[string]struct{})

	lastConversationID := ""
	codexQuestions := make(map[string][]questionSpec)

	for rows.Next() {
		var id, conversationID, role, currentType, content, rawJSON, agentName string
		if err := rows.Scan(&id, &conversationID, &role, &currentType, &content, &rawJSON, &agentName); err != nil {
			return fmt.Errorf("scan message row: %w", err)
		}

		if conversationID != lastConversationID {
			lastConversationID = conversationID
			codexQuestions = make(map[string][]questionSpec)
		}

		nextRole := strings.TrimSpace(role)
		nextContent := content
		nextType := inferMessageType(nextRole, nextContent)

		switch strings.TrimSpace(agentName) {
		case "claude":
			classifiedRole, classifiedType, classifiedContent := classifyClaudeMessage(rawJSON, nextRole, nextContent)
			nextRole, nextType, nextContent = classifiedRole, classifiedType, classifiedContent
		case "codex":
			classifiedRole, classifiedType, classifiedContent := classifyCodexMessage(rawJSON, nextRole, nextContent, codexQuestions)
			nextRole, nextType, nextContent = classifiedRole, classifiedType, classifiedContent
		}

		nextType = canonicalMessageType(nextRole, nextType, nextContent)
		currentType = normalizeMessageType(currentType)
		if nextRole == "" {
			nextRole = role
		}
		if strings.TrimSpace(nextContent) == "" {
			nextContent = content
		}

		if nextRole != role || nextType != currentType || nextContent != content {
			updates = append(updates, rowUpdate{
				id:             id,
				conversationID: conversationID,
				role:           nextRole,
				messageType:    nextType,
				content:        nextContent,
			})
			conversationIDs[conversationID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate message rows: %w", err)
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for backfill: %w", err)
	}
	defer tx.Rollback()

	updateStmt, err := tx.PrepareContext(ctx,
		`UPDATE messages
		 SET role = ?, message_type = ?, content = ?
		 WHERE id = ?`,
	)
	if err != nil {
		return fmt.Errorf("prepare message update: %w", err)
	}
	defer updateStmt.Close()

	for _, u := range updates {
		if _, err := updateStmt.ExecContext(ctx, u.role, u.messageType, u.content, u.id); err != nil {
			return fmt.Errorf("update message %s: %w", u.id, err)
		}
	}

	if len(conversationIDs) > 0 {
		updatePromptStmt, err := tx.PrepareContext(ctx,
			`UPDATE conversations SET user_prompt_count = (
				SELECT COUNT(*) FROM messages
				WHERE conversation_id = ? AND message_type = 'prompt'
			) WHERE id = ?`,
		)
		if err != nil {
			return fmt.Errorf("prepare prompt count update: %w", err)
		}
		defer updatePromptStmt.Close()

		ids := make([]string, 0, len(conversationIDs))
		for id := range conversationIDs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if _, err := updatePromptStmt.ExecContext(ctx, id, id); err != nil {
				return fmt.Errorf("update prompt count for %s: %w", id, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit backfill: %w", err)
	}
	return nil
}

func classifyClaudeMessage(rawJSON, role, content string) (string, string, string) {
	var line struct {
		Type    string `json:"type"`
		Message struct {
			Content []json.RawMessage `json:"content"`
		} `json:"message"`
		ToolUseResult struct {
			Questions []questionSpec `json:"questions"`
			Answers   map[string]any `json:"answers"`
		} `json:"toolUseResult"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &line); err != nil {
		return role, inferMessageType(role, content), content
	}

	for _, blockRaw := range line.Message.Content {
		var block map[string]any
		if err := json.Unmarshal(blockRaw, &block); err != nil {
			continue
		}
		blockType, _ := block["type"].(string)
		switch strings.TrimSpace(blockType) {
		case "tool_use":
			if name, _ := block["name"].(string); strings.TrimSpace(name) == "AskUserQuestion" {
				inputRaw, _ := json.Marshal(block["input"])
				questions := parseQuestionSpecsFromInput(inputRaw)
				if len(questions) > 0 {
					return "agent", MessageTypeQuestion, formatQuestionsMarkdown(questions)
				}
			}
		case "tool_result":
			if len(line.ToolUseResult.Questions) > 0 && len(line.ToolUseResult.Answers) > 0 {
				return "user", MessageTypeAnswer, formatAnswersMarkdown(line.ToolUseResult.Questions, line.ToolUseResult.Answers)
			}
		}
	}

	if len(line.ToolUseResult.Questions) > 0 && len(line.ToolUseResult.Answers) > 0 {
		return "user", MessageTypeAnswer, formatAnswersMarkdown(line.ToolUseResult.Questions, line.ToolUseResult.Answers)
	}
	return role, inferMessageType(role, content), content
}

func classifyCodexMessage(rawJSON, role, content string, questionsByCallID map[string][]questionSpec) (string, string, string) {
	var line struct {
		Type    string `json:"type"`
		Payload struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			CallID    string `json:"call_id"`
			Output    string `json:"output"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &line); err != nil {
		return role, inferMessageType(role, content), content
	}
	if line.Type != "response_item" {
		return role, inferMessageType(role, content), content
	}

	switch strings.TrimSpace(line.Payload.Type) {
	case "function_call":
		if strings.TrimSpace(line.Payload.Name) != "request_user_input" {
			return role, MessageTypeLog, content
		}
		questions := parseQuestionSpecsFromInput([]byte(line.Payload.Arguments))
		if len(questions) == 0 {
			return "agent", MessageTypeQuestion, content
		}
		if callID := strings.TrimSpace(line.Payload.CallID); callID != "" {
			questionsByCallID[callID] = questions
		}
		return "agent", MessageTypeQuestion, formatQuestionsMarkdown(questions)
	case "function_call_output":
		callID := strings.TrimSpace(line.Payload.CallID)
		answers := parseCodexAnswers(line.Payload.Output)
		questions := questionsByCallID[callID]
		if len(questions) == 0 && len(answers) == 0 {
			return role, MessageTypeLog, content
		}
		if len(questions) == 0 {
			return "user", MessageTypeAnswer, strings.TrimSpace(line.Payload.Output)
		}
		answerContent := formatAnswersMarkdown(questions, answers)
		if strings.TrimSpace(answerContent) == "" {
			answerContent = strings.TrimSpace(line.Payload.Output)
		}
		return "user", MessageTypeAnswer, answerContent
	default:
		return role, MessageTypeLog, content
	}
}

func parseQuestionSpecsFromInput(raw []byte) []questionSpec {
	var payload struct {
		Questions []questionSpec `json:"questions"`
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	out := make([]questionSpec, 0, len(payload.Questions))
	for _, q := range payload.Questions {
		if strings.TrimSpace(q.Question) == "" {
			continue
		}
		out = append(out, q)
	}
	return out
}

func parseCodexAnswers(output string) map[string]any {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	var parsed struct {
		Answers map[string]codexAnswerValue `json:"answers"`
	}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		return nil
	}
	if len(parsed.Answers) == 0 {
		return nil
	}
	out := make(map[string]any, len(parsed.Answers))
	for id, v := range parsed.Answers {
		values := make([]any, 0, len(v.Answers))
		for _, answer := range v.Answers {
			values = append(values, answer)
		}
		out[id] = values
	}
	return out
}

func formatQuestionsMarkdown(questions []questionSpec) string {
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

func formatAnswersMarkdown(questions []questionSpec, answers map[string]any) string {
	if len(questions) == 0 {
		return ""
	}
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
		selected, custom := extractAnswerValues(answers[q.ID])
		optionLines := formatOptionsWithSelection(q.Options, selected)
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

func extractAnswerValues(v any) ([]string, []string) {
	values := flattenAnswerValues(v)
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

func flattenAnswerValues(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			out = append(out, flattenAnswerValues(item)...)
		}
		return out
	default:
		return nil
	}
}

func formatOptionsWithSelection(options []questionOption, selected []string) []string {
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
