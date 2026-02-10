package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type Rating struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Rating         int       `json:"rating"`
	Note           string    `json:"note"`
	CreatedAt      time.Time `json:"createdAt"`
}

func InitDB(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS ratings (
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL,
		rating INTEGER NOT NULL CHECK(rating >= 0 AND rating <= 5),
		note TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_ratings_conversation_id ON ratings(conversation_id);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return db, nil
}

func InsertRating(db *sql.DB, conversationID string, rating int, note string) (*Rating, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	_, err := db.Exec(
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

func UpdateConversationID(db *sql.DB, ratingID, conversationID string) error {
	_, err := db.Exec("UPDATE ratings SET conversation_id = ? WHERE id = ?", conversationID, ratingID)
	if err != nil {
		return fmt.Errorf("update conversation_id: %w", err)
	}
	return nil
}

func ListRatings(db *sql.DB, limit int) ([]Rating, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	rows, err := db.Query(
		"SELECT id, conversation_id, rating, note, created_at FROM ratings ORDER BY created_at DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query ratings: %w", err)
	}
	defer rows.Close()

	var ratings []Rating
	for rows.Next() {
		var r Rating
		var createdAt string
		if err := rows.Scan(&r.ID, &r.ConversationID, &r.Rating, &r.Note, &createdAt); err != nil {
			return nil, fmt.Errorf("scan rating: %w", err)
		}
		r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt)
		if r.CreatedAt.IsZero() {
			r.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
		}
		if r.CreatedAt.IsZero() {
			r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		}
		ratings = append(ratings, r)
	}

	if ratings == nil {
		ratings = []Rating{}
	}

	return ratings, rows.Err()
}
