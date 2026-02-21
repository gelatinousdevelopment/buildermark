package db

import (
	"context"
	"database/sql"
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
	ID              string `json:"id"`
	ProjectID       string `json:"projectId"`
	BranchName      string `json:"branchName"`
	CommitHash      string `json:"commitHash"`
	Subject         string `json:"subject"`
	UserName        string `json:"userName"`
	UserEmail       string `json:"userEmail"`
	AuthoredAt      int64  `json:"authoredAt"` // unix seconds
	DiffContent     string `json:"diffContent"`
	LinesTotal      int    `json:"linesTotal"`
	CharsTotal      int    `json:"charsTotal"`
	LinesFromAgent  int    `json:"linesFromAgent"`
	CharsFromAgent  int    `json:"charsFromAgent"`
	LinesAdded      int    `json:"linesAdded"`
	LinesRemoved    int    `json:"linesRemoved"`
	CoverageVersion int    `json:"coverageVersion"`
}

// UpsertCommit inserts or updates a commit row. On conflict (same project_id + commit_hash),
// it updates branch metadata, diff_content, and coverage fields.
func UpsertCommit(ctx context.Context, db *sql.DB, c Commit) error {
	if c.ID == "" {
		c.ID = newID()
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO commits (id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at, diff_content, lines_total, chars_total, lines_from_agent, chars_from_agent, lines_added, lines_removed, coverage_version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, commit_hash) DO UPDATE SET
		   branch_name = excluded.branch_name,
		   subject = excluded.subject,
		   user_name = excluded.user_name,
		   user_email = excluded.user_email,
		   diff_content = excluded.diff_content,
		   lines_total = excluded.lines_total,
		   chars_total = excluded.chars_total,
		   lines_from_agent = excluded.lines_from_agent,
		   chars_from_agent = excluded.chars_from_agent,
		   lines_added = excluded.lines_added,
		   lines_removed = excluded.lines_removed,
		   coverage_version = excluded.coverage_version`,
		c.ID, c.ProjectID, c.BranchName, c.CommitHash, c.Subject, c.UserName, c.UserEmail, c.AuthoredAt,
		c.DiffContent, c.LinesTotal, c.CharsTotal, c.LinesFromAgent, c.CharsFromAgent, c.LinesAdded, c.LinesRemoved, c.CoverageVersion,
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
		`INSERT INTO commits (id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at, diff_content, lines_total, chars_total, lines_from_agent, chars_from_agent, lines_added, lines_removed, coverage_version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, commit_hash) DO UPDATE SET
		   branch_name = excluded.branch_name,
		   subject = excluded.subject,
		   user_name = excluded.user_name,
		   user_email = excluded.user_email,
		   diff_content = excluded.diff_content,
		   lines_total = excluded.lines_total,
		   chars_total = excluded.chars_total,
		   lines_from_agent = excluded.lines_from_agent,
		   chars_from_agent = excluded.chars_from_agent,
		   lines_added = excluded.lines_added,
		   lines_removed = excluded.lines_removed,
		   coverage_version = excluded.coverage_version`,
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
			c.ID, c.ProjectID, c.BranchName, c.CommitHash, c.Subject, c.UserName, c.UserEmail, c.AuthoredAt,
			c.DiffContent, c.LinesTotal, c.CharsTotal, c.LinesFromAgent, c.CharsFromAgent, c.LinesAdded, c.LinesRemoved, c.CoverageVersion,
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
		        lines_total, chars_total, lines_from_agent, chars_from_agent, lines_added, lines_removed, coverage_version
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
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.CharsTotal, &c.LinesFromAgent, &c.CharsFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion); err != nil {
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
		        lines_total, chars_total, lines_from_agent, chars_from_agent, lines_added, lines_removed, coverage_version
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
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.CharsTotal, &c.LinesFromAgent, &c.CharsFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
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

// GetCommitByHash returns a single commit by project ID and commit hash.
func GetCommitByHash(ctx context.Context, db *sql.DB, projectID, branchName, commitHash string) (*Commit, error) {
	var c Commit
	err := db.QueryRowContext(ctx,
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        diff_content, lines_total, chars_total, lines_from_agent, chars_from_agent, lines_added, lines_removed, coverage_version
		 FROM commits WHERE project_id = ? AND branch_name = ? AND commit_hash = ?`,
		projectID, branchName, commitHash,
	).Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
		&c.DiffContent, &c.LinesTotal, &c.CharsTotal, &c.LinesFromAgent, &c.CharsFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion)
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
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        diff_content, lines_total, chars_total, lines_from_agent, chars_from_agent, lines_added, lines_removed, coverage_version
		 FROM commits WHERE project_id = ? AND branch_name = ?
		 ORDER BY authored_at ASC LIMIT 1`,
		projectID, branchName,
	).Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
		&c.DiffContent, &c.LinesTotal, &c.CharsTotal, &c.LinesFromAgent, &c.CharsFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion)
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
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        lines_total, chars_total, lines_from_agent, chars_from_agent, lines_added, lines_removed, coverage_version
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
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.CharsTotal, &c.LinesFromAgent, &c.CharsFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

// CommitAgentCoverage represents a row in the commit_agent_coverage table.
type CommitAgentCoverage struct {
	ID             string `json:"id"`
	CommitID       string `json:"commitId"`
	Agent          string `json:"agent"`
	LinesFromAgent int    `json:"linesFromAgent"`
	CharsFromAgent int    `json:"charsFromAgent"`
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
		`INSERT INTO commit_agent_coverage (id, commit_id, agent, lines_from_agent, chars_from_agent)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(commit_id, agent) DO UPDATE SET
		   lines_from_agent = excluded.lines_from_agent,
		   chars_from_agent = excluded.chars_from_agent`,
	)
	if err != nil {
		return fmt.Errorf("prepare upsert commit_agent_coverage: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		if r.ID == "" {
			r.ID = newID()
		}
		if _, err := stmt.ExecContext(ctx, r.ID, r.CommitID, r.Agent, r.LinesFromAgent, r.CharsFromAgent); err != nil {
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
		`SELECT id, commit_id, agent, lines_from_agent, chars_from_agent
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
		if err := dbRows.Scan(&r.ID, &r.CommitID, &r.Agent, &r.LinesFromAgent, &r.CharsFromAgent); err != nil {
			return nil, fmt.Errorf("scan commit_agent_coverage: %w", err)
		}
		result[r.CommitID] = append(result[r.CommitID], r)
	}
	return result, dbRows.Err()
}

