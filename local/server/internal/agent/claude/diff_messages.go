package claude

import (
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// appendDiffDBMessages is Claude-specific because it also uses snapshot-based
// diff derivation (deriveClaudeSnapshotDiff) and deduplication. Codex and
// Gemini use agent.AppendDiffDBMessages instead.
func appendDiffDBMessages(messages []db.Message) []db.Message {
	if len(messages) == 0 {
		return messages
	}

	usedTimestamps := make(map[int64]struct{}, len(messages))
	seenDerived := make(map[string]struct{}, len(messages))
	snapshotState := newClaudeSnapshotState()
	for _, m := range messages {
		usedTimestamps[m.Timestamp] = struct{}{}
	}

	out := make([]db.Message, 0, len(messages))
	for _, m := range messages {
		out = append(out, m)
		snapshotState.ingestRawJSON(m.RawJSON)
		diff, ok := agent.ExtractReliableDiff(m.Content)
		if !ok {
			diff, ok = agent.ExtractReliableDiffFromJSON(m.RawJSON)
		}
		if !ok {
			diff, ok = deriveClaudeSnapshotDiff(m, snapshotState)
		}
		if !ok {
			continue
		}
		diffKey := m.ConversationID + "\n" + m.Role + "\n" + diff
		if _, seen := seenDerived[diffKey]; seen {
			continue
		}
		seenDerived[diffKey] = struct{}{}

		ts := m.Timestamp + 1
		for {
			if _, exists := usedTimestamps[ts]; !exists {
				break
			}
			ts++
		}
		usedTimestamps[ts] = struct{}{}

		out = append(out, db.Message{
			Timestamp:      ts,
			ProjectID:      m.ProjectID,
			ConversationID: m.ConversationID,
			Role:           m.Role,
			MessageType:    db.MessageTypeDiff,
			Model:          m.Model,
			Content:        agent.FormatDiffMessage(diff),
			RawJSON:        agent.DerivedDiffRawJSON,
		})
	}
	return out
}
