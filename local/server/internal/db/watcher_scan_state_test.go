package db

import (
	"context"
	"testing"
)

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
