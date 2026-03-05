package codex

import (
	"context"
	"strings"
	"testing"
)

func TestParseRequestUserInputQuestions(t *testing.T) {
	arguments := `{
		"questions":[
			{
				"id":"local_user",
				"header":"Filter",
				"question":"Which user filter should we use?",
				"options":[{"label":"Local User","description":"Current git user plus extra local emails"}]
			},
			{"id":"blank","question":"   "}
		]
	}`
	questions := parseRequestUserInputQuestions(arguments)
	if len(questions) != 1 {
		t.Fatalf("len(questions) = %d, want 1", len(questions))
	}
	if questions[0].ID != "local_user" {
		t.Fatalf("question id = %q, want %q", questions[0].ID, "local_user")
	}
}

func TestFormatCodexAnswersMarkdownWithCustomNote(t *testing.T) {
	questions := []codexQuestionSpec{
		{
			ID:       "local_user",
			Header:   "Filter",
			Question: "Which user filter should we use?",
			Options: []codexQuestionOption{
				{Label: "Local User", Description: "Current git user plus extra local emails"},
			},
		},
	}
	answers := map[string]any{
		"local_user": []any{"Local User", "USER_NOTE:This is a long custom answer from a thread."},
	}

	content := formatCodexAnswersMarkdown(questions, answers)
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

func TestWatcherImportsRequestUserInputAsQuestionAndAnswer(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()
	sessionsDir := t.TempDir()
	sessionPath := sessionsDir + "/session.jsonl"

	lines := []any{
		map[string]any{
			"type":      "session_meta",
			"timestamp": 1000,
			"payload": map[string]any{
				"id":    "thread-qa",
				"cwd":   "/proj/a",
				"model": "gpt-5",
			},
		},
		map[string]any{
			"type":      "response_item",
			"timestamp": 1100,
			"payload": map[string]any{
				"type":      "function_call",
				"name":      "request_user_input",
				"call_id":   "call_local_user",
				"arguments": `{"questions":[{"id":"local_user","header":"Filter","question":"Which user filter should we use?","options":[{"label":"Local User","description":"Current git user plus extra local emails"}]}]}`,
			},
		},
		map[string]any{
			"type":      "response_item",
			"timestamp": 1200,
			"payload": map[string]any{
				"type":    "function_call_output",
				"call_id": "call_local_user",
				"output":  `{"answers":{"local_user":{"answers":["Local User","USER_NOTE:This is a long custom answer from a thread."]}}}`,
			},
		},
	}
	writeJSONLObjects(t, sessionPath, lines)

	a := newAgent(database, sessionsDir, tmpDir)
	a.processSessionFile(context.Background(), sessionPath, nil)

	type gotRow struct {
		role        string
		messageType string
		content     string
	}
	rows, err := database.Query(`SELECT role, message_type, content FROM messages WHERE conversation_id = ? AND message_type IN ('question','answer') ORDER BY timestamp`, "thread-qa")
	if err != nil {
		t.Fatalf("query qa messages: %v", err)
	}
	defer rows.Close()

	var got []gotRow
	for rows.Next() {
		var row gotRow
		if err := rows.Scan(&row.role, &row.messageType, &row.content); err != nil {
			t.Fatalf("scan qa message: %v", err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate qa messages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("qa message count = %d, want 2", len(got))
	}
	if got[0].role != "agent" || got[0].messageType != "question" {
		t.Fatalf("first qa message = (%q, %q), want (agent, question)", got[0].role, got[0].messageType)
	}
	if got[1].role != "user" || got[1].messageType != "answer" {
		t.Fatalf("second qa message = (%q, %q), want (user, answer)", got[1].role, got[1].messageType)
	}
	if !strings.Contains(got[1].content, "Custom:") {
		t.Fatalf("answer content missing custom note section: %q", got[1].content)
	}
}
