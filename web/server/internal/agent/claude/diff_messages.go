package claude

import (
	"github.com/davidcann/zrate/web/server/internal/agent"
	"github.com/davidcann/zrate/web/server/internal/db"
)

func appendDiffEntries(entries []agent.Entry) []agent.Entry {
	if len(entries) == 0 {
		return entries
	}

	usedTimestamps := make(map[int64]struct{}, len(entries))
	for _, e := range entries {
		usedTimestamps[e.Timestamp] = struct{}{}
	}

	out := make([]agent.Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
		diff, ok := agent.ExtractReliableDiff(e.Display)
		if !ok {
			diff, ok = agent.ExtractReliableDiffFromJSON(e.RawJSON)
		}
		if !ok {
			continue
		}

		ts := e.Timestamp + 1
		for {
			if _, exists := usedTimestamps[ts]; !exists {
				break
			}
			ts++
		}
		usedTimestamps[ts] = struct{}{}

		out = append(out, agent.Entry{
			Timestamp: ts,
			SessionID: e.SessionID,
			Project:   e.Project,
			Role:      e.Role,
			Model:     e.Model,
			Display:   agent.FormatDiffMessage(diff),
			RawJSON:   `{"source":"derived_diff"}`,
		})
	}
	return out
}

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
			Model:          m.Model,
			Content:        agent.FormatDiffMessage(diff),
			RawJSON:        `{"source":"derived_diff"}`,
		})
	}
	return out
}
