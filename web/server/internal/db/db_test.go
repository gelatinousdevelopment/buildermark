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
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count)
	if err != nil {
		t.Fatalf("query schema_version count: %v", err)
	}
	if version != count {
		t.Errorf("expected max schema version to match count (%d), got %d", count, version)
	}
	if version < 1 {
		t.Errorf("expected at least one migration version, got %d", version)
	}

	// Verify ratings table exists with expected columns.
	_, err = db.Exec("SELECT id, conversation_id, rating, note, created_at FROM ratings LIMIT 0")
	if err != nil {
		t.Fatalf("ratings table missing or wrong schema: %v", err)
	}

	// Verify new tables exist with expected columns.
	_, err = db.Exec("SELECT id, path, label, git_id, ignore_diff_paths FROM projects LIMIT 0")
	if err != nil {
		t.Fatalf("projects table missing or wrong schema: %v", err)
	}
	_, err = db.Exec("SELECT id, project_id, agent, title FROM conversations LIMIT 0")
	if err != nil {
		t.Fatalf("conversations table missing or wrong schema: %v", err)
	}
	_, err = db.Exec("SELECT id, timestamp, project_id, conversation_id, role, model, content, raw_json FROM messages LIMIT 0")
	if err != nil {
		t.Fatalf("messages table missing or wrong schema: %v", err)
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	db := setupTestDB(t)

	var countBefore int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&countBefore)
	if err != nil {
		t.Fatalf("query schema_version before second run: %v", err)
	}

	// Run migrations a second time — should be a no-op.
	if err := runMigrations(db); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}

	var countAfter int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&countAfter)
	if err != nil {
		t.Fatalf("query schema_version after second run: %v", err)
	}
	if countAfter != countBefore {
		t.Errorf("expected migration count to remain %d after idempotent run, got %d", countBefore, countAfter)
	}
}
