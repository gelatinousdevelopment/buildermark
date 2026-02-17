package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Commit represents a row in the commits table.
type Commit struct {
	ID             string `json:"id"`
	ProjectID      string `json:"projectId"`
	BranchName     string `json:"branchName"`
	CommitHash     string `json:"commitHash"`
	Subject        string `json:"subject"`
	AuthorName     string `json:"authorName"`
	AuthorEmail    string `json:"authorEmail"`
	AuthoredAt     int64  `json:"authoredAt"` // unix seconds
	DiffContent    string `json:"diffContent"`
	LinesTotal     int    `json:"linesTotal"`
	CharsTotal     int    `json:"charsTotal"`
	LinesFromAgent int    `json:"linesFromAgent"`
	CharsFromAgent int    `json:"charsFromAgent"`
}

// UpsertCommit inserts or updates a commit row. On conflict (same project_id + branch_name + commit_hash),
// it updates the diff_content and coverage fields.
func UpsertCommit(ctx context.Context, db *sql.DB, c Commit) error {
	if c.ID == "" {
		c.ID = newID()
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO commits (id, project_id, branch_name, commit_hash, subject, author_name, author_email, authored_at, diff_content, lines_total, chars_total, lines_from_agent, chars_from_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, branch_name, commit_hash) DO UPDATE SET
		   subject = excluded.subject,
		   author_name = excluded.author_name,
		   author_email = excluded.author_email,
		   diff_content = excluded.diff_content,
		   lines_total = excluded.lines_total,
		   chars_total = excluded.chars_total,
		   lines_from_agent = excluded.lines_from_agent,
		   chars_from_agent = excluded.chars_from_agent`,
		c.ID, c.ProjectID, c.BranchName, c.CommitHash, c.Subject, c.AuthorName, c.AuthorEmail, c.AuthoredAt,
		c.DiffContent, c.LinesTotal, c.CharsTotal, c.LinesFromAgent, c.CharsFromAgent,
	)
	if err != nil {
		return fmt.Errorf("upsert commit: %w", err)
	}
	return nil
}

// UpsertCommits inserts or updates multiple commits in a single transaction.
func UpsertCommits(ctx context.Context, database *sql.DB, commits []Commit) error {
	if len(commits) == 0 {
		return nil
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO commits (id, project_id, branch_name, commit_hash, subject, author_name, author_email, authored_at, diff_content, lines_total, chars_total, lines_from_agent, chars_from_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, branch_name, commit_hash) DO UPDATE SET
		   subject = excluded.subject,
		   author_name = excluded.author_name,
		   author_email = excluded.author_email,
		   diff_content = excluded.diff_content,
		   lines_total = excluded.lines_total,
		   chars_total = excluded.chars_total,
		   lines_from_agent = excluded.lines_from_agent,
		   chars_from_agent = excluded.chars_from_agent`,
	)
	if err != nil {
		return fmt.Errorf("prepare upsert commit: %w", err)
	}
	defer stmt.Close()

	for _, c := range commits {
		if c.ID == "" {
			c.ID = newID()
		}
		if _, err := stmt.ExecContext(ctx,
			c.ID, c.ProjectID, c.BranchName, c.CommitHash, c.Subject, c.AuthorName, c.AuthorEmail, c.AuthoredAt,
			c.DiffContent, c.LinesTotal, c.CharsTotal, c.LinesFromAgent, c.CharsFromAgent,
		); err != nil {
			return fmt.Errorf("upsert commit %s: %w", c.CommitHash, err)
		}
	}

	return tx.Commit()
}

