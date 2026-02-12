package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// Project represents a row in the projects table.
type Project struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Label   string `json:"label"`
	GitID   string `json:"gitId"`
	Ignored bool   `json:"ignored"`
}

// ProjectDetail is a project with its conversations and their ratings.
type ProjectDetail struct {
	ID            string                    `json:"id"`
	Path          string                    `json:"path"`
	Label         string                    `json:"label"`
	GitID         string                    `json:"gitId"`
	Ignored       bool                      `json:"ignored"`
	Conversations []ConversationWithRatings `json:"conversations"`
}

// ConversationWithRatings is a conversation summary including its ratings.
type ConversationWithRatings struct {
	ID      string   `json:"id"`
	Agent   string   `json:"agent"`
	Title   string   `json:"title"`
	Ratings []Rating `json:"ratings"`
}

// ListProjects returns projects filtered by ignored status.
func ListProjects(ctx context.Context, db *sql.DB, ignored bool) ([]Project, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, path, label, git_id, ignored FROM projects WHERE ignored = ? ORDER BY path", ignored)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Path, &p.Label, &p.GitID, &p.Ignored); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// SetProjectIgnored sets the ignored flag on a project.
func SetProjectIgnored(ctx context.Context, db *sql.DB, projectID string, ignored bool) error {
	res, err := db.ExecContext(ctx, "UPDATE projects SET ignored = ? WHERE id = ?", ignored, projectID)
	if err != nil {
		return fmt.Errorf("update project ignored: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("project %s: %w", projectID, ErrNotFound)
	}
	return nil
}

// GetProjectDetail returns a project with all its conversations and each
// conversation's ratings.
func GetProjectDetail(ctx context.Context, db *sql.DB, projectID string) (*ProjectDetail, error) {
	var p ProjectDetail
	err := db.QueryRowContext(ctx, "SELECT id, path, label, git_id, ignored FROM projects WHERE id = ?", projectID).Scan(&p.ID, &p.Path, &p.Label, &p.GitID, &p.Ignored)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query project: %w", err)
	}

	// Fetch conversations for this project.
	convRows, err := db.QueryContext(ctx, "SELECT id, agent, title FROM conversations WHERE project_id = ? ORDER BY id", projectID)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer convRows.Close()

	var convIDs []string
	p.Conversations = []ConversationWithRatings{}

	for convRows.Next() {
		var c ConversationWithRatings
		if err := convRows.Scan(&c.ID, &c.Agent, &c.Title); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		c.Ratings = []Rating{}
		p.Conversations = append(p.Conversations, c)
		convIDs = append(convIDs, c.ID)
	}
	if err := convRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}

	// Build map after the slice is fully populated to avoid stale pointers
	// from slice reallocation during append.
	convMap := make(map[string]*ConversationWithRatings, len(p.Conversations))
	for i := range p.Conversations {
		convMap[p.Conversations[i].ID] = &p.Conversations[i]
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
			r.CreatedAt, err = parseTime(createdAt)
			if err != nil {
				return nil, fmt.Errorf("parse rating created_at %q: %w", createdAt, err)
			}
			if c, ok := convMap[r.ConversationID]; ok {
				c.Ratings = append(c.Ratings, r)
			}
		}
		if err := ratRows.Err(); err != nil {
			return nil, fmt.Errorf("iterate ratings: %w", err)
		}
	}

	sort.SliceStable(p.Conversations, func(i, j int) bool {
		ti := latestRatingTime(p.Conversations[i].Ratings)
		tj := latestRatingTime(p.Conversations[j].Ratings)
		if ti.IsZero() != tj.IsZero() {
			return !ti.IsZero()
		}
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return p.Conversations[i].ID < p.Conversations[j].ID
	})

	return &p, nil
}

// SetProjectLabel sets the label on a project.
func SetProjectLabel(ctx context.Context, db *sql.DB, projectID, label string) error {
	res, err := db.ExecContext(ctx, "UPDATE projects SET label = ? WHERE id = ?", label, projectID)
	if err != nil {
		return fmt.Errorf("update project label: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("project %s: %w", projectID, ErrNotFound)
	}
	return nil
}

// UpdateProjectGitID sets the git_id on a project.
func UpdateProjectGitID(ctx context.Context, db *sql.DB, projectID, gitID string) error {
	res, err := db.ExecContext(ctx, "UPDATE projects SET git_id = ? WHERE id = ?", gitID, projectID)
	if err != nil {
		return fmt.Errorf("update project git_id: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("project %s: %w", projectID, ErrNotFound)
	}
	return nil
}

// ListProjectsWithoutGitID returns all projects that have no git_id set.
func ListProjectsWithoutGitID(ctx context.Context, db *sql.DB) ([]Project, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, path, label, git_id, ignored FROM projects WHERE git_id = ''")
	if err != nil {
		return nil, fmt.Errorf("query projects without git_id: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Path, &p.Label, &p.GitID, &p.Ignored); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func latestRatingTime(ratings []Rating) time.Time {
	var latest time.Time
	for _, r := range ratings {
		if r.CreatedAt.After(latest) {
			latest = r.CreatedAt
		}
	}
	return latest
}
