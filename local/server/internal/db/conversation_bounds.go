package db

import (
	"context"
	"database/sql"
	"fmt"
)

// backfillConversationBounds recomputes conversation started_at/ended_at from
// persisted messages, excluding Buildermark rating workflow messages.
func backfillConversationBounds(ctx context.Context, db *sql.DB) error {
	if !tableHasColumn(ctx, db, "messages", "content") ||
		!tableHasColumn(ctx, db, "messages", "message_type") ||
		!tableHasColumn(ctx, db, "messages", "raw_json") {
		return nil
	}

	rows, err := db.QueryContext(ctx, `SELECT id FROM conversations`)
	if err != nil {
		return fmt.Errorf("query conversations for bounds backfill: %w", err)
	}
	defer rows.Close()

	conversationIDs := make([]string, 0, 128)
	bounds := make(map[string]conversationBounds)
	for rows.Next() {
		var conversationID string
		if err := rows.Scan(&conversationID); err != nil {
			return fmt.Errorf("scan conversation id for bounds backfill: %w", err)
		}
		conversationIDs = append(conversationIDs, conversationID)
		bounds[conversationID] = conversationBounds{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate conversations for bounds backfill: %w", err)
	}

	messageRows, err := db.QueryContext(ctx,
		`SELECT conversation_id, timestamp, message_type, content, raw_json
		 FROM messages
		 ORDER BY conversation_id ASC, timestamp ASC`,
	)
	if err != nil {
		return fmt.Errorf("query messages for bounds backfill: %w", err)
	}
	defer messageRows.Close()

	for messageRows.Next() {
		var conversationID, messageType, content, rawJSON string
		var timestamp int64
		if err := messageRows.Scan(&conversationID, &timestamp, &messageType, &content, &rawJSON); err != nil {
			return fmt.Errorf("scan message for bounds backfill: %w", err)
		}
		if shouldIgnoreMessageForConversationBounds(content, rawJSON) {
			continue
		}

		bound := bounds[conversationID]
		if !bound.startedSet || timestamp < bound.startedAt {
			bound.startedAt = timestamp
			bound.startedSet = true
		}
		if messageType == MessageTypePrompt || messageType == MessageTypeAnswer {
			if timestamp > bound.endedAt {
				bound.endedAt = timestamp
			}
		}
		bounds[conversationID] = bound
	}
	if err := messageRows.Err(); err != nil {
		return fmt.Errorf("iterate messages for bounds backfill: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin bounds backfill tx: %w", err)
	}
	defer tx.Rollback()

	updateStmt, err := tx.PrepareContext(ctx, `UPDATE conversations SET started_at = ?, ended_at = ? WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("prepare bounds backfill update: %w", err)
	}
	defer updateStmt.Close()

	for _, conversationID := range conversationIDs {
		bound := bounds[conversationID]
		startedAt := int64(0)
		if bound.startedSet {
			startedAt = bound.startedAt
		}
		if _, err := updateStmt.ExecContext(ctx, startedAt, bound.endedAt, conversationID); err != nil {
			return fmt.Errorf("update bounds for %s: %w", conversationID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit bounds backfill: %w", err)
	}

	return nil
}

type conversationBounds struct {
	startedAt  int64
	endedAt    int64
	startedSet bool
}

func tableHasColumn(ctx context.Context, db *sql.DB, tableName, columnName string) bool {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+tableName+")")
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return false
		}
		if name == columnName {
			return true
		}
	}

	return false
}
