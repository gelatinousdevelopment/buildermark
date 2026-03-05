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

func canonicalMessageType(role, messageType, content string) string {
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
	default:
		return inferMessageType(role, content)
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
