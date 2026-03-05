package agent

import (
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// DerivedDiffRawJSON is the JSON marker for synthetic diff messages.
const DerivedDiffRawJSON = `{"source":"derived_diff"}`

// DiffAppendOptions configures synthetic diff message derivation.
type DiffAppendOptions struct {
	// Deduplicate suppresses duplicate synthetic diffs with the same
	// conversation, role, and diff content.
	Deduplicate bool
	// UseAllJSONDiffs appends every reliable diff recovered from JSON payloads.
	// When false, behavior matches legacy extraction (single fallback diff).
	UseAllJSONDiffs bool
}

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
			RawJSON:   DerivedDiffRawJSON,
		})
	}
	return out
}

// AppendDiffDBMessages scans messages for embedded diffs and appends synthetic
// diff-only messages after each match. Used during watcher imports.
func AppendDiffDBMessages(messages []db.Message) []db.Message {
	return AppendDiffDBMessagesWithOptions(messages, DiffAppendOptions{})
}

// AppendDiffDBMessagesWithOptions scans messages for embedded diffs and appends
// synthetic diff-only messages after each match. Used during watcher imports.
func AppendDiffDBMessagesWithOptions(messages []db.Message, opts DiffAppendOptions) []db.Message {
	if len(messages) == 0 {
		return messages
	}

	usedTimestamps := make(map[int64]struct{}, len(messages))
	seenDerived := make(map[string]struct{}, len(messages))
	for _, m := range messages {
		usedTimestamps[m.Timestamp] = struct{}{}
	}

	out := make([]db.Message, 0, len(messages))
	for _, m := range messages {
		out = append(out, m)
		diffs := extractMessageDiffs(m, opts)
		if len(diffs) == 0 {
			continue
		}

		for _, diff := range diffs {
			if opts.Deduplicate {
				diffKey := m.ConversationID + "\n" + m.Role + "\n" + diff
				if _, seen := seenDerived[diffKey]; seen {
					continue
				}
				seenDerived[diffKey] = struct{}{}
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
				RawJSON:        DerivedDiffRawJSON,
			})
		}
	}
	return out
}

func extractMessageDiffs(m db.Message, opts DiffAppendOptions) []string {
	diffs := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	add := func(diff string) {
		diff = strings.TrimSpace(diff)
		if diff == "" {
			return
		}
		if _, exists := seen[diff]; exists {
			return
		}
		seen[diff] = struct{}{}
		diffs = append(diffs, diff)
	}

	if diff, ok := ExtractReliableDiff(m.Content); ok {
		add(diff)
	}

	if opts.UseAllJSONDiffs {
		for _, diff := range ExtractReliableDiffsFromJSON(m.RawJSON) {
			add(diff)
		}
		return diffs
	}

	if len(diffs) == 0 {
		if diff, ok := ExtractReliableDiffFromJSON(m.RawJSON); ok {
			add(diff)
		}
	}
	return diffs
}
