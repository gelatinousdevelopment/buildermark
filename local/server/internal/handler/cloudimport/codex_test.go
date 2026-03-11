package cloudimport

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// Minimal sample data based on the real codex_sample.json structure.
const codexSampleSubset = `{
	"current_turn_id": "task_e_abc~assttrn_e_def",
	"turn_mapping": {
		"task_e_abc~assttrn_e_def": {
			"id": "task_e_abc~assttrn_e_def",
			"parent": "task_e_abc~usertrn_e_ghi",
			"turn": {
				"created_at": 1773064177.615955,
				"role": "assistant",
				"type": "assistant",
				"model_version": "gpt-5.3-codex-1p-codexswic-ev3",
				"input_items": [],
				"output_items": [
					{
						"content": [
							{"content_type": "text", "text": "### Summary\nAdded browser extension section."},
							{"content_type": "repo_file_citation", "path": "page.svelte"},
							{"content_type": "text", "text": "\nMore details here."}
						],
						"role": "assistant",
						"type": "message"
					},
					{
						"output_diff": {
							"diff": "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1,1 +1,2 @@\n old\n+new",
							"type": "output_diff"
						},
						"type": "pr"
					}
				],
				"environment": {
					"repo_map": {
						"github-123": {
							"git_url": "git://github.com/gelatinousdevelopment/buildermark.git",
							"name": "buildermark"
						}
					}
				}
			}
		},
		"task_e_abc~usertrn_e_ghi": {
			"id": "task_e_abc~usertrn_e_ghi",
			"parent": null,
			"turn": {
				"created_at": 1773064176.66284,
				"role": "user",
				"type": "user",
				"input_items": [
					{
						"content": [
							{"content_type": "text", "text": "Add browser extension to /plugins route"}
						],
						"role": "user",
						"type": "message"
					}
				]
			}
		}
	}
}`

func TestProcessCodexTask_SampleData(t *testing.T) {
	result, err := ProcessCodexTask(json.RawMessage(codexSampleSubset))
	if err != nil {
		t.Fatalf("ProcessCodexTask error: %v", err)
	}

	// Expect: 1 user prompt + 1 assistant message + 1 diff = 3 messages
	if len(result.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result.Messages))
	}

	// First message should be user prompt (sorted by created_at).
	if result.Messages[0].Role != "user" {
		t.Errorf("first message role = %q, want user", result.Messages[0].Role)
	}
	if result.Messages[0].MessageType != db.MessageTypePrompt {
		t.Errorf("first message type = %q, want %q", result.Messages[0].MessageType, db.MessageTypePrompt)
	}

	// Second message should be assistant final answer.
	if result.Messages[1].Role != "agent" {
		t.Errorf("second message role = %q, want agent", result.Messages[1].Role)
	}
	if result.Messages[1].MessageType != db.MessageTypeFinalAnswer {
		t.Errorf("second message type = %q, want %q", result.Messages[1].MessageType, db.MessageTypeFinalAnswer)
	}

	// Third message should be the diff.
	if result.Messages[2].MessageType != db.MessageTypeDiff {
		t.Errorf("third message type = %q, want %q", result.Messages[2].MessageType, db.MessageTypeDiff)
	}

	if result.Model != "gpt-5.3-codex-1p-codexswic-ev3" {
		t.Errorf("model = %q, want gpt-5.3-codex-1p-codexswic-ev3", result.Model)
	}

	if !strings.Contains(result.RepoURL, "github.com/gelatinousdevelopment/buildermark") {
		t.Errorf("RepoURL = %q, want to contain github.com/gelatinousdevelopment/buildermark", result.RepoURL)
	}
}

func TestProcessCodexTask_UserMessage(t *testing.T) {
	data := `{
		"current_turn_id": "t~u1",
		"turn_mapping": {
			"t~u1": {
				"id": "t~u1",
				"parent": null,
				"turn": {
					"created_at": 1700000000.5,
					"role": "user",
					"type": "user",
					"input_items": [
						{
							"content": [{"content_type": "text", "text": "Hello world"}],
							"role": "user",
							"type": "message"
						}
					]
				}
			}
		}
	}`

	result, err := ProcessCodexTask(json.RawMessage(data))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}

	m := result.Messages[0]
	if m.Role != "user" {
		t.Errorf("role = %q, want user", m.Role)
	}
	if m.MessageType != db.MessageTypePrompt {
		t.Errorf("type = %q, want %q", m.MessageType, db.MessageTypePrompt)
	}
	if m.Content != "Hello world" {
		t.Errorf("content = %q, want %q", m.Content, "Hello world")
	}
	// created_at 1700000000.5 * 1000 = 1700000000500
	if m.Timestamp != 1700000000500 {
		t.Errorf("timestamp = %d, want 1700000000500", m.Timestamp)
	}
}

