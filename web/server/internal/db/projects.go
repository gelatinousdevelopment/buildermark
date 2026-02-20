package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// Project represents a row in the projects table.
type Project struct {
	ID                     string `json:"id"`
	Path                   string `json:"path"`
	Label                  string `json:"label"`
	GitID                  string `json:"gitId"`
	DefaultBranch          string `json:"defaultBranch"`
	Remote                 string `json:"remote"`
	Ignored                bool   `json:"ignored"`
	IgnoreDiffPaths        string `json:"ignoreDiffPaths"`
	IgnoreDefaultDiffPaths bool   `json:"ignoreDefaultDiffPaths"`
}

// ProjectDetail is a project with its conversations and their ratings.
type ProjectDetail struct {
	ID                     string                    `json:"id"`
	Path                   string                    `json:"path"`
	Label                  string                    `json:"label"`
	GitID                  string                    `json:"gitId"`
	DefaultBranch          string                    `json:"defaultBranch"`
	Remote                 string                    `json:"remote"`
	Ignored                bool                      `json:"ignored"`
	IgnoreDiffPaths        string                    `json:"ignoreDiffPaths"`
	IgnoreDefaultDiffPaths bool                      `json:"ignoreDefaultDiffPaths"`
	ConversationPagination ConversationPagination    `json:"conversationPagination"`
	Conversations          []ConversationWithRatings `json:"conversations"`
}

// ConversationWithRatings is a conversation summary including its ratings.
type ConversationWithRatings struct {
	ID                   string   `json:"id"`
	Agent                string   `json:"agent"`
	Title                string   `json:"title"`
	LastMessageTimestamp int64    `json:"lastMessageTimestamp"`
	Ratings              []Rating `json:"ratings"`
}

type ConversationPagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// ListProjects returns projects filtered by ignored status.
func ListProjects(ctx context.Context, db *sql.DB, ignored bool) ([]Project, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, path, label, git_id, default_branch, remote, ignored, ignore_diff_paths, ignore_default_diff_paths FROM projects WHERE ignored = ? ORDER BY path", ignored)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Path, &p.Label, &p.GitID, &p.DefaultBranch, &p.Remote, &p.Ignored, &p.IgnoreDiffPaths, &p.IgnoreDefaultDiffPaths); err != nil {
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
	return GetProjectDetailPage(ctx, db, projectID, 1, 0)
}

