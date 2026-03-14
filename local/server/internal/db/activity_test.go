package db

import (
	"context"
	"testing"
	"time"
)

func TestGetDailyActivityCountsConversationOnceOnLatestUserMessageDay(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	if err := EnsureConversation(ctx, db, "conv-main", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation conv-main: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-hidden", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation conv-hidden: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-parent", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation conv-parent: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-child", projectID, "codex"); err != nil {
		t.Fatalf("EnsureConversation conv-child: %v", err)
	}
	if err := UpdateConversationParent(ctx, db, "conv-child", "conv-parent"); err != nil {
		t.Fatalf("UpdateConversationParent: %v", err)
	}
	if err := SetConversationHidden(ctx, db, "conv-hidden", true); err != nil {
		t.Fatalf("SetConversationHidden: %v", err)
	}

	if err := InsertMessages(ctx, db, []Message{
		{
			Timestamp:      mustUnixMsUTC(2026, time.March, 10, 9, 0),
			ProjectID:      projectID,
			ConversationID: "conv-main",
			Role:           "user",
			MessageType:    MessageTypePrompt,
			Content:        "start work",
			RawJSON:        `{}`,
		},
		{
			Timestamp:      mustUnixMsUTC(2026, time.March, 10, 11, 0),
			ProjectID:      projectID,
			ConversationID: "conv-main",
			Role:           "agent",
			MessageType:    MessageTypeFinalAnswer,
			Content:        "first reply",
			RawJSON:        `{}`,
		},
		{
			Timestamp:      mustUnixMsUTC(2026, time.March, 11, 8, 0),
			ProjectID:      projectID,
			ConversationID: "conv-main",
			Role:           "user",
			MessageType:    MessageTypeAnswer,
			Content:        "yes, continue",
			RawJSON:        `{}`,
		},
		{
			Timestamp:      mustUnixMsUTC(2026, time.March, 12, 7, 0),
			ProjectID:      projectID,
			ConversationID: "conv-main",
			Role:           "agent",
			MessageType:    MessageTypeFinalAnswer,
			Content:        "done",
			RawJSON:        `{}`,
		},
		{
			Timestamp:      mustUnixMsUTC(2026, time.March, 10, 12, 0),
			ProjectID:      projectID,
			ConversationID: "conv-hidden",
			Role:           "user",
			MessageType:    MessageTypePrompt,
			Content:        "hidden work",
			RawJSON:        `{}`,
		},
		{
			Timestamp:      mustUnixMsUTC(2026, time.March, 10, 13, 0),
			ProjectID:      projectID,
			ConversationID: "conv-child",
			Role:           "user",
			MessageType:    MessageTypePrompt,
			Content:        "Implement the following plan:\n# Fix the thing",
			RawJSON:        `{}`,
		},
		{
			Timestamp:      mustUnixMsUTC(2026, time.March, 11, 9, 0),
			ProjectID:      projectID,
			ConversationID: "conv-child",
			Role:           "user",
			MessageType:    MessageTypePrompt,
			Content:        "actual follow-up prompt",
			RawJSON:        `{}`,
		},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	rows, err := GetDailyActivity(
		ctx,
		db,
		projectID,
		mustUnixMsUTC(2026, time.March, 10, 0, 0),
		mustUnixMsUTC(2026, time.March, 13, 0, 0),
		"UTC",
		0,
	)
	if err != nil {
		t.Fatalf("GetDailyActivity: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}

	assertDailyRow(t, rows[0], "2026-03-10", 0, 1, 0)
	assertDailyRow(t, rows[1], "2026-03-11", 2, 1, 1)
	assertDailyRow(t, rows[2], "2026-03-12", 0, 0, 0)
}

func TestGetDailyActivityUsesTimeZoneAcrossDST(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-dst", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	if err := InsertMessages(ctx, db, []Message{
		{
			Timestamp:      time.Date(2026, time.March, 7, 23, 30, 0, 0, loc).UnixMilli(),
			ProjectID:      projectID,
			ConversationID: "conv-dst",
			Role:           "user",
			MessageType:    MessageTypePrompt,
			Content:        "late-night prompt",
			RawJSON:        `{}`,
		},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	rows, err := GetDailyActivity(
		ctx,
		db,
		projectID,
		time.Date(2026, time.March, 7, 0, 0, 0, 0, loc).UnixMilli(),
		time.Date(2026, time.March, 9, 0, 0, 0, 0, loc).UnixMilli(),
		"America/Los_Angeles",
		-420,
	)
	if err != nil {
		t.Fatalf("GetDailyActivity: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	assertDailyRow(t, rows[0], "2026-03-07", 1, 1, 0)
	assertDailyRow(t, rows[1], "2026-03-08", 0, 0, 0)
}

func mustUnixMsUTC(year int, month time.Month, day, hour, minute int) int64 {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC).UnixMilli()
}

func assertDailyRow(t *testing.T, row DailyActivityRow, wantDate string, wantConversations, wantPrompts, wantAnswers int) {
	t.Helper()
	if row.Date != wantDate {
		t.Fatalf("row.Date = %q, want %q", row.Date, wantDate)
	}
	if row.Conversations != wantConversations {
		t.Fatalf("%s conversations = %d, want %d", row.Date, row.Conversations, wantConversations)
	}
	if row.UserPrompts != wantPrompts {
		t.Fatalf("%s user prompts = %d, want %d", row.Date, row.UserPrompts, wantPrompts)
	}
	if row.UserAnswers != wantAnswers {
		t.Fatalf("%s user answers = %d, want %d", row.Date, row.UserAnswers, wantAnswers)
	}
}
