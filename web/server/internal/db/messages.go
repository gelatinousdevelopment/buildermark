package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
)

// Message holds the data for a single conversation message to be inserted.
type Message struct {
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

// dupWindowMs is the maximum timestamp difference (in milliseconds) within which
// two messages with the same conversation, role, and content are considered duplicates.
const dupWindowMs = 10_000 // 10 seconds

// InsertMessages inserts multiple messages in a single transaction, skipping duplicates.
// Duplicates are detected both within the batch (same conversation + role + content
// within dupWindowMs) and against existing rows in the database.
func InsertMessages(ctx context.Context, db *sql.DB, messages []Message) error {
	messages = deduplicateMessages(messages)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO messages (id, timestamp, project_id, conversation_id, role, content, raw_json)
		 SELECT ?, ?, ?, ?, ?, ?, ?
		 WHERE NOT EXISTS (
		     SELECT 1 FROM messages
		     WHERE conversation_id = ? AND role = ? AND content = ?
		     AND ABS(timestamp - ?) < ?
		 )`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert message: %w", err)
	}
	defer stmt.Close()

	for _, m := range messages {
		if _, err := stmt.ExecContext(ctx,
			uuid.New().String(), m.Timestamp, m.ProjectID, m.ConversationID, m.Role, m.Content, m.RawJSON,
			m.ConversationID, m.Role, m.Content, m.Timestamp, dupWindowMs,
		); err != nil {
			return fmt.Errorf("insert message: %w", err)
		}
	}

	return tx.Commit()
}

// deduplicateMessages removes messages within the same conversation that have the same
// role and content within dupWindowMs, keeping only the first occurrence.
func deduplicateMessages(messages []Message) []Message {
	type key struct {
		conversationID string
		role           string
		content        string
	}
	seen := make(map[key]int64) // key -> first-seen timestamp
	result := make([]Message, 0, len(messages))
	for _, m := range messages {
		k := key{m.ConversationID, m.Role, m.Content}
		if prevTs, ok := seen[k]; ok && absInt64(m.Timestamp-prevTs) < dupWindowMs {
			continue
		}
		seen[k] = m.Timestamp
		result = append(result, m)
	}
	return result
}

func absInt64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
