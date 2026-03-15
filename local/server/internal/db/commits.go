package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// UserInfo holds a distinct user name + email pair.
type UserInfo struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Commit represents a row in the commits table.
type Commit struct {
	ID                    string  `json:"id"`
	ProjectID             string  `json:"projectId"`
	BranchName            string  `json:"branchName"`
	CommitHash            string  `json:"commitHash"`
	Subject               string  `json:"subject"`
	UserName              string  `json:"userName"`
	UserEmail             string  `json:"userEmail"`
	AuthoredAt            int64   `json:"authoredAt"` // unix seconds
	DiffContent           string  `json:"diffContent"`
	LinesTotal            int     `json:"linesTotal"`
	LinesFromAgent        int     `json:"linesFromAgent"`
	LinesAdded            int     `json:"linesAdded"`
	LinesRemoved          int     `json:"linesRemoved"`
	CoverageVersion       int     `json:"coverageVersion"`
	OverrideAgentPercents *string `json:"overrideAgentPercents,omitempty"`
	NeedsParent           bool    `json:"needsParent,omitempty"`
	Ignored               bool    `json:"ignored,omitempty"`
	DetailFiles           string  `json:"detailFiles,omitempty"`
	DetailMessages        string  `json:"detailMessages,omitempty"`
	DetailAgentSegments   string  `json:"detailAgentSegments,omitempty"`
	DetailExactMatched    int     `json:"detailExactMatched,omitempty"`
	DetailFallbackLines   int     `json:"detailFallbackLines,omitempty"`
}

// UpsertCommit inserts or updates a commit row. On conflict (same project_id + commit_hash),
// it updates branch metadata, diff_content, and coverage fields.
func UpsertCommit(ctx context.Context, db *sql.DB, c Commit) error {
	if c.ID == "" {
		c.ID = newID()
	}
	needsParent := 0
	if c.NeedsParent {
		needsParent = 1
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO commits (id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at, diff_content, lines_total, lines_from_agent, lines_added, lines_removed, coverage_version, needs_parent, detail_files, detail_messages, detail_agent_segments, detail_exact_matched, detail_fallback_lines)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, commit_hash) DO UPDATE SET
		   branch_name = excluded.branch_name,
		   subject = excluded.subject,
		   user_name = excluded.user_name,
		   user_email = excluded.user_email,
		   diff_content = excluded.diff_content,
		   lines_total = excluded.lines_total,
		   lines_from_agent = excluded.lines_from_agent,
		   lines_added = excluded.lines_added,
		   lines_removed = excluded.lines_removed,
		   coverage_version = excluded.coverage_version,
		   needs_parent = excluded.needs_parent,
		   detail_files = excluded.detail_files,
		   detail_messages = excluded.detail_messages,
		   detail_agent_segments = excluded.detail_agent_segments,
		   detail_exact_matched = excluded.detail_exact_matched,
		   detail_fallback_lines = excluded.detail_fallback_lines`,
		c.ID, c.ProjectID, c.BranchName, c.CommitHash, c.Subject, c.UserName, c.UserEmail, c.AuthoredAt,
		c.DiffContent, c.LinesTotal, c.LinesFromAgent, c.LinesAdded, c.LinesRemoved, c.CoverageVersion, needsParent,
		c.DetailFiles, c.DetailMessages, c.DetailAgentSegments, c.DetailExactMatched, c.DetailFallbackLines,
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
		`INSERT INTO commits (id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at, diff_content, lines_total, lines_from_agent, lines_added, lines_removed, coverage_version, needs_parent, detail_files, detail_messages, detail_agent_segments, detail_exact_matched, detail_fallback_lines)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, commit_hash) DO UPDATE SET
		   branch_name = excluded.branch_name,
		   subject = excluded.subject,
		   user_name = excluded.user_name,
		   user_email = excluded.user_email,
		   diff_content = excluded.diff_content,
		   lines_total = excluded.lines_total,
		   lines_from_agent = excluded.lines_from_agent,
		   lines_added = excluded.lines_added,
		   lines_removed = excluded.lines_removed,
		   coverage_version = excluded.coverage_version,
		   needs_parent = excluded.needs_parent,
		   detail_files = excluded.detail_files,
		   detail_messages = excluded.detail_messages,
		   detail_agent_segments = excluded.detail_agent_segments,
		   detail_exact_matched = excluded.detail_exact_matched,
		   detail_fallback_lines = excluded.detail_fallback_lines`,
	)
	if err != nil {
		return fmt.Errorf("prepare upsert commit: %w", err)
	}
	defer stmt.Close()

	for _, c := range commits {
		if c.ID == "" {
			c.ID = newID()
		}
		needsParent := 0
		if c.NeedsParent {
			needsParent = 1
		}
		if _, err := stmt.ExecContext(ctx,
			c.ID, c.ProjectID, c.BranchName, c.CommitHash, c.Subject, c.UserName, c.UserEmail, c.AuthoredAt,
			c.DiffContent, c.LinesTotal, c.LinesFromAgent, c.LinesAdded, c.LinesRemoved, c.CoverageVersion, needsParent,
			c.DetailFiles, c.DetailMessages, c.DetailAgentSegments, c.DetailExactMatched, c.DetailFallbackLines,
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
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        lines_total, lines_from_agent, lines_added, lines_removed, coverage_version, override_agent_percents, needs_parent, ignored
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
		var olp sql.NullString
		var np, ig int
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.LinesFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion, &olp, &np, &ig); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		if olp.Valid {
			c.OverrideAgentPercents = &olp.String
		}
		c.NeedsParent = np != 0
		c.Ignored = ig != 0
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

// ListDistinctUsers returns distinct user name/email pairs for a project and branch.
func ListDistinctUsers(ctx context.Context, db *sql.DB, projectID, branchName string) ([]UserInfo, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT user_name, user_email FROM commits
		 WHERE project_id = ? AND branch_name = ?
		 GROUP BY user_email
		 ORDER BY user_name`,
		projectID, branchName,
	)
	if err != nil {
		return nil, fmt.Errorf("list distinct users: %w", err)
	}
	defer rows.Close()

	users := []UserInfo{}
	for rows.Next() {
		var u UserInfo
		if err := rows.Scan(&u.Name, &u.Email); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// ListCommitsByProjectAndUser returns commits filtered by an optional user email.
// When userEmail is empty it delegates to ListCommitsByProject.
func ListCommitsByProjectAndUser(ctx context.Context, db *sql.DB, projectID, branchName, userEmail string, limit, offset int) ([]Commit, error) {
	if userEmail == "" {
		return ListCommitsByProject(ctx, db, projectID, branchName, limit, offset)
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        lines_total, lines_from_agent, lines_added, lines_removed, coverage_version, override_agent_percents, needs_parent, ignored
		 FROM commits
		 WHERE project_id = ? AND branch_name = ? AND user_email = ?
		 ORDER BY authored_at DESC
		 LIMIT ? OFFSET ?`,
		projectID, branchName, userEmail, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query commits by user: %w", err)
	}
	defer rows.Close()

	commits := []Commit{}
	for rows.Next() {
		var c Commit
		var olp sql.NullString
		var np, ig int
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.LinesFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion, &olp, &np, &ig); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		if olp.Valid {
			c.OverrideAgentPercents = &olp.String
		}
		c.NeedsParent = np != 0
		c.Ignored = ig != 0
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

// CountCommitsByProjectAndUser returns the total count filtered by optional user email.
// When userEmail is empty it delegates to CountCommitsByProject.
func CountCommitsByProjectAndUser(ctx context.Context, db *sql.DB, projectID, branchName, userEmail string) (int, error) {
	if userEmail == "" {
		return CountCommitsByProject(ctx, db, projectID, branchName)
	}
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM commits WHERE project_id = ? AND branch_name = ? AND user_email = ?", projectID, branchName, userEmail).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count commits by user: %w", err)
	}
	return count, nil
}

// ListCommitsByBranchAndUsers returns paginated commits filtered by branch and
// optional user emails, with configurable sort order and date range.
func ListCommitsByBranchAndUsers(ctx context.Context, db *sql.DB, projectID, branchName string, userEmails []string, limit, offset int, orderAsc bool, dateFromSec, dateToSec int64) ([]Commit, error) {
	if limit <= 0 {
		limit = 20
	}
	orderDir := "DESC"
	if orderAsc {
		orderDir = "ASC"
	}
	var clauses []string
	args := []any{projectID, branchName}
	clauses = append(clauses, "project_id = ?", "branch_name = ?")
	if len(userEmails) == 1 {
		clauses = append(clauses, "user_email = ?")
		args = append(args, userEmails[0])
	} else if len(userEmails) > 1 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(userEmails)), ",")
		clauses = append(clauses, fmt.Sprintf("user_email IN (%s)", placeholders))
		for _, e := range userEmails {
			args = append(args, e)
		}
	}
	if dateFromSec > 0 {
		clauses = append(clauses, "authored_at >= ?")
		args = append(args, dateFromSec)
	}
	if dateToSec > 0 {
		clauses = append(clauses, "authored_at < ?")
		args = append(args, dateToSec)
	}
	args = append(args, limit, offset)
	query := fmt.Sprintf(
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        lines_total, lines_from_agent, lines_added, lines_removed, coverage_version, override_agent_percents, needs_parent, ignored
		 FROM commits
		 WHERE %s
		 ORDER BY authored_at %s
		 LIMIT ? OFFSET ?`,
		strings.Join(clauses, " AND "), orderDir,
	)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query commits by branch and users: %w", err)
	}
	defer rows.Close()
	return scanCommits(rows)
}

