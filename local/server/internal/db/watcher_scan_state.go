package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// WatcherScanState stores persisted scan checkpoint metadata for watcher sources.
type WatcherScanState struct {
	Agent      string `json:"agent"`
	SourceKind string `json:"sourceKind"`
	SourceKey  string `json:"sourceKey"`

	FileSize    int64  `json:"fileSize"`
	FileMtimeMs int64  `json:"fileMtimeMs"`
	FileOffset  int64  `json:"fileOffset"`
	StateJSON   string `json:"stateJson"`
	UpdatedAtMs int64  `json:"updatedAtMs"`
}

func GetWatcherScanState(ctx context.Context, database *sql.DB, agent, sourceKind, sourceKey string) (*WatcherScanState, error) {
	var st WatcherScanState
	err := database.QueryRowContext(ctx, `SELECT agent, source_kind, source_key, file_size, file_mtime_ms, file_offset, state_json, updated_at_ms
		FROM watcher_scan_state
		WHERE agent = ? AND source_kind = ? AND source_key = ?`, agent, sourceKind, sourceKey).
		Scan(&st.Agent, &st.SourceKind, &st.SourceKey, &st.FileSize, &st.FileMtimeMs, &st.FileOffset, &st.StateJSON, &st.UpdatedAtMs)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get watcher scan state: %w", err)
	}
	return &st, nil
}

func ListWatcherScanStates(ctx context.Context, database *sql.DB, agent, sourceKind string) ([]WatcherScanState, error) {
	rows, err := database.QueryContext(ctx, `SELECT agent, source_kind, source_key, file_size, file_mtime_ms, file_offset, state_json, updated_at_ms
		FROM watcher_scan_state
		WHERE agent = ? AND source_kind = ?`, agent, sourceKind)
	if err != nil {
		return nil, fmt.Errorf("list watcher scan states: %w", err)
	}
	defer rows.Close()

	states := make([]WatcherScanState, 0)
	for rows.Next() {
		var st WatcherScanState
		if err := rows.Scan(&st.Agent, &st.SourceKind, &st.SourceKey, &st.FileSize, &st.FileMtimeMs, &st.FileOffset, &st.StateJSON, &st.UpdatedAtMs); err != nil {
			return nil, fmt.Errorf("scan watcher scan state: %w", err)
		}
		states = append(states, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate watcher scan states: %w", err)
	}
	return states, nil
}

func UpsertWatcherScanState(ctx context.Context, database *sql.DB, st WatcherScanState) error {
	if st.UpdatedAtMs == 0 {
		st.UpdatedAtMs = time.Now().UnixMilli()
	}
	_, err := database.ExecContext(ctx, `INSERT INTO watcher_scan_state (agent, source_kind, source_key, file_size, file_mtime_ms, file_offset, state_json, updated_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent, source_kind, source_key) DO UPDATE SET
			file_size = excluded.file_size,
			file_mtime_ms = excluded.file_mtime_ms,
			file_offset = excluded.file_offset,
			state_json = excluded.state_json,
			updated_at_ms = excluded.updated_at_ms`,
		st.Agent, st.SourceKind, st.SourceKey, st.FileSize, st.FileMtimeMs, st.FileOffset, st.StateJSON, st.UpdatedAtMs,
	)
	if err != nil {
		return fmt.Errorf("upsert watcher scan state: %w", err)
	}
	return nil
}

// LatestWatcherScanTimestamp returns the most recent updated_at_ms across all
// scan state rows for the given agent. Returns 0 if no rows exist.
func LatestWatcherScanTimestamp(ctx context.Context, database *sql.DB, agent string) (int64, error) {
	var ts sql.NullInt64
	err := database.QueryRowContext(ctx, `SELECT MAX(updated_at_ms) FROM watcher_scan_state WHERE agent = ?`, agent).Scan(&ts)
	if err != nil {
		return 0, fmt.Errorf("latest watcher scan timestamp: %w", err)
	}
	if !ts.Valid {
		return 0, nil
	}
	return ts.Int64, nil
}

func DeleteWatcherScanState(ctx context.Context, database *sql.DB, agent, sourceKind, sourceKey string) error {
	res, err := database.ExecContext(ctx, `DELETE FROM watcher_scan_state WHERE agent = ? AND source_kind = ? AND source_key = ?`, agent, sourceKind, sourceKey)
	if err != nil {
		return fmt.Errorf("delete watcher scan state: %w", err)
	}
	deletedRows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("watcher scan state rows affected: %w", err)
	}
	if err := runIncrementalVacuum(ctx, database, deletedRows); err != nil {
		return err
	}
	return nil
}