func TestProcessCodexTask_AssistantMessage(t *testing.T) {
	data := `{
		"current_turn_id": "t~a1",
		"turn_mapping": {
			"t~a1": {
				"id": "t~a1",
				"parent": null,
				"turn": {
					"created_at": 1700000001.0,
					"role": "assistant",
					"type": "assistant",
					"model_version": "gpt-5",
					"output_items": [
						{
							"content": [
								{"content_type": "text", "text": "Part one. "},
								{"content_type": "text", "text": "Part two."}
							],
							"role": "assistant",
							"type": "message"
						}
					]
				}
			}
		}
	}`

	result, err := ProcessCodexTask(json.RawMessage(data))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}

	m := result.Messages[0]
	if m.Role != "agent" {
		t.Errorf("role = %q, want agent", m.Role)
	}
	if m.MessageType != db.MessageTypeFinalAnswer {
		t.Errorf("type = %q, want %q", m.MessageType, db.MessageTypeFinalAnswer)
	}
	if m.Content != "Part one. Part two." {
		t.Errorf("content = %q, want %q", m.Content, "Part one. Part two.")
	}
	if m.Model != "gpt-5" {
		t.Errorf("model = %q, want gpt-5", m.Model)
	}
}

func TestProcessCodexTask_SkipsNonTextContent(t *testing.T) {
	data := `{
		"current_turn_id": "t~a1",
		"turn_mapping": {
			"t~a1": {
				"id": "t~a1",
				"parent": null,
				"turn": {
					"created_at": 1700000001.0,
					"role": "assistant",
					"type": "assistant",
					"output_items": [
						{
							"content": [
								{"content_type": "text", "text": "Summary here."},
								{"content_type": "repo_file_citation", "path": "file.go"},
								{"content_type": "image_asset_pointer_citation", "asset_pointer": "abc"}
							],
							"role": "assistant",
							"type": "message"
						}
					]
				}
			}
		}
	}`

	result, err := ProcessCodexTask(json.RawMessage(data))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	if result.Messages[0].Content != "Summary here." {
		t.Errorf("content = %q, want only text", result.Messages[0].Content)
	}
}

func TestProcessCodexTask_PRDiffExtraction(t *testing.T) {
	data := `{
		"current_turn_id": "t~a1",
		"turn_mapping": {
			"t~a1": {
				"id": "t~a1",
				"parent": null,
				"turn": {
					"created_at": 1700000001.0,
					"role": "assistant",
					"type": "assistant",
					"output_items": [
						{
							"output_diff": {
								"diff": "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1,1 +1,2 @@\n old\n+new",
								"type": "output_diff"
							},
							"type": "pr"
						}
					]
				}
			}
		}
	}`

	result, err := ProcessCodexTask(json.RawMessage(data))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}

	m := result.Messages[0]
	if m.Role != "agent" {
		t.Errorf("role = %q, want agent", m.Role)
	}
	if m.MessageType != db.MessageTypeDiff {
		t.Errorf("type = %q, want %q", m.MessageType, db.MessageTypeDiff)
	}
	if !strings.Contains(m.Content, "```diff") {
		t.Errorf("content should contain ```diff fence, got %q", m.Content)
	}
	if m.RawJSON != agent.DerivedDiffRawJSON {
		t.Errorf("raw_json = %q, want %q", m.RawJSON, agent.DerivedDiffRawJSON)
	}
}

