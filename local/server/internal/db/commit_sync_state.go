package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// CommitSyncState tracks asynchronous commit refresh state per project+branch.
type CommitSyncState struct {
	ProjectID             string `json:"projectId"`
	BranchName            string `json:"branchName"`
	State                 string `json:"state"`
	LatestKnownHeadHash   string `json:"latestKnownHeadHash"`
	LastProcessedHeadHash string `json:"lastProcessedHeadHash"`
	EstimatedTotalCommits int    `json:"estimatedTotalCommits"`
	LastStartedAtMs       int64  `json:"lastStartedAtMs"`
	LastFinishedAtMs      int64  `json:"lastFinishedAtMs"`
	LastDurationMs        int64  `json:"lastDurationMs"`
	LastError             string `json:"lastError"`
}

func GetCommitSyncState(ctx context.Context, database *sql.DB, projectID, branchName string) (*CommitSyncState, error) {
	var st CommitSyncState
	err := database.QueryRowContext(ctx, `SELECT project_id, branch_name, state, latest_known_head_hash, last_processed_head_hash, estimated_total_commits, last_started_at_ms, last_finished_at_ms, last_duration_ms, last_error
		FROM commit_sync_state
		WHERE project_id = ? AND branch_name = ?`, projectID, branchName).
		Scan(&st.ProjectID, &st.BranchName, &st.State, &st.LatestKnownHeadHash, &st.LastProcessedHeadHash, &st.EstimatedTotalCommits, &st.LastStartedAtMs, &st.LastFinishedAtMs, &st.LastDurationMs, &st.LastError)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get commit sync state: %w", err)
	}
	return &st, nil
}

func UpsertCommitSyncState(ctx context.Context, database *sql.DB, st CommitSyncState) error {
	st.State = strings.TrimSpace(st.State)
	if st.State == "" {
		st.State = "idle"
	}
	_, err := database.ExecContext(ctx, `INSERT INTO commit_sync_state (project_id, branch_name, state, latest_known_head_hash, last_processed_head_hash, estimated_total_commits, last_started_at_ms, last_finished_at_ms, last_duration_ms, last_error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, branch_name) DO UPDATE SET
			state = excluded.state,
			latest_known_head_hash = excluded.latest_known_head_hash,
			last_processed_head_hash = excluded.last_processed_head_hash,
			estimated_total_commits = excluded.estimated_total_commits,
			last_started_at_ms = excluded.last_started_at_ms,
			last_finished_at_ms = excluded.last_finished_at_ms,
			last_duration_ms = excluded.last_duration_ms,
			last_error = excluded.last_error`,
		st.ProjectID, st.BranchName, st.State, st.LatestKnownHeadHash, st.LastProcessedHeadHash, st.EstimatedTotalCommits, st.LastStartedAtMs, st.LastFinishedAtMs, st.LastDurationMs, st.LastError,
	)
	if err != nil {
		return fmt.Errorf("upsert commit sync state: %w", err)
	}
	return nil
}