// CountCommitsByBranchAndUsers counts commits matching branch and optional user/date filters.
func CountCommitsByBranchAndUsers(ctx context.Context, db *sql.DB, projectID, branchName string, userEmails []string, dateFromSec, dateToSec int64) (int, error) {
	var clauses []string
	args := []any{projectID, branchName}
	clauses = append(clauses, "project_id = ?", "branch_name = ?")
	if len(userEmails) == 1 {
		clauses = append(clauses, "user_email = ?")
		args = append(args, userEmails[0])
	} else if len(userEmails) > 1 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(userEmails)), ",")
		clauses = append(clauses, fmt.Sprintf("user_email IN (%s)", placeholders))
		for _, e := range userEmails {
			args = append(args, e)
		}
	}
	if dateFromSec > 0 {
		clauses = append(clauses, "authored_at >= ?")
		args = append(args, dateFromSec)
	}
	if dateToSec > 0 {
		clauses = append(clauses, "authored_at < ?")
		args = append(args, dateToSec)
	}
	var count int
	err := db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM commits WHERE %s", strings.Join(clauses, " AND ")),
		args...,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count commits by branch and users: %w", err)
	}
	return count, nil
}

// ListDistinctBranches returns the distinct branch names for a project.
func ListDistinctBranches(ctx context.Context, db *sql.DB, projectID string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT branch_name FROM commits WHERE project_id = ? ORDER BY branch_name`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list distinct branches: %w", err)
	}
	defer rows.Close()
	var branches []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, fmt.Errorf("scan branch: %w", err)
		}
		branches = append(branches, b)
	}
	return branches, rows.Err()
}

// HasStaleCommitCoverageByBranch reports whether any commit on a branch has
// stale coverage (version < minVersion or missing diff).
func HasStaleCommitCoverageByBranch(ctx context.Context, db *sql.DB, projectID, branchName string, minVersion int) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM commits
		 WHERE project_id = ? AND branch_name = ?
		   AND (coverage_version < ? OR (lines_total > 0 AND trim(diff_content) = '') OR (lines_from_agent > 0 AND trim(detail_files) = ''))`,
		projectID, branchName, minVersion,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count stale commit coverage: %w", err)
	}
	return count > 0, nil
}

