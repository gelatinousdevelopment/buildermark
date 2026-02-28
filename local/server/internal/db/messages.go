package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Message holds the data for a single conversation message to be inserted.
type Message struct {
	Timestamp      int64
	ProjectID      string
	ConversationID string
	Role           string
	Model          string
	Content        string
	RawJSON        string
}

// RepoLabel returns the name of the git repository root directory for the
// given path. It walks up the directory tree looking for a .git entry. If no
// git root is found it falls back to the last path component.
func RepoLabel(path string) string {
	dir := filepath.Clean(path)
	for {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			// .git can be a directory (normal repo) or a file (worktree/submodule).
			_ = info
			return filepath.Base(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Base(path)
}

// EnsureProject inserts a project if it doesn't already exist and returns its ID.
func EnsureProject(ctx context.Context, db *sql.DB, path string) (string, error) {
	var id string
	// Check aliases first so renamed/moved projects continue to map to the
	// canonical project even if a legacy exact-path project row still exists.
	if existingID, err := findProjectIDByOldPath(ctx, db, path); err != nil {
		return "", fmt.Errorf("query project by old path: %w", err)
	} else if existingID != "" {
		return existingID, nil
	}

	err := db.QueryRowContext(ctx, "SELECT id FROM projects WHERE path = ?", path).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("query project: %w", err)
	}

	id = newID()
	_, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO projects (id, path, label) VALUES (?, ?, ?)", id, path, RepoLabel(path))
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

func findProjectIDByOldPath(ctx context.Context, db *sql.DB, path string) (string, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, old_paths FROM projects WHERE old_paths <> ''")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var oldPaths string
		if err := rows.Scan(&id, &oldPaths); err != nil {
			return "", err
		}
		for _, oldPath := range strings.Split(oldPaths, "\n") {
			if strings.TrimSpace(oldPath) == path {
				return id, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", nil
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

const hiddenIngestMessageMaxLen = 256

var hiddenIngestMessageRe = regexp.MustCompile(`(?s)^[\[<].*[\]>]$`)

// InsertMessages inserts multiple messages in a single transaction, skipping duplicates.
// Duplicates are detected both within the batch (same conversation + role + content
// within dupWindowMs) and against existing rows in the database.
func InsertMessages(ctx context.Context, db *sql.DB, messages []Message) error {
	messages = filterMessagesForIngest(messages)
	messages = deduplicateMessages(messages)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO messages (id, timestamp, project_id, conversation_id, role, model, content, raw_json)
		 SELECT ?, ?, ?, ?, ?, ?, ?, ?
		 WHERE NOT EXISTS (
		     SELECT 1 FROM messages
		     WHERE conversation_id = ? AND role = ? AND model = ? AND content = ?
		     AND ABS(timestamp - ?) < ?
		 )`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert message: %w", err)
	}
	defer stmt.Close()

	updateModelStmt, err := tx.PrepareContext(ctx,
		`UPDATE messages
		 SET model = ?
		 WHERE conversation_id = ? AND timestamp = ? AND model = ''`,
	)
	if err != nil {
		return fmt.Errorf("prepare update message model: %w", err)
	}
	defer updateModelStmt.Close()

	conversationIDs := make(map[string]struct{}, len(messages))
	for _, m := range messages {
		conversationIDs[m.ConversationID] = struct{}{}
		if _, err := stmt.ExecContext(ctx,
			newID(), m.Timestamp, m.ProjectID, m.ConversationID, m.Role, m.Model, m.Content, m.RawJSON,
			m.ConversationID, m.Role, m.Model, m.Content, m.Timestamp, dupWindowMs,
		); err != nil {
			return fmt.Errorf("insert message: %w", err)
		}
		if m.Model != "" {
			if _, err := updateModelStmt.ExecContext(ctx, m.Model, m.ConversationID, m.Timestamp); err != nil {
				return fmt.Errorf("update message model: %w", err)
			}
		}
	}

	updateConversationBoundsStmt, err := tx.PrepareContext(ctx,
		`UPDATE conversations
		 SET
		     started_at = CASE
		         WHEN started_at = 0 THEN ?
		         WHEN ? = 0 THEN started_at
		         ELSE MIN(started_at, ?)
		     END,
		     ended_at = MAX(ended_at, ?)
		 WHERE id = ?`,
	)
	if err != nil {
		return fmt.Errorf("prepare update conversation bounds: %w", err)
	}
	defer updateConversationBoundsStmt.Close()

	conversationBounds := make(map[string][2]int64, len(messages))
	for _, m := range messages {
		b, ok := conversationBounds[m.ConversationID]
		if !ok {
			conversationBounds[m.ConversationID] = [2]int64{m.Timestamp, m.Timestamp}
			continue
		}
		if m.Timestamp < b[0] {
			b[0] = m.Timestamp
		}
		if m.Timestamp > b[1] {
			b[1] = m.Timestamp
		}
		conversationBounds[m.ConversationID] = b
	}

	for conversationID := range conversationIDs {
		bounds, ok := conversationBounds[conversationID]
		if !ok {
			continue
		}
		batchMin, batchMax := bounds[0], bounds[1]
		if _, err := updateConversationBoundsStmt.ExecContext(ctx, batchMin, batchMin, batchMin, batchMax, conversationID); err != nil {
			return fmt.Errorf("update conversation bounds: %w", err)
		}
	}

	// Recalculate user_prompt_count for each affected conversation.
	updatePromptCountStmt, err := tx.PrepareContext(ctx,
		`UPDATE conversations SET user_prompt_count = (
			SELECT COUNT(*) FROM messages WHERE conversation_id = ? AND role = 'user'
		) WHERE id = ?`,
	)
	if err != nil {
		return fmt.Errorf("prepare update user_prompt_count: %w", err)
	}
	defer updatePromptCountStmt.Close()

	for conversationID := range conversationIDs {
		if _, err := updatePromptCountStmt.ExecContext(ctx, conversationID, conversationID); err != nil {
			return fmt.Errorf("update user_prompt_count: %w", err)
		}
	}

	return tx.Commit()
}

func filterMessagesForIngest(messages []Message) []Message {
	result := make([]Message, 0, len(messages))
	for _, m := range messages {
		if shouldSkipMessageOnIngest(m) {
			continue
		}
		result = append(result, m)
	}
	return result
}

func shouldSkipMessageOnIngest(m Message) bool {
	trimmed := strings.TrimSpace(m.Content)
	if trimmed == "" {
		return false
	}
	if utf8.RuneCountInString(trimmed) >= hiddenIngestMessageMaxLen {
		return false
	}
	return hiddenIngestMessageRe.MatchString(trimmed)
}

// deduplicateMessages removes messages within the same conversation that have the same
// role and content within dupWindowMs, keeping only the first occurrence.
func deduplicateMessages(messages []Message) []Message {
	type key struct {
		conversationID string
		role           string
		model          string
		content        string
	}
	seen := make(map[key]int64) // key -> first-seen timestamp
	result := make([]Message, 0, len(messages))
	for _, m := range messages {
		k := key{m.ConversationID, m.Role, m.Model, m.Content}
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