// ListCommitsByProject returns commit metadata for a project, ordered newest first.
// DiffContent is intentionally omitted to keep list queries lightweight.
func ListCommitsByProject(ctx context.Context, db *sql.DB, projectID, branchName string, limit, offset int) ([]Commit, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, project_id, branch_name, commit_hash, subject, author_name, author_email, authored_at,
		        lines_total, chars_total, lines_from_agent, chars_from_agent
		 FROM commits
		 WHERE project_id = ? AND branch_name = ?
		 ORDER BY authored_at DESC
		 LIMIT ? OFFSET ?`,
		projectID, branchName, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query commits: %w", err)
	}
	defer rows.Close()

	commits := []Commit{}
	for rows.Next() {
		var c Commit
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.AuthorName, &c.AuthorEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.CharsTotal, &c.LinesFromAgent, &c.CharsFromAgent); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

// CountCommitsByProject returns the total number of ingested commits for a project.
func CountCommitsByProject(ctx context.Context, db *sql.DB, projectID, branchName string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM commits WHERE project_id = ? AND branch_name = ?", projectID, branchName).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count commits: %w", err)
	}
	return count, nil
}

// GetCommitByHash returns a single commit by project ID and commit hash.
func GetCommitByHash(ctx context.Context, db *sql.DB, projectID, branchName, commitHash string) (*Commit, error) {
	var c Commit
	err := db.QueryRowContext(ctx,
		`SELECT id, project_id, branch_name, commit_hash, subject, author_name, author_email, authored_at,
		        diff_content, lines_total, chars_total, lines_from_agent, chars_from_agent
		 FROM commits WHERE project_id = ? AND branch_name = ? AND commit_hash = ?`,
		projectID, branchName, commitHash,
	).Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.AuthorName, &c.AuthorEmail, &c.AuthoredAt,
		&c.DiffContent, &c.LinesTotal, &c.CharsTotal, &c.LinesFromAgent, &c.CharsFromAgent)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get commit: %w", err)
	}
	return &c, nil
}

// OldestCommitByProject returns the oldest ingested commit for a project (by authored_at).
func OldestCommitByProject(ctx context.Context, db *sql.DB, projectID, branchName string) (*Commit, error) {
	var c Commit
	err := db.QueryRowContext(ctx,
		`SELECT id, project_id, branch_name, commit_hash, subject, author_name, author_email, authored_at,
		        diff_content, lines_total, chars_total, lines_from_agent, chars_from_agent
		 FROM commits WHERE project_id = ? AND branch_name = ?
		 ORDER BY authored_at ASC LIMIT 1`,
		projectID, branchName,
	).Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.AuthorName, &c.AuthorEmail, &c.AuthoredAt,
		&c.DiffContent, &c.LinesTotal, &c.CharsTotal, &c.LinesFromAgent, &c.CharsFromAgent)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("oldest commit: %w", err)
	}
	return &c, nil
}

// UpdateCommitCoverage updates the agent coverage fields on a commit.
func UpdateCommitCoverage(ctx context.Context, db *sql.DB, projectID, branchName, commitHash string, linesFromAgent, charsFromAgent int) error {
	_, err := db.ExecContext(ctx,
		`UPDATE commits SET lines_from_agent = ?, chars_from_agent = ? WHERE project_id = ? AND branch_name = ? AND commit_hash = ?`,
		linesFromAgent, charsFromAgent, projectID, branchName, commitHash,
	)
	if err != nil {
		return fmt.Errorf("update commit coverage: %w", err)
	}
	return nil
}

// ListCommitsByProjectIDs returns commit metadata for project IDs, ordered oldest first.
// DiffContent is intentionally omitted to keep list queries lightweight.
func ListCommitsByProjectIDs(ctx context.Context, db *sql.DB, projectIDs []string, branchName string) ([]Commit, error) {
	if len(projectIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(projectIDs)), ",")
	query := fmt.Sprintf(
		`SELECT id, project_id, branch_name, commit_hash, subject, author_name, author_email, authored_at,
		        lines_total, chars_total, lines_from_agent, chars_from_agent
		 FROM commits
		 WHERE project_id IN (%s) AND branch_name = ?
		 ORDER BY authored_at ASC`,
		placeholders,
	)
	args := make([]any, 0, len(projectIDs)+1)
	for _, id := range projectIDs {
		args = append(args, id)
	}
	args = append(args, branchName)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query commits by project ids: %w", err)
	}
	defer rows.Close()

	commits := []Commit{}
	for rows.Next() {
		var c Commit
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.AuthorName, &c.AuthorEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.CharsTotal, &c.LinesFromAgent, &c.CharsFromAgent); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		commits = append(commits, c)
	}
	return commits, rows.Err()
}