// ListDistinctAgentsByBranch returns distinct agent names for commits on a branch.
func ListDistinctAgentsByBranch(ctx context.Context, db *sql.DB, projectID, branchName string, userEmails []string, dateFromSec, dateToSec int64) ([]string, error) {
	var clauses []string
	args := []any{projectID, branchName}
	clauses = append(clauses, "c.project_id = ?", "c.branch_name = ?")
	if len(userEmails) == 1 {
		clauses = append(clauses, "c.user_email = ?")
		args = append(args, userEmails[0])
	} else if len(userEmails) > 1 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(userEmails)), ",")
		clauses = append(clauses, fmt.Sprintf("c.user_email IN (%s)", placeholders))
		for _, e := range userEmails {
			args = append(args, e)
		}
	}
	if dateFromSec > 0 {
		clauses = append(clauses, "c.authored_at >= ?")
		args = append(args, dateFromSec)
	}
	if dateToSec > 0 {
		clauses = append(clauses, "c.authored_at < ?")
		args = append(args, dateToSec)
	}
	query := fmt.Sprintf(
		`SELECT DISTINCT cac.agent FROM commit_agent_coverage cac
		 JOIN commits c ON c.id = cac.commit_id
		 WHERE %s
		 ORDER BY cac.agent`,
		strings.Join(clauses, " AND "),
	)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list distinct agents by branch: %w", err)
	}
	defer rows.Close()
	var agents []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// scanCommits scans commit rows into a slice.
func scanCommits(rows *sql.Rows) ([]Commit, error) {
	commits := []Commit{}
	for rows.Next() {
		var c Commit
		var olp sql.NullString
		var np, ig int
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.LinesFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion, &olp, &np, &ig); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		if olp.Valid {
			c.OverrideAgentPercents = &olp.String
		}
		c.NeedsParent = np != 0
		c.Ignored = ig != 0
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

// GetCommitByHash returns a single commit by project ID and commit hash.
func GetCommitByHash(ctx context.Context, db *sql.DB, projectID, branchName, commitHash string) (*Commit, error) {
	var c Commit
	var olp sql.NullString
	var np, ig int
	err := db.QueryRowContext(ctx,
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        diff_content, lines_total, lines_from_agent, lines_added, lines_removed, coverage_version, override_agent_percents, needs_parent, ignored,
		        detail_files, detail_messages, detail_agent_segments, detail_exact_matched, detail_fallback_lines
		 FROM commits WHERE project_id = ? AND branch_name = ? AND commit_hash = ?`,
		projectID, branchName, commitHash,
	).Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
		&c.DiffContent, &c.LinesTotal, &c.LinesFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion, &olp, &np, &ig,
		&c.DetailFiles, &c.DetailMessages, &c.DetailAgentSegments, &c.DetailExactMatched, &c.DetailFallbackLines)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get commit: %w", err)
	}
	if olp.Valid {
		c.OverrideAgentPercents = &olp.String
	}
	c.NeedsParent = np != 0
	c.Ignored = ig != 0
	return &c, nil
}

// OldestCommitByProject returns the oldest ingested commit for a project (by authored_at).
func OldestCommitByProject(ctx context.Context, db *sql.DB, projectID, branchName string) (*Commit, error) {
	var c Commit
	var olp sql.NullString
	var np, ig int
	err := db.QueryRowContext(ctx,
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        diff_content, lines_total, lines_from_agent, lines_added, lines_removed, coverage_version, override_agent_percents, needs_parent, ignored,
		        detail_files, detail_messages, detail_agent_segments, detail_exact_matched, detail_fallback_lines
		 FROM commits WHERE project_id = ? AND branch_name = ?
		 ORDER BY authored_at ASC LIMIT 1`,
		projectID, branchName,
	).Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
		&c.DiffContent, &c.LinesTotal, &c.LinesFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion, &olp, &np, &ig,
		&c.DetailFiles, &c.DetailMessages, &c.DetailAgentSegments, &c.DetailExactMatched, &c.DetailFallbackLines)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("oldest commit: %w", err)
	}
	if olp.Valid {
		c.OverrideAgentPercents = &olp.String
	}
	c.NeedsParent = np != 0
	c.Ignored = ig != 0
	return &c, nil
}

// UpdateCommitCoverage updates the agent coverage fields on a commit.
func UpdateCommitCoverage(ctx context.Context, db *sql.DB, projectID, branchName, commitHash string, linesFromAgent int) error {
	_, err := db.ExecContext(ctx,
		`UPDATE commits SET lines_from_agent = ? WHERE project_id = ? AND branch_name = ? AND commit_hash = ?`,
		linesFromAgent, projectID, branchName, commitHash,
	)
	if err != nil {
		return fmt.Errorf("update commit coverage: %w", err)
	}
	return nil
}

