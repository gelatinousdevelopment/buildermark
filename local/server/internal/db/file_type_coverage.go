package db

import (
	"context"
	"database/sql"
	"fmt"
)

type CommitDetailFilesRow struct {
	DetailFiles           string
	OverrideAgentPercents *string
}

// GetCommitDetailFilesInRange returns raw detail_files JSON and override data
// from commits in the given project and time range where detail_files is not empty.
func GetCommitDetailFilesInRange(ctx context.Context, db *sql.DB, projectID string, startSec, endSec int64) ([]CommitDetailFilesRow, error) {
	query := `
SELECT detail_files, override_agent_percents
FROM commits
WHERE project_id = ?
  AND authored_at >= ?
  AND authored_at < ?
  AND detail_files != ''
  AND ignored = 0`

	rows, err := db.QueryContext(ctx, query, projectID, startSec, endSec)
	if err != nil {
		return nil, fmt.Errorf("query commit detail files in range: %w", err)
	}
	defer rows.Close()

	var results []CommitDetailFilesRow
	for rows.Next() {
		var row CommitDetailFilesRow
		var override sql.NullString
		if err := rows.Scan(&row.DetailFiles, &override); err != nil {
			return nil, fmt.Errorf("scan commit detail files: %w", err)
		}
		if override.Valid {
			row.OverrideAgentPercents = &override.String
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commit detail files: %w", err)
	}
	return results, nil
}
