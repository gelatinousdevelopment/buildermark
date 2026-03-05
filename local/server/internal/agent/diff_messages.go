package agent

import (
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// AppendDiffEntries scans entries for embedded diffs and appends synthetic
// diff-only entries after each match. Used during session resolution.
func AppendDiffEntries(entries []Entry) []Entry {
	if len(entries) == 0 {
		return entries
	}

	usedTimestamps := make(map[int64]struct{}, len(entries))
	for _, e := range entries {
		usedTimestamps[e.Timestamp] = struct{}{}
	}

	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
		diff, ok := ExtractReliableDiff(e.Display)
		if !ok {
			diff, ok = ExtractReliableDiffFromJSON(e.RawJSON)
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

		out = append(out, Entry{
			Timestamp: ts,
			SessionID: e.SessionID,
			Project:   e.Project,
			Role:      e.Role,
			Model:     e.Model,
			Display:   FormatDiffMessage(diff),
			RawJSON:   `{"source":"derived_diff"}`,
		})
	}
	return out
}

// AppendDiffDBMessages scans messages for embedded diffs and appends synthetic
// diff-only messages after each match. Used during watcher imports.
func AppendDiffDBMessages(messages []db.Message) []db.Message {
	if len(messages) == 0 {
		return messages
	}

	usedTimestamps := make(map[int64]struct{}, len(messages))
	for _, m := range messages {
		usedTimestamps[m.Timestamp] = struct{}{}
	}

	out := make([]db.Message, 0, len(messages))
	for _, m := range messages {
		out = append(out, m)
		diff, ok := ExtractReliableDiff(m.Content)
		if !ok {
			diff, ok = ExtractReliableDiffFromJSON(m.RawJSON)
		}
		if !ok {
			continue
		}

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
			Content:        FormatDiffMessage(diff),
			RawJSON:        `{"source":"derived_diff"}`,
		})
	}
	return out
}
