package db

import (
	"context"
	"testing"
)

func TestLatestWatcherScanTimestamp(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	// No rows — should return 0.
	ts, err := LatestWatcherScanTimestamp(ctx, database, "claude")
	if err != nil {
		t.Fatalf("LatestWatcherScanTimestamp (empty): %v", err)
	}
	if ts != 0 {
		t.Fatalf("expected 0 for empty table, got %d", ts)
	}

	// Insert two rows with different updated_at_ms.
	if err := UpsertWatcherScanState(ctx, database, WatcherScanState{
		Agent:       "claude",
		SourceKind:  "history_file",
		SourceKey:   "/tmp/a",
		UpdatedAtMs: 1000,
	}); err != nil {
		t.Fatalf("insert row 1: %v", err)
	}
	if err := UpsertWatcherScanState(ctx, database, WatcherScanState{
		Agent:       "claude",
		SourceKind:  "project_file",
		SourceKey:   "/tmp/b",
		UpdatedAtMs: 5000,
	}); err != nil {
		t.Fatalf("insert row 2: %v", err)
	}
	// Different agent — should not affect claude's result.
	if err := UpsertWatcherScanState(ctx, database, WatcherScanState{
		Agent:       "codex",
		SourceKind:  "session_file",
		SourceKey:   "/tmp/c",
		UpdatedAtMs: 9999,
	}); err != nil {
		t.Fatalf("insert codex row: %v", err)
	}

	ts, err = LatestWatcherScanTimestamp(ctx, database, "claude")
	if err != nil {
		t.Fatalf("LatestWatcherScanTimestamp: %v", err)
	}
	if ts != 5000 {
		t.Fatalf("expected 5000, got %d", ts)
	}

	// Verify codex returns its own max.
	ts, err = LatestWatcherScanTimestamp(ctx, database, "codex")
	if err != nil {
		t.Fatalf("LatestWatcherScanTimestamp (codex): %v", err)
	}
	if ts != 9999 {
		t.Fatalf("expected 9999, got %d", ts)
	}

	// Non-existent agent returns 0.
	ts, err = LatestWatcherScanTimestamp(ctx, database, "gemini")
	if err != nil {
		t.Fatalf("LatestWatcherScanTimestamp (gemini): %v", err)
	}
	if ts != 0 {
		t.Fatalf("expected 0 for non-existent agent, got %d", ts)
	}
}

func TestLatestWatcherScanTimestampForScopes(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	if err := UpsertWatcherScanState(ctx, database, WatcherScanState{
		Agent:       "claude",
		SourceKind:  "history_file",
		SourceKey:   "/Users/me/.claude/history.jsonl",
		UpdatedAtMs: 1000,
	}); err != nil {
		t.Fatalf("insert main history row: %v", err)
	}
	if err := UpsertWatcherScanState(ctx, database, WatcherScanState{
		Agent:       "claude",
		SourceKind:  "project_file",
		SourceKey:   "/Users/me/.claude/projects/-Users-me-repo/session.jsonl",
		UpdatedAtMs: 2000,
	}); err != nil {
		t.Fatalf("insert main project row: %v", err)
	}
	if err := UpsertWatcherScanState(ctx, database, WatcherScanState{
		Agent:       "claude",
		SourceKind:  "history_file",
		SourceKey:   "/Volumes/debian/.claude/history.jsonl",
		UpdatedAtMs: 3000,
	}); err != nil {
		t.Fatalf("insert debian history row: %v", err)
	}
	if err := UpsertWatcherScanState(ctx, database, WatcherScanState{
		Agent:       "claude",
		SourceKind:  "project_file",
		SourceKey:   "/Volumes/debian/.claude/projects/-home-debian-github-buildermark/011a10e8.jsonl",
		UpdatedAtMs: 4000,
	}); err != nil {
		t.Fatalf("insert debian project row: %v", err)
	}

	ts, err := LatestWatcherScanTimestampForScopes(ctx, database, "claude",
		WatcherScanScope{SourceKind: "history_file", SourceKey: "/Volumes/debian/.claude/history.jsonl"},
		WatcherScanScope{SourceKind: "project_file", SourceKey: "/Volumes/debian/.claude/projects", MatchPrefix: true},
	)
	if err != nil {
		t.Fatalf("LatestWatcherScanTimestampForScopes: %v", err)
	}
	if ts != 4000 {
		t.Fatalf("scoped timestamp = %d, want 4000", ts)
	}

	ts, err = LatestWatcherScanTimestampForScopes(ctx, database, "claude",
		WatcherScanScope{SourceKind: "history_file", SourceKey: "/missing/.claude/history.jsonl"},
	)
	if err != nil {
		t.Fatalf("LatestWatcherScanTimestampForScopes missing: %v", err)
	}
	if ts != 0 {
		t.Fatalf("missing scoped timestamp = %d, want 0", ts)
	}
}

func TestWatcherScanStateCRUD(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	state := WatcherScanState{
		Agent:       "claude",
		SourceKind:  "history_file",
		SourceKey:   "/tmp/history.jsonl",
		FileSize:    123,
		FileMtimeMs: 456,
		FileOffset:  120,
		StateJSON:   `{"foo":"bar"}`,
	}
	if err := UpsertWatcherScanState(ctx, database, state); err != nil {
		t.Fatalf("UpsertWatcherScanState insert: %v", err)
	}

	got, err := GetWatcherScanState(ctx, database, "claude", "history_file", "/tmp/history.jsonl")
	if err != nil {
		t.Fatalf("GetWatcherScanState: %v", err)
	}
	if got == nil {
		t.Fatalf("GetWatcherScanState returned nil")
	}
	if got.FileSize != 123 || got.FileMtimeMs != 456 || got.FileOffset != 120 {
		t.Fatalf("unexpected state values: %+v", got)
	}
	if got.StateJSON != `{"foo":"bar"}` {
		t.Fatalf("state json = %q", got.StateJSON)
	}
	if got.UpdatedAtMs == 0 {
		t.Fatalf("expected UpdatedAtMs to be set")
	}

	state.FileSize = 789
	state.FileOffset = 700
	state.StateJSON = `{"foo":"baz"}`
	if err := UpsertWatcherScanState(ctx, database, state); err != nil {
		t.Fatalf("UpsertWatcherScanState update: %v", err)
	}

	got, err = GetWatcherScanState(ctx, database, "claude", "history_file", "/tmp/history.jsonl")
	if err != nil {
		t.Fatalf("GetWatcherScanState after update: %v", err)
	}
	if got.FileSize != 789 || got.FileOffset != 700 || got.StateJSON != `{"foo":"baz"}` {
		t.Fatalf("unexpected updated state values: %+v", got)
	}

	states, err := ListWatcherScanStates(ctx, database, "claude", "history_file")
	if err != nil {
		t.Fatalf("ListWatcherScanStates: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("ListWatcherScanStates len = %d, want 1", len(states))
	}

	if err := DeleteWatcherScanState(ctx, database, "claude", "history_file", "/tmp/history.jsonl"); err != nil {
		t.Fatalf("DeleteWatcherScanState: %v", err)
	}
	got, err = GetWatcherScanState(ctx, database, "claude", "history_file", "/tmp/history.jsonl")
	if err != nil {
		t.Fatalf("GetWatcherScanState after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil state after delete")
	}
}
