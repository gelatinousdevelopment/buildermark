package cloudimport

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// CodexTask represents the top-level structure of a Codex Cloud task response.
type CodexTask struct {
	CurrentTurnID string                      `json:"current_turn_id"`
	TurnMapping   map[string]CodexTurnWrapper `json:"turn_mapping"`
}

// CodexTurnWrapper wraps a turn with its tree structure.
type CodexTurnWrapper struct {
	ID     string    `json:"id"`
	Parent string    `json:"parent"`
	Turn   CodexTurn `json:"turn"`
}

// CodexTurn represents a single turn in a Codex conversation.
type CodexTurn struct {
	CreatedAt    float64           `json:"created_at"`
	Role         string            `json:"role"`
	Type         string            `json:"type"`
	ModelVersion string            `json:"model_version"`
	InputItems   []CodexInputItem  `json:"input_items"`
	OutputItems  []json.RawMessage `json:"output_items"`
	Environment  *CodexEnvironment `json:"environment"`
}

// CodexInputItem represents an input item in a user turn.
type CodexInputItem struct {
	Type    string             `json:"type"`
	Role    string             `json:"role"`
	Content []CodexContentBlock `json:"content"`
}

// CodexContentBlock represents a content block within an input/output item.
type CodexContentBlock struct {
	ContentType string `json:"content_type"`
	Text        string `json:"text"`
}

// CodexOutputItem represents an output item (message or PR).
type CodexOutputItem struct {
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Content    []CodexContentBlock `json:"content"`
	OutputDiff *CodexOutputDiff   `json:"output_diff"`
}

// CodexOutputDiff holds diff data from a PR output item.
type CodexOutputDiff struct {
	Diff string `json:"diff"`
	Type string `json:"type"`
}

// CodexEnvironment holds environment configuration for a Codex task.
type CodexEnvironment struct {
	RepoMap map[string]CodexRepo `json:"repo_map"`
}

// CodexRepo represents a repository in the Codex environment.
type CodexRepo struct {
	GitURL string `json:"git_url"`
	Name   string `json:"name"`
}

// ProcessCodexTask processes a Codex Cloud task response into messages.
func ProcessCodexTask(cloudData json.RawMessage) (CloudProcessResult, error) {
	var task CodexTask
	if err := json.Unmarshal(cloudData, &task); err != nil {
		return CloudProcessResult{}, fmt.Errorf("unmarshal codex task: %w", err)
	}

	if len(task.TurnMapping) == 0 {
		// Log top-level keys for debugging.
		var raw map[string]json.RawMessage
		json.Unmarshal(cloudData, &raw)
		keys := make([]string, 0, len(raw))
		for k := range raw {
			keys = append(keys, k)
		}
		log.Printf("[import-codex] empty turn_mapping — cloudData has %d bytes, top-level keys: %v", len(cloudData), keys)
		return CloudProcessResult{}, fmt.Errorf("empty turn_mapping (cloudData %d bytes, keys: %v)", len(cloudData), keys)
	}

	// Collect turns and sort by created_at.
	turns := make([]CodexTurnWrapper, 0, len(task.TurnMapping))
	for _, tw := range task.TurnMapping {
		turns = append(turns, tw)
	}
	sort.Slice(turns, func(i, j int) bool {
		return turns[i].Turn.CreatedAt < turns[j].Turn.CreatedAt
	})

	var messages []db.Message
	var model string
	var repoURL string

	var totalOutputItems int
	outputItemTypes := map[string]int{}

	for _, tw := range turns {
		turn := tw.Turn
		ts := int64(turn.CreatedAt * 1000)

		// Extract repo URL from first turn's environment.
		if repoURL == "" && turn.Environment != nil {
			for _, repo := range turn.Environment.RepoMap {
				if repo.GitURL != "" {
					repoURL = repo.GitURL
					break
				}
			}
		}

		// User turn: role == "user" && type == "user"
		if turn.Role == "user" && turn.Type == "user" {
			text := extractCodexUserText(turn.InputItems)
			if text != "" {
				messages = append(messages, db.Message{
					Timestamp:   ts,
					Role:        "user",
					MessageType: db.MessageTypePrompt,
					Content:     text,
				})
			}
			continue
		}

		// Agent turn: has output_items
		if len(turn.OutputItems) > 0 {
			if model == "" && turn.ModelVersion != "" {
				model = turn.ModelVersion
			}

			// Offset timestamps by 1ms per item within the same turn to avoid
			// UNIQUE(conversation_id, timestamp) collisions in the messages table.
			itemTS := ts

			for _, rawItem := range turn.OutputItems {
				var item CodexOutputItem
				if err := json.Unmarshal(rawItem, &item); err != nil {
					continue
				}
				totalOutputItems++
				outputItemTypes[item.Type]++

				switch {
				case item.Type == "message" && item.Role == "assistant":
					text := extractCodexTextBlocks(item.Content)
					if text != "" {
						messages = append(messages, db.Message{
							Timestamp:   itemTS,
							Role:        "agent",
							MessageType: db.MessageTypeFinalAnswer,
							Content:     text,
							Model:       turn.ModelVersion,
						})
						itemTS++
					}

				case item.Type == "pr" && item.OutputDiff != nil && strings.TrimSpace(item.OutputDiff.Diff) != "":
					diffContent := agent.FormatDiffMessage(item.OutputDiff.Diff)
					if diffContent != "" {
						messages = append(messages, db.Message{
							Timestamp:   itemTS,
							Role:        "agent",
							MessageType: db.MessageTypeLog,
							Content:     diffContent,
							Model:       turn.ModelVersion,
							RawJSON:     agent.DerivedDiffRawJSON,
						})
						itemTS++
					}
				}
			}
		}
	}

	log.Printf("[import-codex] processed %d turns, %d output_items %v, produced %d messages",
		len(turns), totalOutputItems, outputItemTypes, len(messages))

	// Derive title from first user message.
	var title string
	for _, m := range messages {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			title = agent.TitleFromPrompt(m.Content)
			break
		}
	}

	return CloudProcessResult{
		Messages: messages,
		Title:    title,
		Model:    model,
		RepoURL:  repoURL,
	}, nil
}

// extractCodexUserText extracts text from user input items.
func extractCodexUserText(items []CodexInputItem) string {
	var parts []string
	for _, item := range items {
		if item.Type == "message" && item.Role == "user" {
			text := extractCodexTextBlocks(item.Content)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// extractCodexTextBlocks concatenates text blocks from content, ignoring non-text types.
func extractCodexTextBlocks(blocks []CodexContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.ContentType == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "")
}
