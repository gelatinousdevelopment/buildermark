package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Rating represents a single conversation rating.
type Rating struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Rating         int       `json:"rating"`
	Note           string    `json:"note"`
	CreatedAt      time.Time `json:"createdAt"`
}

// InsertRating creates a new rating and returns the persisted record.
func InsertRating(ctx context.Context, db *sql.DB, conversationID string, rating int, note string) (*Rating, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	_, err := db.ExecContext(ctx,
		"INSERT INTO ratings (id, conversation_id, rating, note, created_at) VALUES (?, ?, ?, ?, ?)",
		id, conversationID, rating, note, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert rating: %w", err)
	}

	return &Rating{
		ID:             id,
		ConversationID: conversationID,
		Rating:         rating,
		Note:           note,
		CreatedAt:      now,
	}, nil
}

// UpdateConversationID replaces the conversation_id on an existing rating.
func UpdateConversationID(ctx context.Context, db *sql.DB, ratingID, conversationID string) error {
	_, err := db.ExecContext(ctx, "UPDATE ratings SET conversation_id = ? WHERE id = ?", conversationID, ratingID)
	if err != nil {
		return fmt.Errorf("update conversation_id: %w", err)
	}
	return nil
}

// ListRatings returns the most recent ratings, up to limit.
func ListRatings(ctx context.Context, db *sql.DB, limit int) ([]Rating, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	rows, err := db.QueryContext(ctx,
		"SELECT id, conversation_id, rating, note, created_at FROM ratings ORDER BY created_at DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query ratings: %w", err)
	}
	defer rows.Close()

	ratings := []Rating{}
	for rows.Next() {
		var r Rating
		var createdAt string
		if err := rows.Scan(&r.ID, &r.ConversationID, &r.Rating, &r.Note, &createdAt); err != nil {
			return nil, fmt.Errorf("scan rating: %w", err)
		}

		// go-sqlite3 stores time.Time as RFC 3339 by default.
		// Fall back to the bare SQLite CURRENT_TIMESTAMP format for rows
		// that were not inserted through Go.
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		if r.CreatedAt.IsZero() {
			r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		}

		ratings = append(ratings, r)
	}

	return ratings, rows.Err()
}
