package db

import "strings"

const (
	MessageTypePrompt   = "prompt"
	MessageTypeQuestion = "question"
	MessageTypeAnswer   = "answer"
	MessageTypeLog      = "log"
)

func normalizeMessageType(messageType string) string {
	switch strings.TrimSpace(strings.ToLower(messageType)) {
	case MessageTypePrompt:
		return MessageTypePrompt
	case MessageTypeQuestion:
		return MessageTypeQuestion
	case MessageTypeAnswer:
		return MessageTypeAnswer
	default:
		return MessageTypeLog
	}
}

func inferMessageType(role, content string) string {
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
