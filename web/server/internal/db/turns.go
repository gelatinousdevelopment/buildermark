package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
)

// Turn holds the data for a single conversation turn to be inserted.
type Turn struct {
	Timestamp      int64
	ProjectID      string
	ConversationID string
	Role           string
	Content        string
	RawJSON        string
}

// EnsureProject inserts a project if it doesn't already exist and returns its ID.
func EnsureProject(ctx context.Context, db *sql.DB, path string) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, "SELECT id FROM projects WHERE path = ?", path).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("query project: %w", err)
	}

	id = uuid.New().String()
	_, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO projects (id, path, label) VALUES (?, ?, ?)", id, path, filepath.Base(path))
	if err != nil {
		return "", fmt.Errorf("insert project: %w", err)
	}

	// Re-query in case another goroutine inserted first.
	err = db.QueryRowContext(ctx, "SELECT id FROM projects WHERE path = ?", path).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("re-query project: %w", err)
	}
	return id, nil
}

// EnsureConversation inserts a conversation if it doesn't already exist.
func EnsureConversation(ctx context.Context, db *sql.DB, conversationID, projectID, agent string) error {
	_, err := db.ExecContext(ctx,
		"INSERT OR IGNORE INTO conversations (id, project_id, agent) VALUES (?, ?, ?)",
		conversationID, projectID, agent,
	)
	if err != nil {
		return fmt.Errorf("insert conversation: %w", err)
	}
	return nil
}

// InsertTurns inserts multiple turns in a single transaction, skipping duplicates.
func InsertTurns(ctx context.Context, db *sql.DB, turns []Turn) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		"INSERT OR IGNORE INTO turns (id, timestamp, project_id, conversation_id, role, content, raw_json) VALUES (?, ?, ?, ?, ?, ?, ?)",
	)
	if err != nil {
		return fmt.Errorf("prepare insert turn: %w", err)
	}
	defer stmt.Close()

	for _, t := range turns {
		if _, err := stmt.ExecContext(ctx, uuid.New().String(), t.Timestamp, t.ProjectID, t.ConversationID, t.Role, t.Content, t.RawJSON); err != nil {
			return fmt.Errorf("insert turn: %w", err)
		}
	}

	return tx.Commit()
}