// UpdateCommitIgnored sets or clears the ignored flag on a commit.
func UpdateCommitIgnored(ctx context.Context, db *sql.DB, projectID, commitHash string, ignored bool) error {
	val := 0
	if ignored {
		val = 1
	}
	_, err := db.ExecContext(ctx,
		`UPDATE commits SET ignored = ? WHERE project_id = ? AND commit_hash = ?`,
		val, projectID, commitHash,
	)
	if err != nil {
		return fmt.Errorf("update commit ignored: %w", err)
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
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        lines_total, lines_from_agent, lines_added, lines_removed, coverage_version, override_agent_percents, needs_parent, ignored
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
		var olp sql.NullString
		var np, ig int
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.LinesFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion, &olp, &np, &ig); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		if olp.Valid {
			c.OverrideAgentPercents = &olp.String
		}
		c.NeedsParent = np != 0
		c.Ignored = ig != 0
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

// ListCommitsWithDiffByHashes loads commits (including diff_content) for the
// given project IDs and commit hashes. Only returns rows where
// lines_from_agent > 0 (commits with zero agent attribution can't have
// matching messages). Results are batched by sqliteBatchSize.
func ListCommitsWithDiffByHashes(ctx context.Context, database *sql.DB, projectIDs []string, hashes []string) ([]Commit, error) {
	if len(projectIDs) == 0 || len(hashes) == 0 {
		return nil, nil
	}

	pidPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(projectIDs)), ",")
	var all []Commit

	for i := 0; i < len(hashes); i += sqliteBatchSize {
		end := i + sqliteBatchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		batch := hashes[i:end]
		hashPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		query := fmt.Sprintf(
			`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
			        diff_content, lines_total, lines_from_agent, lines_added, lines_removed, coverage_version, override_agent_percents, needs_parent, ignored,
			        detail_files, detail_messages, detail_agent_segments, detail_exact_matched, detail_fallback_lines
			 FROM commits
			 WHERE project_id IN (%s) AND commit_hash IN (%s) AND lines_from_agent > 0`,
			pidPlaceholders, hashPlaceholders,
		)
		args := make([]any, 0, len(projectIDs)+len(batch))
		for _, pid := range projectIDs {
			args = append(args, pid)
		}
		for _, h := range batch {
			args = append(args, h)
		}

		rows, err := database.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query commits with diff by hashes: %w", err)
		}
		for rows.Next() {
			var c Commit
			var olp sql.NullString
			var np, ig int
			if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
				&c.DiffContent, &c.LinesTotal, &c.LinesFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion, &olp, &np, &ig,
				&c.DetailFiles, &c.DetailMessages, &c.DetailAgentSegments, &c.DetailExactMatched, &c.DetailFallbackLines); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan commit with diff: %w", err)
			}
			if olp.Valid {
				c.OverrideAgentPercents = &olp.String
			}
			c.Ignored = ig != 0
			all = append(all, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	return all, nil
}

// CommitAgentCoverage represents a row in the commit_agent_coverage table.
type CommitAgentCoverage struct {
	ID             string `json:"id"`
	CommitID       string `json:"commitId"`
	Agent          string `json:"agent"`
	LinesFromAgent int    `json:"linesFromAgent"`
}

