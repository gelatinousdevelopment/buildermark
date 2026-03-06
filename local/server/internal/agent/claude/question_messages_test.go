package claude

import (
	"strings"
	"testing"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func TestClassifyClaudeMessageQuestion(t *testing.T) {
	raw := `{
		"type":"assistant",
		"timestamp":"2026-03-05T10:00:00.000Z",
		"message":{
			"role":"assistant",
			"content":[
				{
					"type":"tool_use",
					"name":"AskUserQuestion",
					"input":{
						"questions":[
							{
								"id":"local_user",
								"header":"Filter",
								"question":"Which user filter should we use?",
								"options":[{"label":"Local User","description":"Current git user plus extra local emails"}]
							}
						]
					}
				}
			]
		}
	}`

	role, messageType, content := classifyClaudeMessage("agent", "", raw, "")
	if role != "agent" {
		t.Fatalf("role = %q, want %q", role, "agent")
	}
	if messageType != "question" {
		t.Fatalf("messageType = %q, want %q", messageType, "question")
	}
	if !strings.Contains(content, "Which user filter should we use?") {
		t.Fatalf("question markdown missing question text: %q", content)
	}
	if !strings.Contains(content, "Local User") {
		t.Fatalf("question markdown missing option label: %q", content)
	}
}

func TestClassifyClaudeMessageAnswerWithCustomNote(t *testing.T) {
	raw := `{
		"type":"tool_result",
		"timestamp":"2026-03-05T10:01:00.000Z",
		"toolUseResult":{
			"questions":[
				{
					"id":"local_user",
					"header":"Filter",
					"question":"Which user filter should we use?",
					"options":[{"label":"Local User","description":"Current git user plus extra local emails"}]
				}
			],
			"answers":{
				"local_user":["Local User","USER_NOTE:This is a long custom answer from the user."]
			}
		}
	}`

	role, messageType, content := classifyClaudeMessage("user", "", raw, "")
	if role != "user" {
		t.Fatalf("role = %q, want %q", role, "user")
	}
	if messageType != "answer" {
		t.Fatalf("messageType = %q, want %q", messageType, "answer")
	}
	if !strings.Contains(content, "Question: Which user filter should we use?") {
		t.Fatalf("answer markdown missing question prefix: %q", content)
	}
	if !strings.Contains(content, "✓ Local User") {
		t.Fatalf("answer markdown missing selected checkmark: %q", content)
	}
	if !strings.Contains(content, "Custom:") || !strings.Contains(content, "long custom answer") {
		t.Fatalf("answer markdown missing custom note: %q", content)
	}
}

func TestClassifyClaudeMessageFinalAnswer(t *testing.T) {
	role, messageType, content := classifyClaudeMessage("agent", "Here is the result.", "{}", "end_turn")
	if role != "agent" {
		t.Fatalf("role = %q, want %q", role, "agent")
	}
	if messageType != db.MessageTypeFinalAnswer {
		t.Fatalf("messageType = %q, want %q", messageType, db.MessageTypeFinalAnswer)
	}
	if content != "Here is the result." {
		t.Fatalf("content = %q, want %q", content, "Here is the result.")
	}
}

func TestClassifyClaudeMessageEndTurnEmptyContent(t *testing.T) {
	// end_turn with empty content should NOT be final_answer
	_, messageType, _ := classifyClaudeMessage("agent", "", "{}", "end_turn")
	if messageType == db.MessageTypeFinalAnswer {
		t.Fatalf("empty content with end_turn should not be final_answer")
	}
}

func TestClassifyClaudeMessageToolUseStopReason(t *testing.T) {
	// tool_use stop_reason should NOT be final_answer
	_, messageType, _ := classifyClaudeMessage("agent", "some content", "{}", "tool_use")
	if messageType == db.MessageTypeFinalAnswer {
		t.Fatalf("tool_use stop_reason should not be final_answer")
	}
}
