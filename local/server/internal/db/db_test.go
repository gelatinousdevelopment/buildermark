package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
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
	if err := ensureIncrementalAutoVacuum(context.Background(), db); err != nil {
		db.Close()
		t.Fatalf("configure auto vacuum: %v", err)
	}
	if err := runMigrations(db); err != nil {
		db.Close()
		t.Fatalf("run migrations: %v", err)
	}
	if err := ensureSearchIndexSchema(context.Background(), db); err != nil {
		db.Close()
		t.Fatalf("initialize search schema: %v", err)
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
	_, err = db.Exec("SELECT id, conversation_id, temp_conversation_id, rating, note, created_at FROM ratings LIMIT 0")
	if err != nil {
		t.Fatalf("ratings table missing or wrong schema: %v", err)
	}

	// Verify new tables exist with expected columns.
	_, err = db.Exec("SELECT id, path, old_paths, label, git_id, ignore_diff_paths FROM projects LIMIT 0")
	if err != nil {
		t.Fatalf("projects table missing or wrong schema: %v", err)
	}
	_, err = db.Exec("SELECT id, project_id, agent, title, started_at, ended_at, hidden FROM conversations LIMIT 0")
	if err != nil {
		t.Fatalf("conversations table missing or wrong schema: %v", err)
	}
	_, err = db.Exec("SELECT id, timestamp, project_id, conversation_id, role, message_type, model, content, raw_json FROM messages LIMIT 0")
	if err != nil {
		t.Fatalf("messages table missing or wrong schema: %v", err)
	}
	_, err = db.Exec("SELECT id, type, source, source_id, timestamp, content FROM import_logs LIMIT 0")
	if err != nil {
		t.Fatalf("import_logs table missing or wrong schema: %v", err)
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

func TestTimestampColumnsUseIntegerUnixMs(t *testing.T) {
	db := setupTestDB(t)

	assertColumnType := func(table, column, wantType string) {
		t.Helper()
		rows, err := db.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			t.Fatalf("pragma table_info(%s): %v", table, err)
		}
		defer rows.Close()

		for rows.Next() {
			var cid int
			var name, colType string
			var notNull int
			var dflt sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
				t.Fatalf("scan pragma for %s: %v", table, err)
			}
			if name == column {
				if strings.ToUpper(colType) != wantType {
					t.Fatalf("%s.%s type = %s, want %s", table, column, colType, wantType)
				}
				return
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate pragma for %s: %v", table, err)
		}
		t.Fatalf("column %s.%s not found", table, column)
	}

	assertColumnType("ratings", "created_at", "INTEGER")
	assertColumnType("commits", "created_at", "INTEGER")
	assertColumnType("schema_version", "applied_at", "INTEGER")
	assertColumnType("import_logs", "timestamp", "INTEGER")

	assertColumnMissing := func(table, column string) {
		t.Helper()
		rows, err := db.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			t.Fatalf("pragma table_info(%s): %v", table, err)
		}
		defer rows.Close()

		for rows.Next() {
			var cid int
			var name, colType string
			var notNull int
			var dflt sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
				t.Fatalf("scan pragma for %s: %v", table, err)
			}
			if name == column {
				t.Fatalf("column %s.%s should be absent", table, column)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate pragma for %s: %v", table, err)
		}
	}

	assertColumnMissing("commits", "chars_total")
	assertColumnMissing("commits", "chars_from_agent")
	assertColumnMissing("commit_agent_coverage", "chars_from_agent")
}