// UpsertCommitAgentCoverage batch-upserts per-agent coverage rows.
func UpsertCommitAgentCoverage(ctx context.Context, database *sql.DB, rows []CommitAgentCoverage) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO commit_agent_coverage (id, commit_id, agent, lines_from_agent)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(commit_id, agent) DO UPDATE SET
		   lines_from_agent = excluded.lines_from_agent`,
	)
	if err != nil {
		return fmt.Errorf("prepare upsert commit_agent_coverage: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		if r.ID == "" {
			r.ID = newID()
		}
		if _, err := stmt.ExecContext(ctx, r.ID, r.CommitID, r.Agent, r.LinesFromAgent); err != nil {
			return fmt.Errorf("upsert commit_agent_coverage: %w", err)
		}
	}

	return tx.Commit()
}

// ListCommitAgentCoverageByCommitIDs bulk-fetches per-agent coverage keyed by commit ID.
func ListCommitAgentCoverageByCommitIDs(ctx context.Context, database *sql.DB, commitIDs []string) (map[string][]CommitAgentCoverage, error) {
	if len(commitIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(commitIDs)), ",")
	query := fmt.Sprintf(
		`SELECT id, commit_id, agent, lines_from_agent
		 FROM commit_agent_coverage
		 WHERE commit_id IN (%s)
		 ORDER BY commit_id, agent`,
		placeholders,
	)
	args := make([]any, 0, len(commitIDs))
	for _, id := range commitIDs {
		args = append(args, id)
	}

	dbRows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query commit_agent_coverage: %w", err)
	}
	defer dbRows.Close()

	result := make(map[string][]CommitAgentCoverage)
	for dbRows.Next() {
		var r CommitAgentCoverage
		if err := dbRows.Scan(&r.ID, &r.CommitID, &r.Agent, &r.LinesFromAgent); err != nil {
			return nil, fmt.Errorf("scan commit_agent_coverage: %w", err)
		}
		result[r.CommitID] = append(result[r.CommitID], r)
	}
	return result, dbRows.Err()
}

// ListDistinctAgentsByCommitIDs returns the distinct agent names across the given commit IDs.
func ListDistinctAgentsByCommitIDs(ctx context.Context, database *sql.DB, commitIDs []string) ([]string, error) {
	if len(commitIDs) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool)
	var agents []string
	for i := 0; i < len(commitIDs); i += sqliteBatchSize {
		end := i + sqliteBatchSize
		if end > len(commitIDs) {
			end = len(commitIDs)
		}
		batch := commitIDs[i:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		query := fmt.Sprintf(
			`SELECT DISTINCT agent FROM commit_agent_coverage WHERE commit_id IN (%s) ORDER BY agent`,
			placeholders,
		)
		args := make([]any, 0, len(batch))
		for _, id := range batch {
			args = append(args, id)
		}
		rows, err := database.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("list distinct agents by commit ids: %w", err)
		}
		for rows.Next() {
			var agent string
			if err := rows.Scan(&agent); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan agent: %w", err)
			}
			if !seen[agent] {
				seen[agent] = true
				agents = append(agents, agent)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return agents, nil
}

// ListCommitIDsByAgent returns commit IDs that have coverage from a specific agent.
// When agent is "manual", returns commit IDs where lines_from_agent = 0 in the commits table.
func ListCommitIDsByAgent(ctx context.Context, database *sql.DB, commitIDs []string, agent string) (map[string]bool, error) {
	if len(commitIDs) == 0 {
		return nil, nil
	}
	result := make(map[string]bool)

	if agent == "manual" {
		// Return commit IDs where lines_from_agent = 0.
		for i := 0; i < len(commitIDs); i += sqliteBatchSize {
			end := i + sqliteBatchSize
			if end > len(commitIDs) {
				end = len(commitIDs)
			}
			batch := commitIDs[i:end]
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
			query := fmt.Sprintf(
				`SELECT id FROM commits WHERE id IN (%s) AND lines_from_agent = 0`,
				placeholders,
			)
			args := make([]any, 0, len(batch))
			for _, id := range batch {
				args = append(args, id)
			}
			rows, err := database.QueryContext(ctx, query, args...)
			if err != nil {
				return nil, fmt.Errorf("list manual commit ids: %w", err)
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return nil, fmt.Errorf("scan commit id: %w", err)
				}
				result[id] = true
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return nil, err
			}
		}
		return result, nil
	}

	// Return commit IDs that have a matching agent in commit_agent_coverage.
	for i := 0; i < len(commitIDs); i += sqliteBatchSize - 1 {
		end := i + sqliteBatchSize - 1
		if end > len(commitIDs) {
			end = len(commitIDs)
		}
		batch := commitIDs[i:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		query := fmt.Sprintf(
			`SELECT DISTINCT commit_id FROM commit_agent_coverage WHERE commit_id IN (%s) AND agent = ?`,
			placeholders,
		)
		args := make([]any, 0, len(batch)+1)
		for _, id := range batch {
			args = append(args, id)
		}
		args = append(args, agent)
		rows, err := database.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("list commit ids by agent: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan commit id: %w", err)
			}
			result[id] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// DeleteCommitAgentCoverageByCommitID removes all per-agent coverage rows for a commit.
func DeleteCommitAgentCoverageByCommitID(ctx context.Context, database *sql.DB, commitID string) error {
	if strings.TrimSpace(commitID) == "" {
		return nil
	}
	res, err := database.ExecContext(ctx, "DELETE FROM commit_agent_coverage WHERE commit_id = ?", commitID)
	if err != nil {
		return fmt.Errorf("delete commit_agent_coverage: %w", err)
	}
	deletedRows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("commit agent coverage rows affected: %w", err)
	}
	if err := runIncrementalVacuum(ctx, database, deletedRows); err != nil {
		return err
	}
	return nil
}

// ResetCoverageVersionByDateRange sets coverage_version = 0 for all commits
// in a project+branch within the given authored_at range (unix seconds).
// If sinceSec is 0, all commits on the branch are reset.
func ResetCoverageVersionByDateRange(ctx context.Context, database *sql.DB, projectID, branchName string, sinceSec int64) (int64, error) {
	var result sql.Result
	var err error
	if sinceSec > 0 {
		result, err = database.ExecContext(ctx,
			`UPDATE commits SET coverage_version = 0
			 WHERE project_id = ? AND branch_name = ? AND authored_at >= ?`,
			projectID, branchName, sinceSec,
		)
	} else {
		result, err = database.ExecContext(ctx,
			`UPDATE commits SET coverage_version = 0
			 WHERE project_id = ? AND branch_name = ?`,
			projectID, branchName,
		)
	}
	if err != nil {
		return 0, fmt.Errorf("reset coverage version: %w", err)
	}
	return result.RowsAffected()
}

// HasStaleCommitCoverage reports whether any commit in a project+branch has a
// coverage_version lower than minVersion.
func HasStaleCommitCoverage(ctx context.Context, database *sql.DB, projectID, branchName string, minVersion int) (bool, error) {
	var count int
	err := database.QueryRowContext(ctx,
		`SELECT COUNT(*)
		 FROM commits
		 WHERE project_id = ?
		   AND branch_name = ?
		   AND (
		     coverage_version < ?
		     OR (lines_total > 0 AND trim(diff_content) = '')
		     OR (lines_from_agent > 0 AND trim(detail_files) = '')
		   )`,
		projectID, branchName, minVersion,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count stale commit coverage: %w", err)
	}
	return count > 0, nil
}

// sqliteBatchSize is the maximum number of placeholders in a single IN clause
// to stay within SQLite's limit of 999 bound parameters.
const sqliteBatchSize = 999

// GetCommitByProjectAndHash returns a single commit by project ID and commit hash,
// without filtering by branch.
func GetCommitByProjectAndHash(ctx context.Context, db *sql.DB, projectID, commitHash string) (*Commit, error) {
	var c Commit
	var olp sql.NullString
	var np, ig int
	err := db.QueryRowContext(ctx,
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        diff_content, lines_total, lines_from_agent, lines_added, lines_removed, coverage_version, override_agent_percents, needs_parent, ignored,
		        detail_files, detail_messages, detail_agent_segments, detail_exact_matched, detail_fallback_lines
		 FROM commits WHERE project_id = ? AND commit_hash = ?`,
		projectID, commitHash,
	).Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
		&c.DiffContent, &c.LinesTotal, &c.LinesFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion, &olp, &np, &ig,
		&c.DetailFiles, &c.DetailMessages, &c.DetailAgentSegments, &c.DetailExactMatched, &c.DetailFallbackLines)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get commit by project and hash: %w", err)
	}
	if olp.Valid {
		c.OverrideAgentPercents = &olp.String
	}
	c.NeedsParent = np != 0
	c.Ignored = ig != 0
	return &c, nil
}

// ListCommitsByHashes returns commits matching the given hashes for a project,
// ordered by authored_at DESC. DiffContent is omitted.
func ListCommitsByHashes(ctx context.Context, db *sql.DB, projectID string, hashes []string, limit, offset int) ([]Commit, error) {
	return ListCommitsByHashesOrdered(ctx, db, projectID, hashes, limit, offset, false)
}

