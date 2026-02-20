package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Rating represents a single conversation rating.
type Rating struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	// TempConversationID is the per-rating temporary ID returned to plugins.
	// It can be used as an alias that resolves to ConversationID.
	TempConversationID string    `json:"tempConversationId"`
	Rating             int       `json:"rating"`
	Note               string    `json:"note"`
	Analysis           string    `json:"analysis"`
	CreatedAt          time.Time `json:"createdAt"`
	// MatchedTimestamp is the message timestamp of the /zrate user message
	// that was matched to this rating (within 120s). Nil if unmatched.
	MatchedTimestamp *int64 `json:"matchedTimestamp,omitempty"`
}

// InsertRating creates a new rating and returns the persisted record.
func InsertRating(ctx context.Context, db *sql.DB, conversationID string, rating int, note, analysis string) (*Rating, error) {
	return InsertRatingWithTemp(ctx, db, conversationID, conversationID, rating, note, analysis)
}

// InsertRatingWithTemp creates a new rating with explicit canonical and temp conversation IDs.
func InsertRatingWithTemp(ctx context.Context, db *sql.DB, conversationID, tempConversationID string, rating int, note, analysis string) (*Rating, error) {
	if tempConversationID == "" {
		tempConversationID = conversationID
	}

	id := newID()
	now := time.Now().UTC()

	_, err := db.ExecContext(ctx,
		"INSERT INTO ratings (id, conversation_id, temp_conversation_id, rating, note, analysis, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, conversationID, tempConversationID, rating, note, analysis, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert rating: %w", err)
	}

	return &Rating{
		ID:                 id,
		ConversationID:     conversationID,
		TempConversationID: tempConversationID,
		Rating:             rating,
		Note:               note,
		Analysis:           analysis,
		CreatedAt:          now,
	}, nil
}

// UpdateConversationID replaces the conversation_id on an existing rating.
func UpdateConversationID(ctx context.Context, db *sql.DB, ratingID, conversationID string) error {
	res, err := db.ExecContext(ctx, "UPDATE ratings SET conversation_id = ? WHERE id = ?", conversationID, ratingID)
	if err != nil {
		return fmt.Errorf("update conversation_id: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("rating %s: %w", ratingID, ErrNotFound)
	}
	return nil
}

// ListRatings returns the most recent ratings, up to limit.
func ListRatings(ctx context.Context, db *sql.DB, limit int) ([]Rating, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	rows, err := db.QueryContext(ctx,
		"SELECT id, conversation_id, temp_conversation_id, rating, note, analysis, created_at FROM ratings ORDER BY created_at DESC LIMIT ?",
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
		if err := rows.Scan(&r.ID, &r.ConversationID, &r.TempConversationID, &r.Rating, &r.Note, &r.Analysis, &createdAt); err != nil {
			return nil, fmt.Errorf("scan rating: %w", err)
		}

		r.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse rating created_at %q: %w", createdAt, err)
		}

		ratings = append(ratings, r)
	}

	return ratings, rows.Err()
}

// ResolveConversationIDByTempID resolves a temporary conversation alias ID to
// its canonical conversation ID using the latest matching rating row.
func ResolveConversationIDByTempID(ctx context.Context, db *sql.DB, tempConversationID string) (string, bool, error) {
	var conversationID string
	err := db.QueryRowContext(ctx,
		`SELECT conversation_id
		 FROM ratings
		 WHERE temp_conversation_id = ?
		 ORDER BY created_at DESC
		 LIMIT 1`,
		tempConversationID,
	).Scan(&conversationID)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("resolve conversation by temp id: %w", err)
	}
	return conversationID, true, nil
}

// ReconcileOrphanedRating finds an orphaned rating (whose conversation_id has no
// matching row in conversations) that matches the given rating value, note, and
// timestamp within 60 seconds, then updates it to point to realSessionID.
func ReconcileOrphanedRating(ctx context.Context, db *sql.DB, rating int, note string, historyTimestampMs int64, realSessionID string) error {
	// Convert history timestamp to time.Time for comparison with created_at.
	historyTime := time.UnixMilli(historyTimestampMs).UTC()
	windowStart := historyTime.Add(-60 * time.Second)
	windowEnd := historyTime.Add(60 * time.Second)

	var ratingID string
	err := db.QueryRowContext(ctx, `
		SELECT r.id FROM ratings r
		LEFT JOIN conversations c ON r.conversation_id = c.id
		WHERE c.id IS NULL
		  AND r.rating = ?
		  AND r.note = ?
		  AND r.created_at >= ?
		  AND r.created_at <= ?
		ORDER BY ABS(CAST(strftime('%s', r.created_at) AS INTEGER) - CAST(strftime('%s', ?) AS INTEGER)) ASC
		LIMIT 1`,
		rating, note, windowStart, windowEnd, historyTime,
	).Scan(&ratingID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find orphaned rating: %w", err)
	}

	res, err := db.ExecContext(ctx, "UPDATE ratings SET conversation_id = ? WHERE id = ?", realSessionID, ratingID)
	if err != nil {
		return fmt.Errorf("update orphaned rating: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("rating %s: %w", ratingID, ErrNotFound)
	}

	return nil
}