func TestMigration44DropsCharColumnsWithOrphanedCommits(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	stmts := []string{
		`CREATE TABLE schema_version (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL DEFAULT 0
		)`,
		`INSERT INTO schema_version (version, applied_at) VALUES (43, 0)`,
		`CREATE TABLE projects (
			id TEXT PRIMARY KEY
		)`,
		`INSERT INTO projects (id) VALUES ('project-valid')`,
		`CREATE TABLE commits (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id),
			commit_hash TEXT NOT NULL,
			subject TEXT NOT NULL DEFAULT '',
			user_name TEXT NOT NULL DEFAULT '',
			user_email TEXT NOT NULL DEFAULT '',
			authored_at INTEGER NOT NULL DEFAULT 0,
			diff_content TEXT NOT NULL DEFAULT '',
			lines_total INTEGER NOT NULL DEFAULT 0,
			chars_total INTEGER NOT NULL DEFAULT 0,
			lines_from_agent INTEGER NOT NULL DEFAULT 0,
			chars_from_agent INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL DEFAULT 0,
			branch_name TEXT NOT NULL DEFAULT '',
			coverage_version INTEGER NOT NULL DEFAULT 0,
			lines_added INTEGER NOT NULL DEFAULT 0,
			lines_removed INTEGER NOT NULL DEFAULT 0,
			override_line_percent REAL DEFAULT NULL,
			needs_parent INTEGER NOT NULL DEFAULT 0,
			detail_files TEXT NOT NULL DEFAULT '',
			detail_messages TEXT NOT NULL DEFAULT '',
			detail_agent_segments TEXT NOT NULL DEFAULT '',
			detail_exact_matched INTEGER NOT NULL DEFAULT 0,
			detail_fallback_lines INTEGER NOT NULL DEFAULT 0,
			UNIQUE(project_id, commit_hash)
		)`,
		`CREATE TABLE commit_agent_coverage (
			id TEXT PRIMARY KEY,
			commit_id TEXT NOT NULL,
			agent TEXT NOT NULL DEFAULT '',
			lines_from_agent INTEGER NOT NULL DEFAULT 0,
			chars_from_agent INTEGER NOT NULL DEFAULT 0,
			UNIQUE(commit_id, agent)
		)`,
		`CREATE TABLE conversations (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL DEFAULT '',
			agent TEXT NOT NULL DEFAULT 'claude',
			title TEXT NOT NULL DEFAULT '',
			parent_conversation_id TEXT NOT NULL DEFAULT '',
			hidden INTEGER NOT NULL DEFAULT 0,
			started_at INTEGER NOT NULL DEFAULT 0,
			ended_at INTEGER NOT NULL DEFAULT 0,
			user_prompt_count INTEGER NOT NULL DEFAULT 0,
			files_edited TEXT NOT NULL DEFAULT '',
			url TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE messages (
			id TEXT PRIMARY KEY,
			message_type TEXT NOT NULL DEFAULT 'log',
			raw_json TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE commit_conversation_links (
			commit_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			UNIQUE(commit_id, conversation_id)
		)`,
		`INSERT INTO commits (
			id, project_id, commit_hash, subject, user_name, user_email,
			authored_at, diff_content, lines_total, chars_total, lines_from_agent, chars_from_agent,
			created_at, branch_name, coverage_version, lines_added, lines_removed, override_line_percent, needs_parent
		) VALUES
			('commit-valid', 'project-valid', 'hash-valid', 'valid', 'User', 'user@example.com', 1, '', 3, 10, 2, 7, 1, 'main', 8, 2, 1, NULL, 0),
			('commit-orphan', 'project-missing', 'hash-orphan', 'orphan', 'User', 'user@example.com', 2, '', 4, 11, 1, 5, 2, 'main', 8, 3, 2, NULL, 0)`,
		`INSERT INTO commit_agent_coverage (id, commit_id, agent, lines_from_agent, chars_from_agent) VALUES
			('cac-valid', 'commit-valid', 'codex', 2, 7),
			('cac-orphan', 'commit-orphan', 'codex', 1, 5)`,
		`INSERT INTO commit_conversation_links (commit_id, conversation_id) VALUES
			('commit-valid', 'conv-valid'),
			('commit-orphan', 'conv-orphan')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed pre-044 schema: %v", err)
		}
	}

	if err := runMigrations(db); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	var commitCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM commits`).Scan(&commitCount); err != nil {
		t.Fatalf("count commits: %v", err)
	}
	if commitCount != 1 {
		t.Fatalf("commit count = %d, want 1", commitCount)
	}

	var projectID string
	if err := db.QueryRow(`SELECT project_id FROM commits WHERE id = 'commit-valid'`).Scan(&projectID); err != nil {
		t.Fatalf("query valid commit: %v", err)
	}
	if projectID != "project-valid" {
		t.Fatalf("project_id = %q, want %q", projectID, "project-valid")
	}

	var coverageCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM commit_agent_coverage`).Scan(&coverageCount); err != nil {
		t.Fatalf("count commit_agent_coverage: %v", err)
	}
	if coverageCount != 1 {
		t.Fatalf("commit_agent_coverage count = %d, want 1", coverageCount)
	}

	var linkCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM commit_conversation_links`).Scan(&linkCount); err != nil {
		t.Fatalf("count commit_conversation_links: %v", err)
	}
	if linkCount != 1 {
		t.Fatalf("commit_conversation_links count = %d, want 1", linkCount)
	}
}

