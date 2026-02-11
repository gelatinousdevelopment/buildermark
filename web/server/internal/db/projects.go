package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Project represents a row in the projects table.
type Project struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// ProjectDetail is a project with its conversations and their ratings.
type ProjectDetail struct {
	ID            string                     `json:"id"`
	Path          string                     `json:"path"`
	Conversations []ConversationWithRatings  `json:"conversations"`
}

// ConversationWithRatings is a conversation summary including its ratings.
type ConversationWithRatings struct {
	ID      string   `json:"id"`
	Agent   string   `json:"agent"`
	Ratings []Rating `json:"ratings"`
}

// ListProjects returns all projects.
func ListProjects(ctx context.Context, db *sql.DB) ([]Project, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, path FROM projects ORDER BY path")
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Path); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// GetProjectDetail returns a project with all its conversations and each
// conversation's ratings.
func GetProjectDetail(ctx context.Context, db *sql.DB, projectID string) (*ProjectDetail, error) {
	var p ProjectDetail
	err := db.QueryRowContext(ctx, "SELECT id, path FROM projects WHERE id = ?", projectID).Scan(&p.ID, &p.Path)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query project: %w", err)
	}

	// Fetch conversations for this project.
	convRows, err := db.QueryContext(ctx, "SELECT id, agent FROM conversations WHERE project_id = ? ORDER BY id", projectID)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer convRows.Close()

	var convIDs []string
	convMap := map[string]*ConversationWithRatings{}
	p.Conversations = []ConversationWithRatings{}

	for convRows.Next() {
		var c ConversationWithRatings
		if err := convRows.Scan(&c.ID, &c.Agent); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		c.Ratings = []Rating{}
		p.Conversations = append(p.Conversations, c)
		convIDs = append(convIDs, c.ID)
		convMap[c.ID] = &p.Conversations[len(p.Conversations)-1]
	}
	if err := convRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}

	// Fetch ratings for all conversations in this project.
	if len(convIDs) > 0 {
		ratRows, err := db.QueryContext(ctx,
			"SELECT id, conversation_id, rating, note, analysis, created_at FROM ratings WHERE conversation_id IN (SELECT id FROM conversations WHERE project_id = ?) ORDER BY created_at DESC",
			projectID,
		)
		if err != nil {
			return nil, fmt.Errorf("query ratings for project: %w", err)
		}
		defer ratRows.Close()

		for ratRows.Next() {
			var r Rating
			var createdAt string
			if err := ratRows.Scan(&r.ID, &r.ConversationID, &r.Rating, &r.Note, &r.Analysis, &createdAt); err != nil {
				return nil, fmt.Errorf("scan rating: %w", err)
			}
			r.CreatedAt = parseTime(createdAt)
			if c, ok := convMap[r.ConversationID]; ok {
				c.Ratings = append(c.Ratings, r)
			}
		}
		if err := ratRows.Err(); err != nil {
			return nil, fmt.Errorf("iterate ratings: %w", err)
		}
	}

	return &p, nil
}
