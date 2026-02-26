package db

import (
	"context"
	"database/sql"
	"fmt"
)

const (
	autoVacuumIncremental      = 2
	incrementalVacuumChunkSize = 1000
)

func ensureIncrementalAutoVacuum(ctx context.Context, database *sql.DB) error {
	var mode int
	if err := database.QueryRowContext(ctx, "PRAGMA auto_vacuum").Scan(&mode); err != nil {
		return fmt.Errorf("read auto_vacuum mode: %w", err)
	}
	if mode == autoVacuumIncremental {
		return nil
	}

	if _, err := database.ExecContext(ctx, "PRAGMA auto_vacuum = INCREMENTAL"); err != nil {
		return fmt.Errorf("set auto_vacuum incremental: %w", err)
	}
	if _, err := database.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("vacuum database: %w", err)
	}
	return nil
}

func runIncrementalVacuum(ctx context.Context, database *sql.DB, deletedRows int64) error {
	if deletedRows <= 0 {
		return nil
	}
	if _, err := database.ExecContext(ctx, fmt.Sprintf("PRAGMA incremental_vacuum(%d)", incrementalVacuumChunkSize)); err != nil {
		return fmt.Errorf("incremental vacuum: %w", err)
	}
	return nil
}
