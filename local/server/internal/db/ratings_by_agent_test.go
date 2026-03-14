package db

import (
	"context"
	"testing"
	"time"
)

func TestRatingsByAgent_EmptyProject(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/empty/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	now := time.Now()
	start := now.Add(-24 * time.Hour).UnixMilli()
	end := now.Add(24 * time.Hour).UnixMilli()

	result, err := GetRatingsByAgent(ctx, db, projectID, start, end)
	if err != nil {
		t.Fatalf("GetRatingsByAgent: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(result) != 0 {
		t.Errorf("got %d agents, want 0", len(result))
	}
}

func TestRatingsByAgent_MixedRatedUnrated(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	nowMs := time.Now().UnixMilli()

	// Create conversations: 3 claude, 2 codex
	for i, id := range []string{"conv-1", "conv-2", "conv-3"} {
		if err := EnsureConversation(ctx, db, id, projectID, "claude"); err != nil {
			t.Fatalf("EnsureConversation %d: %v", i, err)
		}
		// Set started_at to now so it falls in range.
		if _, err := db.ExecContext(ctx, "UPDATE conversations SET started_at = ? WHERE id = ?", nowMs, id); err != nil {
			t.Fatalf("update started_at %d: %v", i, err)
		}
	}
	for i, id := range []string{"conv-4", "conv-5"} {
		if err := EnsureConversation(ctx, db, id, projectID, "codex"); err != nil {
			t.Fatalf("EnsureConversation codex %d: %v", i, err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE conversations SET started_at = ? WHERE id = ?", nowMs, id); err != nil {
			t.Fatalf("update started_at codex %d: %v", i, err)
		}
	}

	// Rate some conversations.
	if _, err := InsertRating(ctx, db, "conv-1", 5, "", ""); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}
	if _, err := InsertRating(ctx, db, "conv-2", 3, "", ""); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}
	// conv-3 unrated
	if _, err := InsertRating(ctx, db, "conv-4", 4, "", ""); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}
	// conv-5 unrated

	start := nowMs - 1000*60*60 // 1 hour ago
	end := nowMs + 1000*60*60   // 1 hour from now

	result, err := GetRatingsByAgent(ctx, db, projectID, start, end)
	if err != nil {
		t.Fatalf("GetRatingsByAgent: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("got %d agents, want 2", len(result))
	}

	// Find claude and codex in results.
	agentMap := make(map[string]AgentRatingDistribution)
	for _, a := range result {
		agentMap[a.Agent] = a
	}

	claude := agentMap["claude"]
	if claude.TotalConversations != 3 {
		t.Errorf("claude total = %d, want 3", claude.TotalConversations)
	}
	if claude.RatedConversations != 2 {
		t.Errorf("claude rated = %d, want 2", claude.RatedConversations)
	}
	if claude.AverageRating != 4.0 { // (5+3)/2 = 4
		t.Errorf("claude avg = %f, want 4.0", claude.AverageRating)
	}
	if claude.Distribution["unrated"] != 1 {
		t.Errorf("claude unrated = %d, want 1", claude.Distribution["unrated"])
	}
	if claude.Distribution["5"] != 1 {
		t.Errorf("claude 5-star = %d, want 1", claude.Distribution["5"])
	}
	if claude.Distribution["3"] != 1 {
		t.Errorf("claude 3-star = %d, want 1", claude.Distribution["3"])
	}

	codex := agentMap["codex"]
	if codex.TotalConversations != 2 {
		t.Errorf("codex total = %d, want 2", codex.TotalConversations)
	}
	if codex.RatedConversations != 1 {
		t.Errorf("codex rated = %d, want 1", codex.RatedConversations)
	}
	if codex.Distribution["4"] != 1 {
		t.Errorf("codex 4-star = %d, want 1", codex.Distribution["4"])
	}
	if codex.Distribution["unrated"] != 1 {
		t.Errorf("codex unrated = %d, want 1", codex.Distribution["unrated"])
	}
}

func TestRatingsByAgent_MultiRatingAveraging(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/multi")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	nowMs := time.Now().UnixMilli()

	if err := EnsureConversation(ctx, db, "conv-mr", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE conversations SET started_at = ? WHERE id = ?", nowMs, "conv-mr"); err != nil {
		t.Fatalf("update started_at: %v", err)
	}

	// Multiple ratings for the same conversation: 4 and 2, avg = 3
	if _, err := InsertRating(ctx, db, "conv-mr", 4, "", ""); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}
	if _, err := InsertRating(ctx, db, "conv-mr", 2, "", ""); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	start := nowMs - 1000*60*60
	end := nowMs + 1000*60*60

	result, err := GetRatingsByAgent(ctx, db, projectID, start, end)
	if err != nil {
		t.Fatalf("GetRatingsByAgent: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("got %d agents, want 1", len(result))
	}
	if result[0].Distribution["3"] != 1 {
		t.Errorf("expected bucket 3 with count 1, got distribution: %v", result[0].Distribution)
	}
	if result[0].AverageRating != 3.0 {
		t.Errorf("avg = %f, want 3.0", result[0].AverageRating)
	}
}

func TestRatingsByAgent_DateFiltering(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projectID, err := EnsureProject(ctx, db, "/test/datefilter")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	nowMs := time.Now().UnixMilli()
	oldMs := nowMs - 7*24*60*60*1000 // 7 days ago

	// One conversation in range, one outside.
	if err := EnsureConversation(ctx, db, "conv-new", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE conversations SET started_at = ? WHERE id = ?", nowMs, "conv-new"); err != nil {
		t.Fatalf("update: %v", err)
	}

	if err := EnsureConversation(ctx, db, "conv-old", projectID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE conversations SET started_at = ? WHERE id = ?", oldMs, "conv-old"); err != nil {
		t.Fatalf("update: %v", err)
	}

	if _, err := InsertRating(ctx, db, "conv-new", 5, "", ""); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}
	if _, err := InsertRating(ctx, db, "conv-old", 1, "", ""); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	// Query only the last 24 hours — should only include conv-new.
	start := nowMs - 24*60*60*1000
	end := nowMs + 1000*60*60

	result, err := GetRatingsByAgent(ctx, db, projectID, start, end)
	if err != nil {
		t.Fatalf("GetRatingsByAgent: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("got %d agents, want 1", len(result))
	}
	if result[0].TotalConversations != 1 {
		t.Errorf("total = %d, want 1", result[0].TotalConversations)
	}
	if result[0].Distribution["5"] != 1 {
		t.Errorf("expected 5-star bucket, got: %v", result[0].Distribution)
	}
}