// ListCommitsByHashesOrdered returns commits matching the given hashes for a project.
// When orderAsc is true, results are ordered oldest first; otherwise newest first.
func ListCommitsByHashesOrdered(ctx context.Context, db *sql.DB, projectID string, hashes []string, limit, offset int, orderAsc bool) ([]Commit, error) {
	if len(hashes) == 0 {
		return []Commit{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	// For small hash lists, use a single query.
	if len(hashes) <= sqliteBatchSize {
		return listCommitsByHashesSingle(ctx, db, projectID, hashes, nil, limit, offset, orderAsc)
	}

	// For large hash lists, batch and merge in Go.
	return listCommitsByHashesBatched(ctx, db, projectID, hashes, nil, sqliteBatchSize, limit, offset, orderAsc)
}

// ListCommitsByHashesAndUser returns commits matching hashes filtered by user emails.
// When userEmails is empty, no user filter is applied.
func ListCommitsByHashesAndUser(ctx context.Context, db *sql.DB, projectID string, hashes []string, userEmails []string, limit, offset int) ([]Commit, error) {
	return ListCommitsByHashesAndUserOrdered(ctx, db, projectID, hashes, userEmails, limit, offset, false)
}

// ListCommitsByHashesAndUserOrdered returns commits matching hashes filtered by user emails.
// When orderAsc is true, results are ordered oldest first; otherwise newest first.
func ListCommitsByHashesAndUserOrdered(ctx context.Context, db *sql.DB, projectID string, hashes []string, userEmails []string, limit, offset int, orderAsc bool) ([]Commit, error) {
	if len(userEmails) == 0 {
		return ListCommitsByHashesOrdered(ctx, db, projectID, hashes, limit, offset, orderAsc)
	}
	if len(hashes) == 0 {
		return []Commit{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	// Leave room for email params within SQLite's 999 param limit.
	hashBatchSize := sqliteBatchSize - len(userEmails) - 1 // -1 for projectID
	if hashBatchSize < 1 {
		hashBatchSize = 1
	}

	if len(hashes) <= hashBatchSize {
		return listCommitsByHashesSingle(ctx, db, projectID, hashes, userEmails, limit, offset, orderAsc)
	}
	return listCommitsByHashesBatched(ctx, db, projectID, hashes, userEmails, hashBatchSize, limit, offset, orderAsc)
}

func listCommitsByHashesSingle(ctx context.Context, db *sql.DB, projectID string, hashes []string, userEmails []string, limit, offset int, orderAsc bool) ([]Commit, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(hashes)), ",")
	userClause := ""
	if len(userEmails) == 1 {
		userClause = " AND user_email = ?"
	} else if len(userEmails) > 1 {
		userClause = " AND user_email IN (" + strings.TrimSuffix(strings.Repeat("?,", len(userEmails)), ",") + ")"
	}
	orderDir := "DESC"
	if orderAsc {
		orderDir = "ASC"
	}
	query := fmt.Sprintf(
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        lines_total, lines_from_agent, lines_added, lines_removed, coverage_version, override_agent_percents, needs_parent, ignored
		 FROM commits
		 WHERE project_id = ? AND commit_hash IN (%s)%s
		 ORDER BY authored_at %s
		 LIMIT ? OFFSET ?`,
		placeholders, userClause, orderDir,
	)
	args := make([]any, 0, 1+len(hashes)+len(userEmails)+2)
	args = append(args, projectID)
	for _, h := range hashes {
		args = append(args, h)
	}
	for _, e := range userEmails {
		args = append(args, e)
	}
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query commits by hashes: %w", err)
	}
	defer rows.Close()

	commits := []Commit{}
	for rows.Next() {
		var c Commit
		var olp sql.NullString
		var np, ig int
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.LinesFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion, &olp, &np, &ig); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		if olp.Valid {
			c.OverrideAgentPercents = &olp.String
		}
		c.NeedsParent = np != 0
		c.Ignored = ig != 0
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

func listCommitsByHashesBatched(ctx context.Context, db *sql.DB, projectID string, hashes []string, userEmails []string, hashBatchSize, limit, offset int, orderAsc bool) ([]Commit, error) {
	// Collect all matching commits across batches, then sort and paginate in Go.
	var all []Commit
	for i := 0; i < len(hashes); i += hashBatchSize {
		end := i + hashBatchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		batch, err := listCommitsByHashesSingle(ctx, db, projectID, hashes[i:end], userEmails, end-i, 0, orderAsc)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
	}

	// Sort by authored_at.
	if orderAsc {
		sortCommitsAsc(all)
	} else {
		sortCommitsDesc(all)
	}

	// Apply offset and limit.
	if offset >= len(all) {
		return []Commit{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}

func sortCommitsDesc(commits []Commit) {
	for i := 1; i < len(commits); i++ {
		for j := i; j > 0 && commits[j].AuthoredAt > commits[j-1].AuthoredAt; j-- {
			commits[j], commits[j-1] = commits[j-1], commits[j]
		}
	}
}

func sortCommitsAsc(commits []Commit) {
	for i := 1; i < len(commits); i++ {
		for j := i; j > 0 && commits[j].AuthoredAt < commits[j-1].AuthoredAt; j-- {
			commits[j], commits[j-1] = commits[j-1], commits[j]
		}
	}
}

// CountCommitsByHashes returns the count of commits matching hashes for a project.
func CountCommitsByHashes(ctx context.Context, db *sql.DB, projectID string, hashes []string) (int, error) {
	if len(hashes) == 0 {
		return 0, nil
	}
	total := 0
	for i := 0; i < len(hashes); i += sqliteBatchSize {
		end := i + sqliteBatchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		batch := hashes[i:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		query := fmt.Sprintf("SELECT COUNT(*) FROM commits WHERE project_id = ? AND commit_hash IN (%s)", placeholders)
		args := make([]any, 0, 1+len(batch))
		args = append(args, projectID)
		for _, h := range batch {
			args = append(args, h)
		}
		var count int
		if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			return 0, fmt.Errorf("count commits by hashes: %w", err)
		}
		total += count
	}
	return total, nil
}

// CountCommitsByHashesAndUser returns the count of commits matching hashes and user emails.
// When userEmails is empty, no user filter is applied.
func CountCommitsByHashesAndUser(ctx context.Context, db *sql.DB, projectID string, hashes []string, userEmails []string) (int, error) {
	if len(userEmails) == 0 {
		return CountCommitsByHashes(ctx, db, projectID, hashes)
	}
	if len(hashes) == 0 {
		return 0, nil
	}
	hashBatchSize := sqliteBatchSize - len(userEmails) - 1
	if hashBatchSize < 1 {
		hashBatchSize = 1
	}
	total := 0
	for i := 0; i < len(hashes); i += hashBatchSize {
		end := i + hashBatchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		batch := hashes[i:end]
		hashPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		userClause := ""
		if len(userEmails) == 1 {
			userClause = " AND user_email = ?"
		} else {
			userClause = " AND user_email IN (" + strings.TrimSuffix(strings.Repeat("?,", len(userEmails)), ",") + ")"
		}
		query := fmt.Sprintf("SELECT COUNT(*) FROM commits WHERE project_id = ? AND commit_hash IN (%s)%s", hashPlaceholders, userClause)
		args := make([]any, 0, 1+len(batch)+len(userEmails))
		args = append(args, projectID)
		for _, h := range batch {
			args = append(args, h)
		}
		for _, e := range userEmails {
			args = append(args, e)
		}
		var count int
		if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			return 0, fmt.Errorf("count commits by hashes and user: %w", err)
		}
		total += count
	}
	return total, nil
}

// ExistingCommitHashes returns a set of commit hashes that already exist in the DB for a project.
func ExistingCommitHashes(ctx context.Context, db *sql.DB, projectID string, hashes []string) (map[string]bool, error) {
	if len(hashes) == 0 {
		return map[string]bool{}, nil
	}
	result := make(map[string]bool, len(hashes))
	for i := 0; i < len(hashes); i += sqliteBatchSize {
		end := i + sqliteBatchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		batch := hashes[i:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		query := fmt.Sprintf("SELECT commit_hash FROM commits WHERE project_id = ? AND commit_hash IN (%s)", placeholders)
		args := make([]any, 0, 1+len(batch))
		args = append(args, projectID)
		for _, h := range batch {
			args = append(args, h)
		}
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("existing commit hashes: %w", err)
		}
		for rows.Next() {
			var hash string
			if err := rows.Scan(&hash); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan commit hash: %w", err)
			}
			result[hash] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ListDistinctUsersByHashes returns distinct user name/email pairs for commits matching hashes.
func ListDistinctUsersByHashes(ctx context.Context, db *sql.DB, projectID string, hashes []string) ([]UserInfo, error) {
	if len(hashes) == 0 {
		return []UserInfo{}, nil
	}
	seen := make(map[string]bool)
	var users []UserInfo
	for i := 0; i < len(hashes); i += sqliteBatchSize {
		end := i + sqliteBatchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		batch := hashes[i:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		query := fmt.Sprintf(
			`SELECT user_name, user_email FROM commits
			 WHERE project_id = ? AND commit_hash IN (%s)
			 GROUP BY user_email
			 ORDER BY user_name`,
			placeholders,
		)
		args := make([]any, 0, 1+len(batch))
		args = append(args, projectID)
		for _, h := range batch {
			args = append(args, h)
		}
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("list distinct users by hashes: %w", err)
		}
		for rows.Next() {
			var u UserInfo
			if err := rows.Scan(&u.Name, &u.Email); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan user: %w", err)
			}
			if !seen[u.Email] {
				seen[u.Email] = true
				users = append(users, u)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return users, nil
}

// HasStaleCommitCoverageByHashes reports whether any commit matching the given
// hashes has a coverage_version lower than minVersion.
func HasStaleCommitCoverageByHashes(ctx context.Context, database *sql.DB, projectID string, hashes []string, minVersion int) (bool, error) {
	if len(hashes) == 0 {
		return false, nil
	}
	for i := 0; i < len(hashes); i += sqliteBatchSize {
		end := i + sqliteBatchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		batch := hashes[i:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		query := fmt.Sprintf(
			`SELECT COUNT(*)
			 FROM commits
			 WHERE project_id = ?
			   AND commit_hash IN (%s)
			   AND (
			     coverage_version < ?
			     OR (lines_total > 0 AND trim(diff_content) = '')
			     OR (lines_from_agent > 0 AND trim(detail_files) = '')
			   )`,
			placeholders,
		)
		args := make([]any, 0, 2+len(batch))
		args = append(args, projectID)
		for _, h := range batch {
			args = append(args, h)
		}
		args = append(args, minVersion)
		var count int
		if err := database.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			return false, fmt.Errorf("count stale commit coverage by hashes: %w", err)
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

// SetCommitOverrideAgentPercents sets or clears the per-agent override percentages
// on a commit. Pass nil or empty map to clear.
func SetCommitOverrideAgentPercents(ctx context.Context, db *sql.DB, projectID, commitHash string, override map[string]int) error {
	var val any
	if len(override) > 0 {
		b, err := json.Marshal(override)
		if err != nil {
			return fmt.Errorf("marshal override agent percents: %w", err)
		}
		val = string(b)
	}
	res, err := db.ExecContext(ctx,
		"UPDATE commits SET override_agent_percents = ? WHERE project_id = ? AND commit_hash = ?",
		val, projectID, commitHash,
	)
	if err != nil {
		return fmt.Errorf("set commit override agent percents: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("commit %s in project %s: %w", commitHash, projectID, ErrNotFound)
	}
	return nil
}

// ListNeedsParentCommitHashes returns commit hashes that have needs_parent=1 for a project.
func ListNeedsParentCommitHashes(ctx context.Context, db *sql.DB, projectID string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT commit_hash FROM commits WHERE project_id = ? AND needs_parent = 1`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list needs_parent commit hashes: %w", err)
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, fmt.Errorf("scan commit hash: %w", err)
		}
		hashes = append(hashes, h)
	}
	return hashes, rows.Err()
}

// UpsertCommitConversationLinks replaces the conversation links for a commit.
func UpsertCommitConversationLinks(ctx context.Context, database *sql.DB, commitID string, conversationIDs []string) error {
	if strings.TrimSpace(commitID) == "" {
		return nil
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM commit_conversation_links WHERE commit_id = ?", commitID); err != nil {
		return fmt.Errorf("delete old commit conversation links: %w", err)
	}

	if len(conversationIDs) == 0 {
		return tx.Commit()
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO commit_conversation_links (commit_id, conversation_id) VALUES (?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert commit conversation link: %w", err)
	}
	defer stmt.Close()

	for _, cid := range conversationIDs {
		if _, err := stmt.ExecContext(ctx, commitID, cid); err != nil {
			return fmt.Errorf("insert commit conversation link: %w", err)
		}
	}
	return tx.Commit()
}

// GetCachedCommitConversationLinks returns cached commit-to-conversation mappings
// for commits matching the given project IDs and commit hashes.
func GetCachedCommitConversationLinks(ctx context.Context, database *sql.DB, projectIDs []string, commitHashes []string) (map[string][]string, error) {
	if len(projectIDs) == 0 || len(commitHashes) == 0 {
		return map[string][]string{}, nil
	}

	pidPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(projectIDs)), ",")
	result := make(map[string][]string)

	for i := 0; i < len(commitHashes); i += sqliteBatchSize {
		end := i + sqliteBatchSize
		if end > len(commitHashes) {
			end = len(commitHashes)
		}
		batch := commitHashes[i:end]
		hashPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		query := fmt.Sprintf(
			`SELECT c.commit_hash, ccl.conversation_id
			 FROM commit_conversation_links ccl
			 JOIN commits c ON c.id = ccl.commit_id
			 WHERE c.project_id IN (%s) AND c.commit_hash IN (%s)`,
			pidPlaceholders, hashPlaceholders,
		)
		args := make([]any, 0, len(projectIDs)+len(batch))
		for _, pid := range projectIDs {
			args = append(args, pid)
		}
		for _, h := range batch {
			args = append(args, h)
		}

		rows, err := database.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query cached commit conversation links: %w", err)
		}
		for rows.Next() {
			var commitHash, conversationID string
			if err := rows.Scan(&commitHash, &conversationID); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan commit conversation link: %w", err)
			}
			result[commitHash] = append(result[commitHash], conversationID)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// GetCachedConversationCommitLinks returns cached conversation-to-commit
// mappings for conversations matching the given project IDs.
func GetCachedConversationCommitLinks(ctx context.Context, database *sql.DB, projectIDs []string, conversationIDs []string) (map[string][]string, map[string]string, map[string]string, error) {
	if len(projectIDs) == 0 || len(conversationIDs) == 0 {
		return map[string][]string{}, map[string]string{}, map[string]string{}, nil
	}

	pidPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(projectIDs)), ",")
	conversationToCommits := make(map[string][]string)
	commitBranches := make(map[string]string)
	commitSubjects := make(map[string]string)

	for i := 0; i < len(conversationIDs); i += sqliteBatchSize {
		end := i + sqliteBatchSize
		if end > len(conversationIDs) {
			end = len(conversationIDs)
		}
		batch := conversationIDs[i:end]
		conversationPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		query := fmt.Sprintf(
			`SELECT ccl.conversation_id, c.commit_hash, c.branch_name, c.subject
			 FROM commit_conversation_links ccl
			 JOIN commits c ON c.id = ccl.commit_id
			 WHERE c.project_id IN (%s) AND ccl.conversation_id IN (%s)
			 ORDER BY c.authored_at DESC, c.commit_hash DESC`,
			pidPlaceholders, conversationPlaceholders,
		)
		args := make([]any, 0, len(projectIDs)+len(batch))
		for _, pid := range projectIDs {
			args = append(args, pid)
		}
		for _, id := range batch {
			args = append(args, id)
		}

		rows, err := database.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("query cached conversation commit links: %w", err)
		}
		for rows.Next() {
			var conversationID, commitHash, branchName, subject string
			if err := rows.Scan(&conversationID, &commitHash, &branchName, &subject); err != nil {
				rows.Close()
				return nil, nil, nil, fmt.Errorf("scan conversation commit link: %w", err)
			}
			conversationToCommits[conversationID] = append(conversationToCommits[conversationID], commitHash)
			commitBranches[commitHash] = branchName
			commitSubjects[commitHash] = subject
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, nil, nil, err
		}
	}

	return conversationToCommits, commitBranches, commitSubjects, nil
}

