package db

import (
	"context"
	"database/sql"
	"fmt"
)

func supportsFTS5(ctx context.Context, database *sql.DB) bool {
	var enabled int
	if err := database.QueryRowContext(ctx, "SELECT sqlite_compileoption_used('ENABLE_FTS5')").Scan(&enabled); err != nil {
		return false
	}
	return enabled == 1
}

func ensureSearchIndexSchema(ctx context.Context, database *sql.DB) error {
	if supportsFTS5(ctx, database) {
		return ensureFTS5SearchSchema(ctx, database)
	}
	return ensureFallbackSearchSchema(ctx, database)
}

func ensureFTS5SearchSchema(ctx context.Context, database *sql.DB) error {
	statements := []string{
		`DROP TRIGGER IF EXISTS messages_fts_ai`,
		`DROP TRIGGER IF EXISTS messages_fts_ad`,
		`DROP TRIGGER IF EXISTS messages_fts_au`,
		`DROP TRIGGER IF EXISTS commits_fts_ai`,
		`DROP TRIGGER IF EXISTS commits_fts_ad`,
		`DROP TRIGGER IF EXISTS commits_fts_au`,
		`DROP TABLE IF EXISTS messages_fts`,
		`DROP TABLE IF EXISTS commits_fts`,
		`CREATE VIRTUAL TABLE messages_fts USING fts5(
			message_id UNINDEXED,
			conversation_id UNINDEXED,
			project_id UNINDEXED,
			content,
			tokenize='trigram'
		)`,
		`CREATE VIRTUAL TABLE commits_fts USING fts5(
			commit_id UNINDEXED,
			project_id UNINDEXED,
			commit_hash,
			subject,
			diff_content,
			tokenize='trigram'
		)`,
		`CREATE TRIGGER messages_fts_ai
		AFTER INSERT ON messages
		WHEN NEW.role = 'user'
		BEGIN
			INSERT INTO messages_fts (message_id, conversation_id, project_id, content)
			VALUES (NEW.id, NEW.conversation_id, NEW.project_id, NEW.content);
		END`,
		`CREATE TRIGGER messages_fts_ad
		AFTER DELETE ON messages
		WHEN OLD.role = 'user'
		BEGIN
			DELETE FROM messages_fts WHERE message_id = OLD.id;
		END`,
		`CREATE TRIGGER messages_fts_au
		AFTER UPDATE ON messages
		BEGIN
			DELETE FROM messages_fts WHERE message_id = OLD.id;
			INSERT INTO messages_fts (message_id, conversation_id, project_id, content)
			SELECT NEW.id, NEW.conversation_id, NEW.project_id, NEW.content
			WHERE NEW.role = 'user';
		END`,
		`CREATE TRIGGER commits_fts_ai
		AFTER INSERT ON commits
		BEGIN
			INSERT INTO commits_fts (commit_id, project_id, commit_hash, subject, diff_content)
			VALUES (NEW.id, NEW.project_id, NEW.commit_hash, NEW.subject, NEW.diff_content);
		END`,
		`CREATE TRIGGER commits_fts_ad
		AFTER DELETE ON commits
		BEGIN
			DELETE FROM commits_fts WHERE commit_id = OLD.id;
		END`,
		`CREATE TRIGGER commits_fts_au
		AFTER UPDATE ON commits
		BEGIN
			DELETE FROM commits_fts WHERE commit_id = OLD.id;
			INSERT INTO commits_fts (commit_id, project_id, commit_hash, subject, diff_content)
			VALUES (NEW.id, NEW.project_id, NEW.commit_hash, NEW.subject, NEW.diff_content);
		END`,
		`INSERT INTO messages_fts (message_id, conversation_id, project_id, content)
		SELECT id, conversation_id, project_id, content
		FROM messages
		WHERE role = 'user'`,
		`INSERT INTO commits_fts (commit_id, project_id, commit_hash, subject, diff_content)
		SELECT id, project_id, commit_hash, subject, diff_content
		FROM commits`,
	}
	for _, stmt := range statements {
		if _, err := database.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure fts5 search schema: %w", err)
		}
	}
	return nil
}

