package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DeleteConversationsAndMessagesByStartedAtWindow deletes only conversations
// and messages where the conversation started_at is within the provided window.
// It intentionally does not delete rows from any other table.
func DeleteConversationsAndMessagesByStartedAtWindow(ctx context.Context, database *sql.DB, since time.Time) (int64, int64, error) {
	sinceMs := since.UnixMilli()

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	msgRes, err := tx.ExecContext(ctx, `DELETE FROM messages
		WHERE conversation_id IN (
			SELECT id FROM conversations
			WHERE started_at >= ? AND started_at > 0
		)`, sinceMs)
	if err != nil {
		return 0, 0, fmt.Errorf("delete messages in window: %w", err)
	}
	deletedMessages, err := msgRes.RowsAffected()
	if err != nil {
		return 0, 0, fmt.Errorf("messages rows affected: %w", err)
	}

	convRes, err := tx.ExecContext(ctx, `DELETE FROM conversations
		WHERE started_at >= ? AND started_at > 0`, sinceMs)
	if err != nil {
		return 0, 0, fmt.Errorf("delete conversations in window: %w", err)
	}
	deletedConversations, err := convRes.RowsAffected()
	if err != nil {
		return 0, 0, fmt.Errorf("conversations rows affected: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit window delete: %w", err)
	}
	return deletedConversations, deletedMessages, nil
}