func TestMigration49BackfillsDerivedDiffRowsToDiffType(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	stmts := []string{
		`CREATE TABLE schema_version (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL DEFAULT 0
		)`,
		`INSERT INTO schema_version (version, applied_at) VALUES (48, 0)`,
		`CREATE TABLE projects (
			id TEXT PRIMARY KEY
		)`,
		`CREATE TABLE commits (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id),
			commit_hash TEXT NOT NULL,
			subject TEXT NOT NULL DEFAULT '',
			user_name TEXT NOT NULL DEFAULT '',
			user_email TEXT NOT NULL DEFAULT '',
			authored_at INTEGER NOT NULL DEFAULT 0,
			diff_content TEXT NOT NULL DEFAULT '',
			lines_total INTEGER NOT NULL DEFAULT 0,
			lines_from_agent INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL DEFAULT 0,
			branch_name TEXT NOT NULL DEFAULT '',
			coverage_version INTEGER NOT NULL DEFAULT 0,
			lines_added INTEGER NOT NULL DEFAULT 0,
			lines_removed INTEGER NOT NULL DEFAULT 0,
			override_line_percent REAL DEFAULT NULL,
			needs_parent INTEGER NOT NULL DEFAULT 0,
			detail_files TEXT NOT NULL DEFAULT '',
			detail_messages TEXT NOT NULL DEFAULT '',
			detail_agent_segments TEXT NOT NULL DEFAULT '',
			detail_exact_matched INTEGER NOT NULL DEFAULT 0,
			detail_fallback_lines INTEGER NOT NULL DEFAULT 0,
			UNIQUE(project_id, commit_hash)
		)`,
		`CREATE TABLE messages (
			id TEXT PRIMARY KEY,
			message_type TEXT NOT NULL DEFAULT 'log',
			raw_json TEXT NOT NULL DEFAULT ''
		)`,
		`INSERT INTO messages (id, message_type, raw_json) VALUES
			('m-diff', 'log', '{"source":"derived_diff"}'),
			('m-log', 'log', '{"source":"other"}')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed statement failed: %v\n%s", err, stmt)
		}
	}

	if err := runMigrations(db); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	var diffType string
	if err := db.QueryRow(`SELECT message_type FROM messages WHERE id = 'm-diff'`).Scan(&diffType); err != nil {
		t.Fatalf("query m-diff: %v", err)
	}
	if diffType != MessageTypeDiff {
		t.Fatalf("m-diff message_type = %q, want %q", diffType, MessageTypeDiff)
	}

	var logType string
	if err := db.QueryRow(`SELECT message_type FROM messages WHERE id = 'm-log'`).Scan(&logType); err != nil {
		t.Fatalf("query m-log: %v", err)
	}
	if logType != MessageTypeLog {
		t.Fatalf("m-log message_type = %q, want %q", logType, MessageTypeLog)
	}
}

func TestInitDBSetsIncrementalAutoVacuum(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "buildermark.db")
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	var mode int
	if err := database.QueryRow("PRAGMA auto_vacuum").Scan(&mode); err != nil {
		t.Fatalf("query auto_vacuum: %v", err)
	}
	if mode != autoVacuumIncremental {
		t.Fatalf("auto_vacuum mode = %d, want %d", mode, autoVacuumIncremental)
	}
}

func TestEnsureIncrementalAutoVacuumUpgradesExistingDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "existing.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := raw.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		raw.Close()
		t.Fatalf("create table: %v", err)
	}
	if _, err := raw.Exec("INSERT INTO t(v) VALUES ('x')"); err != nil {
		raw.Close()
		t.Fatalf("insert row: %v", err)
	}
	raw.Close()

	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB existing db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	var mode int
	if err := database.QueryRow("PRAGMA auto_vacuum").Scan(&mode); err != nil {
		t.Fatalf("query auto_vacuum: %v", err)
	}
	if mode != autoVacuumIncremental {
		t.Fatalf("auto_vacuum mode = %d, want %d", mode, autoVacuumIncremental)
	}
}