// DeleteCommitAgentCoverageByCommitID removes all per-agent coverage rows for a commit.
func DeleteCommitAgentCoverageByCommitID(ctx context.Context, database *sql.DB, commitID string) error {
	if strings.TrimSpace(commitID) == "" {
		return nil
	}
	if _, err := database.ExecContext(ctx, "DELETE FROM commit_agent_coverage WHERE commit_id = ?", commitID); err != nil {
		return fmt.Errorf("delete commit_agent_coverage: %w", err)
	}
	return nil
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
	err := db.QueryRowContext(ctx,
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        diff_content, lines_total, chars_total, lines_from_agent, chars_from_agent, lines_added, lines_removed, coverage_version
		 FROM commits WHERE project_id = ? AND commit_hash = ?`,
		projectID, commitHash,
	).Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
		&c.DiffContent, &c.LinesTotal, &c.CharsTotal, &c.LinesFromAgent, &c.CharsFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get commit by project and hash: %w", err)
	}
	return &c, nil
}

// ListCommitsByHashes returns commits matching the given hashes for a project,
// ordered by authored_at DESC. DiffContent is omitted.
func ListCommitsByHashes(ctx context.Context, db *sql.DB, projectID string, hashes []string, limit, offset int) ([]Commit, error) {
	if len(hashes) == 0 {
		return []Commit{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	// For small hash lists, use a single query.
	if len(hashes) <= sqliteBatchSize {
		return listCommitsByHashesSingle(ctx, db, projectID, hashes, "", limit, offset)
	}

	// For large hash lists, batch and merge in Go.
	return listCommitsByHashesBatched(ctx, db, projectID, hashes, "", limit, offset)
}

// ListCommitsByHashesAndUser returns commits matching hashes filtered by user email.
func ListCommitsByHashesAndUser(ctx context.Context, db *sql.DB, projectID string, hashes []string, userEmail string, limit, offset int) ([]Commit, error) {
	if userEmail == "" {
		return ListCommitsByHashes(ctx, db, projectID, hashes, limit, offset)
	}
	if len(hashes) == 0 {
		return []Commit{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	if len(hashes) <= sqliteBatchSize {
		return listCommitsByHashesSingle(ctx, db, projectID, hashes, userEmail, limit, offset)
	}
	return listCommitsByHashesBatched(ctx, db, projectID, hashes, userEmail, limit, offset)
}

func listCommitsByHashesSingle(ctx context.Context, db *sql.DB, projectID string, hashes []string, userEmail string, limit, offset int) ([]Commit, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(hashes)), ",")
	userClause := ""
	if userEmail != "" {
		userClause = " AND user_email = ?"
	}
	query := fmt.Sprintf(
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        lines_total, chars_total, lines_from_agent, chars_from_agent, lines_added, lines_removed, coverage_version
		 FROM commits
		 WHERE project_id = ? AND commit_hash IN (%s)%s
		 ORDER BY authored_at DESC
		 LIMIT ? OFFSET ?`,
		placeholders, userClause,
	)
	args := make([]any, 0, 1+len(hashes)+2)
	args = append(args, projectID)
	for _, h := range hashes {
		args = append(args, h)
	}
	if userEmail != "" {
		args = append(args, userEmail)
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
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.CharsTotal, &c.LinesFromAgent, &c.CharsFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

func listCommitsByHashesBatched(ctx context.Context, db *sql.DB, projectID string, hashes []string, userEmail string, limit, offset int) ([]Commit, error) {
	// Collect all matching commits across batches, then sort and paginate in Go.
	var all []Commit
	for i := 0; i < len(hashes); i += sqliteBatchSize {
		end := i + sqliteBatchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		batch, err := listCommitsByHashesSingle(ctx, db, projectID, hashes[i:end], userEmail, end-i, 0)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
	}

	// Sort by authored_at DESC.
	sortCommitsDesc(all)

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

// CountCommitsByHashesAndUser returns the count of commits matching hashes and user email.
func CountCommitsByHashesAndUser(ctx context.Context, db *sql.DB, projectID string, hashes []string, userEmail string) (int, error) {
	if userEmail == "" {
		return CountCommitsByHashes(ctx, db, projectID, hashes)
	}
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
		query := fmt.Sprintf("SELECT COUNT(*) FROM commits WHERE project_id = ? AND commit_hash IN (%s) AND user_email = ?", placeholders)
		args := make([]any, 0, 2+len(batch))
		args = append(args, projectID)
		for _, h := range batch {
			args = append(args, h)
		}
		args = append(args, userEmail)
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

// ListAllCommitsByProject returns ALL commits for a project (no branch filter),
// ordered by authored_at DESC. DiffContent is omitted.
func ListAllCommitsByProject(ctx context.Context, db *sql.DB, projectID string, limit, offset int) ([]Commit, error) {
	if limit <= 0 {
		limit = 10000
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, project_id, branch_name, commit_hash, subject, user_name, user_email, authored_at,
		        lines_total, chars_total, lines_from_agent, chars_from_agent, lines_added, lines_removed, coverage_version
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
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.BranchName, &c.CommitHash, &c.Subject, &c.UserName, &c.UserEmail, &c.AuthoredAt,
			&c.LinesTotal, &c.CharsTotal, &c.LinesFromAgent, &c.CharsFromAgent, &c.LinesAdded, &c.LinesRemoved, &c.CoverageVersion); err != nil {
			return nil, fmt.Errorf("scan commit: %w", err)
		}
		commits = append(commits, c)
	}
	return commits, rows.Err()
}
