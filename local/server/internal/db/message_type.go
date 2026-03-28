package db

import (
	"encoding/json"
	"strings"
)

const (
	MessageTypePrompt      = "prompt"
	MessageTypeQuestion    = "question"
	MessageTypeAnswer      = "answer"
	MessageTypeFinalAnswer = "final_answer"
	MessageTypeDiff        = "diff"
	MessageTypeLog         = "log"
)

func normalizeMessageType(messageType string) string {
	switch strings.TrimSpace(strings.ToLower(messageType)) {
	case MessageTypePrompt:
		return MessageTypePrompt
	case MessageTypeQuestion:
		return MessageTypeQuestion
	case MessageTypeAnswer:
		return MessageTypeAnswer
	case MessageTypeFinalAnswer:
		return MessageTypeFinalAnswer
	case MessageTypeDiff:
		return MessageTypeDiff
	default:
		return MessageTypeLog
	}
}

func canonicalMessageType(role, messageType, content string) string {
	if isBuildermarkRatingWorkflowContent(content) {
		return MessageTypeLog
	}

	switch normalizeMessageType(messageType) {
	case MessageTypePrompt:
		if strings.TrimSpace(role) == "user" {
			return MessageTypePrompt
		}
		return MessageTypeLog
	case MessageTypeQuestion:
		if strings.TrimSpace(role) == "agent" {
			return MessageTypeQuestion
		}
		return MessageTypeLog
	case MessageTypeAnswer:
		if strings.TrimSpace(role) == "user" {
			return MessageTypeAnswer
		}
		return MessageTypeLog
	case MessageTypeFinalAnswer:
		if strings.TrimSpace(role) == "agent" {
			return MessageTypeFinalAnswer
		}
		return MessageTypeLog
	case MessageTypeDiff:
		return MessageTypeDiff
	default:
		return inferMessageType(role, content)
	}
}

func inferMessageType(role, content string) string {
	if isBuildermarkRatingWorkflowContent(content) {
		return MessageTypeLog
	}

	if strings.TrimSpace(role) != "user" {
		return MessageTypeLog
	}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return MessageTypeLog
	}
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "$bb") {
		return MessageTypeLog
	}
	return MessageTypePrompt
}

func isBuildermarkRatingWorkflowContent(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}

	if isBuildermarkRatingCommand(trimmed) {
		return true
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "base directory for this skill:") &&
		(strings.Contains(lower, "rate-buildermark") || strings.Contains(lower, "the user wants to rate this conversation.")) {
		return true
	}

	return false
}

func isBuildermarkRatingCommand(content string) bool {
	normalized := normalizeMarkdownCommand(content)
	for _, prefix := range []string{
		"/bb",
		"/bb:rate",
		"/brate",
		"/rate-buildermark",
		"$bb",
		"$rate-buildermark",
	} {
		if hasCommandPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func normalizeMarkdownCommand(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "[") {
		return trimmed
	}

	labelEnd := strings.Index(trimmed, "](")
	if labelEnd <= 1 {
		return trimmed
	}
	linkEnd := strings.Index(trimmed[labelEnd+2:], ")")
	if linkEnd < 0 {
		return trimmed
	}

	label := trimmed[1:labelEnd]
	rest := trimmed[labelEnd+2+linkEnd+1:]
	return strings.TrimSpace(label + rest)
}

func hasCommandPrefix(content, prefix string) bool {
	return content == prefix || strings.HasPrefix(content, prefix+" ")
}

func isMetaConversationMessageRawJSON(rawJSON string) bool {
	rawJSON = strings.TrimSpace(rawJSON)
	if rawJSON == "" {
		return false
	}

	var payload struct {
		IsMeta bool `json:"isMeta"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		return false
	}
	return payload.IsMeta
}
