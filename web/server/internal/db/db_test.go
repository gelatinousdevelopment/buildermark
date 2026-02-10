package db

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB opens an in-memory SQLite database with migrations applied.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := runMigrations(db); err != nil {
		db.Close()
		t.Fatalf("run migrations: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrationsRunCleanly(t *testing.T) {
	db := setupTestDB(t)

	// Verify schema_version table has an entry.
	var version int
	err := db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version)
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}

	// Verify ratings table exists with expected columns.
	_, err = db.Exec("SELECT id, conversation_id, rating, note, created_at FROM ratings LIMIT 0")
	if err != nil {
		t.Fatalf("ratings table missing or wrong schema: %v", err)
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	db := setupTestDB(t)

	// Run migrations a second time — should be a no-op.
	if err := runMigrations(db); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count)
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 version row after idempotent run, got %d", count)
	}
}
