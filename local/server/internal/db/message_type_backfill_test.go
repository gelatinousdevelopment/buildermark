package db

import (
	"context"
	"testing"
)

func TestBackfillMessageTypesDowngradesAgentPromptRowsToLog(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, database, "/proj/codex")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, database, "conv-codex", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	if _, err := database.Exec(
		`INSERT INTO messages (id, timestamp, project_id, conversation_id, role, message_type, model, content, raw_json)
		 VALUES
		 ('m-agent', 1000, ?, 'conv-codex', 'agent', 'prompt', '', 'internal wrapper prompt', ?),
		 ('m-user', 1001, ?, 'conv-codex', 'user', 'prompt', '', 'real prompt', ?)`,
		projectID,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"internal wrapper prompt"}]}}`,
		projectID,
		`{"type":"event_msg","payload":{"type":"user_message","message":"real prompt"}}`,
	); err != nil {
		t.Fatalf("insert messages: %v", err)
	}
	if _, err := database.Exec("UPDATE conversations SET user_prompt_count = 2 WHERE id = 'conv-codex'"); err != nil {
		t.Fatalf("seed user_prompt_count: %v", err)
	}

	if err := backfillMessageTypes(ctx, database); err != nil {
		t.Fatalf("backfillMessageTypes: %v", err)
	}

	var agentType string
	if err := database.QueryRow("SELECT message_type FROM messages WHERE id = 'm-agent'").Scan(&agentType); err != nil {
		t.Fatalf("query m-agent: %v", err)
	}
	if agentType != MessageTypeLog {
		t.Fatalf("m-agent message_type = %q, want %q", agentType, MessageTypeLog)
	}

	var promptCount int
	if err := database.QueryRow("SELECT user_prompt_count FROM conversations WHERE id = 'conv-codex'").Scan(&promptCount); err != nil {
		t.Fatalf("query user_prompt_count: %v", err)
	}
	if promptCount != 1 {
		t.Fatalf("user_prompt_count = %d, want 1", promptCount)
	}
}
