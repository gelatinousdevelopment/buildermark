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
	Title     string `json:"title"`
}

// MessageRead is a message as returned by read queries.
type MessageRead struct {
	ID             string `json:"id"`
	Timestamp      int64  `json:"timestamp"`
	ConversationID string `json:"conversationId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	RawJSON        string `json:"rawJson"`
}

// ConversationDetail is a conversation with all its messages and ratings.
type ConversationDetail struct {
	ID        string        `json:"id"`
	ProjectID string        `json:"projectId"`
	Agent     string        `json:"agent"`
	Title     string        `json:"title"`
	Messages  []MessageRead `json:"messages"`
	Ratings   []Rating      `json:"ratings"`
}

// ListConversations returns conversations, up to limit.
func ListConversations(ctx context.Context, db *sql.DB, limit int) ([]Conversation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := db.QueryContext(ctx, "SELECT id, project_id, agent, title FROM conversations ORDER BY id LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer rows.Close()

	conversations := []Conversation{}
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Agent, &c.Title); err != nil {
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
		"SELECT id, project_id, agent, title FROM conversations WHERE id = ?", conversationID,
	).Scan(&c.ID, &c.ProjectID, &c.Agent, &c.Title)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query conversation: %w", err)
	}

	// Fetch messages ordered by most recent first.
	messageRows, err := db.QueryContext(ctx,
		"SELECT id, timestamp, conversation_id, role, content, raw_json FROM messages WHERE conversation_id = ? ORDER BY timestamp DESC",
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer messageRows.Close()

	c.Messages = []MessageRead{}
	for messageRows.Next() {
		var m MessageRead
		if err := messageRows.Scan(&m.ID, &m.Timestamp, &m.ConversationID, &m.Role, &m.Content, &m.RawJSON); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		c.Messages = append(c.Messages, m)
	}
	if err := messageRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
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
		r.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse rating created_at %q: %w", createdAt, err)
		}
		c.Ratings = append(c.Ratings, r)
	}
	if err := ratRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ratings: %w", err)
	}

	return &c, nil
}

// UntitledConversation is a conversation with an empty title, joined with its project path.
type UntitledConversation struct {
	ID          string
	ProjectPath string
}

// ListUntitledConversations returns conversations that have an empty title for the given agent.
func ListUntitledConversations(ctx context.Context, db *sql.DB, agent string) ([]UntitledConversation, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT c.id, p.path FROM conversations c JOIN projects p ON c.project_id = p.id WHERE c.agent = ? AND c.title = ''",
		agent,
	)
	if err != nil {
		return nil, fmt.Errorf("query untitled conversations: %w", err)
	}
	defer rows.Close()

	var result []UntitledConversation
	for rows.Next() {
		var u UntitledConversation
		if err := rows.Scan(&u.ID, &u.ProjectPath); err != nil {
			return nil, fmt.Errorf("scan untitled conversation: %w", err)
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

// UpdateConversationTitle sets the title on an existing conversation.
func UpdateConversationTitle(ctx context.Context, db *sql.DB, conversationID, title string) error {
	res, err := db.ExecContext(ctx, "UPDATE conversations SET title = ? WHERE id = ?", title, conversationID)
	if err != nil {
		return fmt.Errorf("update conversation title: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("conversation %s: %w", conversationID, ErrNotFound)
	}
	return nil
}

// UpdateConversationProject sets the project_id on an existing conversation.
func UpdateConversationProject(ctx context.Context, db *sql.DB, conversationID, projectID string) error {
	res, err := db.ExecContext(ctx, "UPDATE conversations SET project_id = ? WHERE id = ?", projectID, conversationID)
	if err != nil {
		return fmt.Errorf("update conversation project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("conversation %s: %w", conversationID, ErrNotFound)
	}
	return nil
}