func TestProcessCodexTask_TurnOrdering(t *testing.T) {
	data := `{
		"current_turn_id": "t~a1",
		"turn_mapping": {
			"t~a1": {
				"id": "t~a1",
				"parent": "t~u1",
				"turn": {
					"created_at": 2000000000.0,
					"role": "assistant",
					"type": "assistant",
					"output_items": [
						{
							"content": [{"content_type": "text", "text": "Response"}],
							"role": "assistant",
							"type": "message"
						}
					]
				}
			},
			"t~u1": {
				"id": "t~u1",
				"parent": null,
				"turn": {
					"created_at": 1000000000.0,
					"role": "user",
					"type": "user",
					"input_items": [
						{
							"content": [{"content_type": "text", "text": "Hello"}],
							"role": "user",
							"type": "message"
						}
					]
				}
			}
		}
	}`

	result, err := ProcessCodexTask(json.RawMessage(data))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(result.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.Messages))
	}

	// Despite map iteration order, user message should come first.
	if result.Messages[0].Role != "user" {
		t.Errorf("first message role = %q, want user", result.Messages[0].Role)
	}
	if result.Messages[1].Role != "agent" {
		t.Errorf("second message role = %q, want agent", result.Messages[1].Role)
	}
	if result.Messages[0].Timestamp >= result.Messages[1].Timestamp {
		t.Errorf("timestamps not in order: %d >= %d", result.Messages[0].Timestamp, result.Messages[1].Timestamp)
	}
}

func TestProcessCodexTask_ModelExtraction(t *testing.T) {
	data := `{
		"current_turn_id": "t~a1",
		"turn_mapping": {
			"t~a1": {
				"id": "t~a1",
				"parent": null,
				"turn": {
					"created_at": 1700000001.0,
					"role": "assistant",
					"type": "assistant",
					"model_version": "gpt-5.3-custom",
					"output_items": [
						{
							"content": [{"content_type": "text", "text": "Done"}],
							"role": "assistant",
							"type": "message"
						}
					]
				}
			}
		}
	}`

	result, err := ProcessCodexTask(json.RawMessage(data))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if result.Model != "gpt-5.3-custom" {
		t.Errorf("model = %q, want gpt-5.3-custom", result.Model)
	}
}

func TestProcessCodexTask_RepoURLExtraction(t *testing.T) {
	data := `{
		"current_turn_id": "t~u1",
		"turn_mapping": {
			"t~u1": {
				"id": "t~u1",
				"parent": null,
				"turn": {
					"created_at": 1700000001.0,
					"role": "user",
					"type": "user",
					"input_items": [
						{
							"content": [{"content_type": "text", "text": "Hi"}],
							"role": "user",
							"type": "message"
						}
					],
					"environment": {
						"repo_map": {
							"repo-1": {
								"git_url": "git://github.com/owner/myrepo.git",
								"name": "myrepo"
							}
						}
					}
				}
			}
		}
	}`

	result, err := ProcessCodexTask(json.RawMessage(data))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if result.RepoURL != "git://github.com/owner/myrepo.git" {
		t.Errorf("RepoURL = %q, want git://github.com/owner/myrepo.git", result.RepoURL)
	}
}

func TestProcessCodexTask_EmptyTurnMapping(t *testing.T) {
	data := `{"current_turn_id": "t~u1", "turn_mapping": {}}`

	_, err := ProcessCodexTask(json.RawMessage(data))
	if err == nil {
		t.Fatal("expected error for empty turn_mapping")
	}
}

func TestProcessCodexTask_MultipleContentBlocks(t *testing.T) {
	data := `{
		"current_turn_id": "t~u1",
		"turn_mapping": {
			"t~u1": {
				"id": "t~u1",
				"parent": null,
				"turn": {
					"created_at": 1700000001.0,
					"role": "user",
					"type": "user",
					"input_items": [
						{
							"content": [
								{"content_type": "text", "text": "First part. "},
								{"content_type": "text", "text": "Second part."}
							],
							"role": "user",
							"type": "message"
						}
					]
				}
			}
		}
	}`

	result, err := ProcessCodexTask(json.RawMessage(data))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	if result.Messages[0].Content != "First part. Second part." {
		t.Errorf("content = %q, want concatenated text", result.Messages[0].Content)
	}
}