func ensureFallbackSearchSchema(ctx context.Context, database *sql.DB) error {
	statements := []string{
		`DROP TRIGGER IF EXISTS messages_fts_ai`,
		`DROP TRIGGER IF EXISTS messages_fts_ad`,
		`DROP TRIGGER IF EXISTS messages_fts_au`,
		`DROP TRIGGER IF EXISTS commits_fts_ai`,
		`DROP TRIGGER IF EXISTS commits_fts_ad`,
		`DROP TRIGGER IF EXISTS commits_fts_au`,
		`DROP TABLE IF EXISTS messages_fts`,
		`DROP TABLE IF EXISTS commits_fts`,
		`CREATE TABLE messages_fts (
			message_id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			project_id TEXT NOT NULL,
			content TEXT NOT NULL
		)`,
		`CREATE INDEX idx_messages_fts_project_conversation
		ON messages_fts(project_id, conversation_id)`,
		`CREATE TABLE commits_fts (
			commit_id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			commit_hash TEXT NOT NULL,
			subject TEXT NOT NULL,
			diff_content TEXT NOT NULL
		)`,
		`CREATE INDEX idx_commits_fts_project_hash
		ON commits_fts(project_id, commit_hash)`,
		`CREATE TRIGGER messages_fts_ai
		AFTER INSERT ON messages
		WHEN NEW.role = 'user'
		BEGIN
			INSERT OR REPLACE INTO messages_fts (message_id, conversation_id, project_id, content)
			VALUES (NEW.id, NEW.conversation_id, NEW.project_id, NEW.content);
		END`,
		`CREATE TRIGGER messages_fts_ad
		AFTER DELETE ON messages
		WHEN OLD.role = 'user'
		BEGIN
			DELETE FROM messages_fts WHERE message_id = OLD.id;
		END`,
		`CREATE TRIGGER messages_fts_au
		AFTER UPDATE ON messages
		BEGIN
			DELETE FROM messages_fts WHERE message_id = OLD.id;
			INSERT OR REPLACE INTO messages_fts (message_id, conversation_id, project_id, content)
			SELECT NEW.id, NEW.conversation_id, NEW.project_id, NEW.content
			WHERE NEW.role = 'user';
		END`,
		`CREATE TRIGGER commits_fts_ai
		AFTER INSERT ON commits
		BEGIN
			INSERT OR REPLACE INTO commits_fts (commit_id, project_id, commit_hash, subject, diff_content)
			VALUES (NEW.id, NEW.project_id, NEW.commit_hash, NEW.subject, NEW.diff_content);
		END`,
		`CREATE TRIGGER commits_fts_ad
		AFTER DELETE ON commits
		BEGIN
			DELETE FROM commits_fts WHERE commit_id = OLD.id;
		END`,
		`CREATE TRIGGER commits_fts_au
		AFTER UPDATE ON commits
		BEGIN
			DELETE FROM commits_fts WHERE commit_id = OLD.id;
			INSERT OR REPLACE INTO commits_fts (commit_id, project_id, commit_hash, subject, diff_content)
			VALUES (NEW.id, NEW.project_id, NEW.commit_hash, NEW.subject, NEW.diff_content);
		END`,
		`INSERT INTO messages_fts (message_id, conversation_id, project_id, content)
		SELECT id, conversation_id, project_id, content
		FROM messages
		WHERE role = 'user'`,
		`INSERT INTO commits_fts (commit_id, project_id, commit_hash, subject, diff_content)
		SELECT id, project_id, commit_hash, subject, diff_content
		FROM commits`,
	}
	for _, stmt := range statements {
		if _, err := database.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure fallback search schema: %w", err)
		}
	}
	return nil
}