// ListAllCommitsByProject returns ALL commits for a project (no branch filter),
// ordered by authored_at DESC. DiffContent is omitted.
func ListAllCommitsByProject(ctx context.Context, db *sql.DB, projectID string, limit, offset int) ([]Commit, error) {
	if limit <= 0 {
		limit = 10000
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        lines_total, lines_from_agent, lines_added, lines_removed, coverage_version, override_agent_percents, needs_parent, ignored
		 FROM commits
		 WHERE project_id = ?
		 ORDER BY authored_at DESC
		 LIMIT ? OFFSET ?`,
		projectID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query all commits by project: %w", err)
	}
	defer rows.Close()

	commits := []Commit{}
	for rows.Next() {
		var c Commit
		var olp sql.NullString
		var np, ig int
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.LinesFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion, &olp, &np, &ig); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		if olp.Valid {
			c.OverrideAgentPercents = &olp.String
		}
		c.NeedsParent = np != 0
		c.Ignored = ig != 0
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

// ResetStuckCommitSyncStates resets any commit_sync_state rows stuck in
// "running" or "queued" (e.g. from a previous server crash) back to "idle".
// Returns the number of rows affected.
func ResetStuckCommitSyncStates(ctx context.Context, d *sql.DB) (int64, error) {
	res, err := d.ExecContext(ctx, `UPDATE commit_sync_state SET state = 'idle' WHERE state IN ('running', 'queued')`)
	if err != nil {
		return 0, fmt.Errorf("reset stuck commit sync states: %w", err)
	}
	return res.RowsAffected()
}
