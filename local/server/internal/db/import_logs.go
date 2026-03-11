package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ImportLog stores a raw import request for future reprocessing.
type ImportLog struct {
	ID        string
	Type      string
	Source    string
	SourceID  string
	Timestamp int64
	Content   string
}

// InsertImportLog persists a single raw import request.
func InsertImportLog(ctx context.Context, database *sql.DB, log ImportLog) error {
	if strings.TrimSpace(log.ID) == "" {
		log.ID = newID()
	}

	_, err := database.ExecContext(ctx,
		"INSERT INTO import_logs (id, type, source, source_id, timestamp, content) VALUES (?, ?, ?, ?, ?, ?)",
		log.ID, nullableImportLogText(log.Type), nullableImportLogText(log.Source), log.SourceID, log.Timestamp, log.Content,
	)
	if err != nil {
		return fmt.Errorf("insert import log: %w", err)
	}

	return nil
}

func nullableImportLogText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