// GetProjectDetailPage returns a project with conversations sorted by most
// recent message first. If pageSize <= 0, all conversations are returned.
func GetProjectDetailPage(ctx context.Context, db *sql.DB, projectID string, page, pageSize int) (*ProjectDetail, error) {
	var p ProjectDetail
	err := db.QueryRowContext(ctx, "SELECT id, path, label, git_id, default_branch, remote, ignored, ignore_diff_paths, ignore_default_diff_paths FROM projects WHERE id = ?", projectID).Scan(&p.ID, &p.Path, &p.Label, &p.GitID, &p.DefaultBranch, &p.Remote, &p.Ignored, &p.IgnoreDiffPaths, &p.IgnoreDefaultDiffPaths)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query project: %w", err)
	}

	if page < 1 {
		page = 1
	}

	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM conversations WHERE project_id = ?", projectID).Scan(&total); err != nil {
		return nil, fmt.Errorf("count conversations: %w", err)
	}

	if pageSize <= 0 {
		pageSize = total
	}

	totalPages := 0
	if pageSize > 0 && total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	if totalPages > 0 && page > totalPages {
		page = totalPages
	}
	offset := 0
	if pageSize > 0 {
		offset = (page - 1) * pageSize
		if offset < 0 {
			offset = 0
		}
	}
	p.ConversationPagination = ConversationPagination{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}

	limit := pageSize
	if limit <= 0 {
		limit = total
	}

	// Fetch conversations for this project ordered by start time.
	convRows, err := db.QueryContext(ctx,
		`SELECT id, agent, title, started_at
		 FROM conversations
		 WHERE project_id = ?
		 ORDER BY started_at DESC, id DESC
		 LIMIT ? OFFSET ?`,
		projectID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer convRows.Close()

	var convIDs []string
	p.Conversations = []ConversationWithRatings{}

	for convRows.Next() {
		var c ConversationWithRatings
		if err := convRows.Scan(&c.ID, &c.Agent, &c.Title, &c.LastMessageTimestamp); err != nil {
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
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(convIDs)), ",")
		args := make([]any, 0, len(convIDs))
		for _, id := range convIDs {
			args = append(args, id)
		}
		ratRows, err := db.QueryContext(ctx,
			fmt.Sprintf("SELECT id, conversation_id, temp_conversation_id, rating, note, analysis, created_at FROM ratings WHERE conversation_id IN (%s) ORDER BY created_at DESC", placeholders),
			args...,
		)
		if err != nil {
			return nil, fmt.Errorf("query ratings for project: %w", err)
		}
		defer ratRows.Close()

		for ratRows.Next() {
			var r Rating
			var createdAt string
			if err := ratRows.Scan(&r.ID, &r.ConversationID, &r.TempConversationID, &r.Rating, &r.Note, &r.Analysis, &createdAt); err != nil {
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

// UpdateProjectDefaultBranch sets the default branch on a project.
func UpdateProjectDefaultBranch(ctx context.Context, db *sql.DB, projectID, defaultBranch string) error {
	res, err := db.ExecContext(ctx, "UPDATE projects SET default_branch = ? WHERE id = ?", defaultBranch, projectID)
	if err != nil {
		return fmt.Errorf("update project default_branch: %w", err)
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

// SetProjectIgnoreDiffPaths sets ignore_diff_paths on a project.
func SetProjectIgnoreDiffPaths(ctx context.Context, db *sql.DB, projectID, ignoreDiffPaths string) error {
	res, err := db.ExecContext(ctx, "UPDATE projects SET ignore_diff_paths = ? WHERE id = ?", ignoreDiffPaths, projectID)
	if err != nil {
		return fmt.Errorf("update project ignore_diff_paths: %w", err)
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

// SetProjectIgnoreDefaultDiffPaths sets the ignore_default_diff_paths flag on a project.
func SetProjectIgnoreDefaultDiffPaths(ctx context.Context, db *sql.DB, projectID string, ignoreDefaults bool) error {
	res, err := db.ExecContext(ctx, "UPDATE projects SET ignore_default_diff_paths = ? WHERE id = ?", ignoreDefaults, projectID)
	if err != nil {
		return fmt.Errorf("update project ignore_default_diff_paths: %w", err)
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
	rows, err := db.QueryContext(ctx, "SELECT id, path, label, git_id, default_branch, remote, ignored, ignore_diff_paths, ignore_default_diff_paths FROM projects WHERE git_id = ''")
	if err != nil {
		return nil, fmt.Errorf("query projects without git_id: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Path, &p.Label, &p.GitID, &p.DefaultBranch, &p.Remote, &p.Ignored, &p.IgnoreDiffPaths, &p.IgnoreDefaultDiffPaths); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// ListAllProjects returns all projects (used for label backfill).
func ListAllProjects(ctx context.Context, db *sql.DB) ([]Project, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, path, label, git_id, default_branch, remote, ignored, ignore_diff_paths, ignore_default_diff_paths FROM projects")
	if err != nil {
		return nil, fmt.Errorf("query all projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Path, &p.Label, &p.GitID, &p.DefaultBranch, &p.Remote, &p.Ignored, &p.IgnoreDiffPaths, &p.IgnoreDefaultDiffPaths); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// UpdateProjectRemote sets the remote URL on a project.
func UpdateProjectRemote(ctx context.Context, db *sql.DB, projectID, remote string) error {
	res, err := db.ExecContext(ctx, "UPDATE projects SET remote = ? WHERE id = ?", remote, projectID)
	if err != nil {
		return fmt.Errorf("update project remote: %w", err)
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