// TestProcessCodexTask_TurnsFormat verifies processing works with the /turns endpoint
// response format (no current_turn_id, children in wrapper, model_version deep in turn).
func TestProcessCodexTask_TurnsFormat(t *testing.T) {
	data := `{
		"turn_mapping": {
			"task_e_abc~usertrn_e_ghi": {
				"id": "task_e_abc~usertrn_e_ghi",
				"turn": {
					"type": "user",
					"id": "task_e_abc~usertrn_e_ghi",
					"role": "user",
					"input_items": [
						{
							"type": "message",
							"role": "user",
							"content": [
								{"content_type": "text", "text": "Fix the bug"}
							]
						}
					],
					"created_at": 1700000000.0
				},
				"children": ["task_e_abc~assttrn_e_def"],
				"parent": null
			},
			"task_e_abc~assttrn_e_def": {
				"id": "task_e_abc~assttrn_e_def",
				"turn": {
					"type": "assistant",
					"id": "task_e_abc~assttrn_e_def",
					"role": "assistant",
					"output_items": [
						{
							"type": "message",
							"role": "assistant",
							"content": [
								{"content_type": "text", "text": "### Summary\nFixed the bug."},
								{"content_type": "repo_file_citation", "path": "main.go"}
							]
						},
						{
							"type": "partial_repo_snapshot",
							"files": []
						},
						{
							"type": "pr",
							"pr_title": "Fix the bug",
							"output_diff": {
								"type": "output_diff",
								"repo_id": "owner/repo",
								"diff": "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1,2 @@\n old\n+new",
								"external_storage_diff": {"file_id": "file_abc", "ttl": null}
							}
						}
					],
					"turn_status": "completed",
					"model_version": "gpt-5.3-codex",
					"created_at": 1700000001.0,
					"environment": {
						"repo_map": {
							"github-123": {
								"git_url": "git://github.com/owner/repo.git",
								"name": "repo"
							}
						}
					}
				},
				"children": [],
				"parent": "task_e_abc~usertrn_e_ghi"
			}
		},
		"current_turn_id": "task_e_abc~assttrn_e_def"
	}`

	result, err := ProcessCodexTask(json.RawMessage(data))
	if err != nil {
		t.Fatalf("ProcessCodexTask error: %v", err)
	}

	// Expect: user prompt + assistant message + diff = 3
	if len(result.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result.Messages))
	}

	if result.Messages[0].Role != "user" || result.Messages[0].MessageType != db.MessageTypePrompt {
		t.Errorf("first message: role=%q type=%q, want user/prompt", result.Messages[0].Role, result.Messages[0].MessageType)
	}
	if result.Messages[1].Role != "agent" || result.Messages[1].MessageType != db.MessageTypeFinalAnswer {
		t.Errorf("second message: role=%q type=%q, want agent/final_answer", result.Messages[1].Role, result.Messages[1].MessageType)
	}
	if result.Messages[2].MessageType != db.MessageTypeDiff || !strings.Contains(result.Messages[2].Content, "```diff") {
		t.Errorf("third message: type=%q content does not contain diff fence", result.Messages[2].MessageType)
	}
	if result.Messages[2].RawJSON != agent.DerivedDiffRawJSON {
		t.Errorf("diff message raw_json = %q, want %q", result.Messages[2].RawJSON, agent.DerivedDiffRawJSON)
	}

	if result.Model != "gpt-5.3-codex" {
		t.Errorf("model = %q, want gpt-5.3-codex", result.Model)
	}
	if !strings.Contains(result.RepoURL, "github.com/owner/repo") {
		t.Errorf("RepoURL = %q, want to contain github.com/owner/repo", result.RepoURL)
	}
}

func TestProcessCodexTask_UserTurnRequiresBothRoleAndType(t *testing.T) {
	// A turn with role="user" but type!="user" should not be treated as a user turn.
	data := `{
		"current_turn_id": "t~u1",
		"turn_mapping": {
			"t~u1": {
				"id": "t~u1",
				"parent": null,
				"turn": {
					"created_at": 1700000001.0,
					"role": "user",
					"type": "system",
					"input_items": [
						{
							"content": [{"content_type": "text", "text": "System message"}],
							"role": "user",
							"type": "message"
						}
					]
				}
			}
		}
	}`

	result, err := ProcessCodexTask(json.RawMessage(data))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// Should produce 0 messages since it's not a user turn and has no output_items.
	if len(result.Messages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(result.Messages))
	}
}
