package db

import (
	"context"
	"database/sql"
	"fmt"
)

// GetCommitDetailFilesInRange returns raw detail_files JSON strings from commits
// in the given project and time range where detail_files is not empty.
func GetCommitDetailFilesInRange(ctx context.Context, db *sql.DB, projectID string, startSec, endSec int64) ([]string, error) {
	query := `
SELECT detail_files
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

	var results []string
	for rows.Next() {
		var detailFiles string
		if err := rows.Scan(&detailFiles); err != nil {
			return nil, fmt.Errorf("scan commit detail files: %w", err)
		}
		results = append(results, detailFiles)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commit detail files: %w", err)
	}
	return results, nil
}
