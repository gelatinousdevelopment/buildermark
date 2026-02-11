package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Conversation represents a row in the conversations table.
type Conversation struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Agent     string `json:"agent"`
}

// TurnRead is a turn as returned by read queries (excludes raw_json).
type TurnRead struct {
	ID             string `json:"id"`
	Timestamp      int64  `json:"timestamp"`
	ConversationID string `json:"conversationId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
}

// ConversationDetail is a conversation with all its turns and ratings.
type ConversationDetail struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"projectId"`
	Agent     string     `json:"agent"`
	Turns     []TurnRead `json:"turns"`
	Ratings   []Rating   `json:"ratings"`
}

// ListConversations returns conversations, up to limit.
func ListConversations(ctx context.Context, db *sql.DB, limit int) ([]Conversation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := db.QueryContext(ctx, "SELECT id, project_id, agent FROM conversations ORDER BY id LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer rows.Close()

	conversations := []Conversation{}
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Agent); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		conversations = append(conversations, c)
	}
	return conversations, rows.Err()
}

// GetConversationDetail returns a conversation with all its turns and ratings.
func GetConversationDetail(ctx context.Context, db *sql.DB, conversationID string) (*ConversationDetail, error) {
	var c ConversationDetail
	err := db.QueryRowContext(ctx,
		"SELECT id, project_id, agent FROM conversations WHERE id = ?", conversationID,
	).Scan(&c.ID, &c.ProjectID, &c.Agent)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query conversation: %w", err)
	}

	// Fetch turns ordered by most recent first.
	turnRows, err := db.QueryContext(ctx,
		"SELECT id, timestamp, conversation_id, role, content FROM turns WHERE conversation_id = ? ORDER BY timestamp DESC",
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("query turns: %w", err)
	}
	defer turnRows.Close()

	c.Turns = []TurnRead{}
	for turnRows.Next() {
		var t TurnRead
		if err := turnRows.Scan(&t.ID, &t.Timestamp, &t.ConversationID, &t.Role, &t.Content); err != nil {
			return nil, fmt.Errorf("scan turn: %w", err)
		}
		c.Turns = append(c.Turns, t)
	}
	if err := turnRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate turns: %w", err)
	}

	// Fetch ratings.
	ratRows, err := db.QueryContext(ctx,
		"SELECT id, conversation_id, rating, note, analysis, created_at FROM ratings WHERE conversation_id = ? ORDER BY created_at DESC",
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("query ratings: %w", err)
	}
	defer ratRows.Close()

	c.Ratings = []Rating{}
	for ratRows.Next() {
		var r Rating
		var createdAt string
		if err := ratRows.Scan(&r.ID, &r.ConversationID, &r.Rating, &r.Note, &r.Analysis, &createdAt); err != nil {
			return nil, fmt.Errorf("scan rating: %w", err)
		}
		r.CreatedAt = parseTime(createdAt)
		c.Ratings = append(c.Ratings, r)
	}
	if err := ratRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ratings: %w", err)
	}

	return &c, nil
}
