package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// Project represents a row in the projects table.
type Project struct {
	ID                     string `json:"id"`
	Path                   string `json:"path"`
	OldPaths               string `json:"oldPaths"`
	Label                  string `json:"label"`
	GitID                  string `json:"gitId"`
	DefaultBranch          string `json:"defaultBranch"`
	Remote                 string `json:"remote"`
	LocalUser              string `json:"localUser"`
	LocalEmail             string `json:"localEmail"`
	Ignored                bool   `json:"ignored"`
	IgnoreDiffPaths        string `json:"ignoreDiffPaths"`
	IgnoreDefaultDiffPaths bool   `json:"ignoreDefaultDiffPaths"`
}

// ProjectDetail is a project with its conversations and their ratings.
type ProjectDetail struct {
	ID                     string                    `json:"id"`
	Path                   string                    `json:"path"`
	OldPaths               string                    `json:"oldPaths"`
	Label                  string                    `json:"label"`
	GitID                  string                    `json:"gitId"`
	DefaultBranch          string                    `json:"defaultBranch"`
	Remote                 string                    `json:"remote"`
	LocalUser              string                    `json:"localUser"`
	LocalEmail             string                    `json:"localEmail"`
	Ignored                bool                      `json:"ignored"`
	IgnoreDiffPaths        string                    `json:"ignoreDiffPaths"`
	IgnoreDefaultDiffPaths bool                      `json:"ignoreDefaultDiffPaths"`
	Agents                 []string                  `json:"agents"`
	ConversationPagination ConversationPagination    `json:"conversationPagination"`
	Conversations          []ConversationWithRatings `json:"conversations"`
}

// ConversationWithRatings is a conversation summary including its ratings.
type ConversationWithRatings struct {
	ID                   string   `json:"id"`
	Agent                string   `json:"agent"`
	Title                string   `json:"title"`
	ParentConversationID string   `json:"parentConversationId"`
	LastMessageTimestamp  int64    `json:"lastMessageTimestamp"`
	Ratings              []Rating `json:"ratings"`
	FilesEdited          []string `json:"filesEdited"`
}

// ConversationFilters holds optional filter criteria for conversation queries.
type ConversationFilters struct {
	Agent  string // filter by agent name (empty = all)
	Rating int    // 0 = all, -1 = < 5 stars, 1-5 = exact rating
}

type ConversationPagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// ListProjects returns projects filtered by ignored status.
func ListProjects(ctx context.Context, db *sql.DB, ignored bool) ([]Project, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, path, old_paths, label, git_id, default_branch, remote, local_user, local_email, ignored, ignore_diff_paths, ignore_default_diff_paths FROM projects WHERE ignored = ? ORDER BY path", ignored)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Path, &p.OldPaths, &p.Label, &p.GitID, &p.DefaultBranch, &p.Remote, &p.LocalUser, &p.LocalEmail, &p.Ignored, &p.IgnoreDiffPaths, &p.IgnoreDefaultDiffPaths); err != nil {
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
	return GetProjectDetailPage(ctx, db, projectID, 1, 0, ConversationFilters{})
}

// GetProjectDetailPage returns a project with conversations sorted by most
// recent message first. If pageSize <= 0, all conversations are returned.
// Filters optionally restrict results by agent name and/or rating.
func GetProjectDetailPage(ctx context.Context, db *sql.DB, projectID string, page, pageSize int, filters ConversationFilters) (*ProjectDetail, error) {
	var p ProjectDetail
	err := db.QueryRowContext(ctx, "SELECT id, path, old_paths, label, git_id, default_branch, remote, local_user, local_email, ignored, ignore_diff_paths, ignore_default_diff_paths FROM projects WHERE id = ?", projectID).Scan(&p.ID, &p.Path, &p.OldPaths, &p.Label, &p.GitID, &p.DefaultBranch, &p.Remote, &p.LocalUser, &p.LocalEmail, &p.Ignored, &p.IgnoreDiffPaths, &p.IgnoreDefaultDiffPaths)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query project: %w", err)
	}

	// Fetch distinct agents for filter dropdown.
	agentRows, err := db.QueryContext(ctx, "SELECT DISTINCT agent FROM conversations WHERE project_id = ? ORDER BY agent", projectID)
	if err != nil {
		return nil, fmt.Errorf("query distinct agents: %w", err)
	}
	defer agentRows.Close()
	p.Agents = []string{}
	for agentRows.Next() {
		var a string
		if err := agentRows.Scan(&a); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		p.Agents = append(p.Agents, a)
	}
	if err := agentRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agents: %w", err)
	}

	if page < 1 {
		page = 1
	}

	// Build filter WHERE clauses.
	var filterClauses []string
	var filterArgs []any
	if filters.Agent != "" {
		filterClauses = append(filterClauses, "c.agent = ?")
		filterArgs = append(filterArgs, filters.Agent)
	}
	if filters.Rating == -1 {
		// "< 5 stars" — conversations that have at least one rating but none equal to 5
		filterClauses = append(filterClauses, "c.id IN (SELECT DISTINCT conversation_id FROM ratings) AND c.id NOT IN (SELECT conversation_id FROM ratings WHERE rating = 5)")
	} else if filters.Rating >= 1 && filters.Rating <= 5 {
		filterClauses = append(filterClauses, "c.id IN (SELECT conversation_id FROM ratings WHERE rating = ?)")
		filterArgs = append(filterArgs, filters.Rating)
	}

	filterWhere := ""
	if len(filterClauses) > 0 {
		filterWhere = " AND " + strings.Join(filterClauses, " AND ")
	}

	var total int
	countArgs := append([]any{projectID}, filterArgs...)
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM conversations c WHERE c.project_id = ?"+filterWhere, countArgs...).Scan(&total); err != nil {
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

	// Fetch conversations for this project ordered by latest activity.
	selectArgs := append([]any{projectID}, filterArgs...)
	selectArgs = append(selectArgs, limit, offset)
	convRows, err := db.QueryContext(ctx,
		`SELECT c.id, c.agent, c.title, c.parent_conversation_id, c.ended_at
		 FROM conversations c
		 WHERE c.project_id = ?`+filterWhere+`
		 ORDER BY c.ended_at DESC, c.id DESC
		 LIMIT ? OFFSET ?`,
		selectArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer convRows.Close()

	var convIDs []string
	p.Conversations = []ConversationWithRatings{}

	for convRows.Next() {
		var c ConversationWithRatings
		if err := convRows.Scan(&c.ID, &c.Agent, &c.Title, &c.ParentConversationID, &c.LastMessageTimestamp); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		c.Ratings = []Rating{}
		c.FilesEdited = []string{}
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

	if len(convIDs) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(convIDs)), ",")
		idArgs := make([]any, 0, len(convIDs))
		for _, id := range convIDs {
			idArgs = append(idArgs, id)
		}

		// Fetch ratings for all conversations in this page.
		ratRows, err := db.QueryContext(ctx,
			fmt.Sprintf("SELECT id, conversation_id, temp_conversation_id, rating, note, analysis, created_at FROM ratings WHERE conversation_id IN (%s) ORDER BY created_at DESC, rowid DESC", placeholders),
			idArgs...,
		)
		if err != nil {
			return nil, fmt.Errorf("query ratings for project: %w", err)
		}
		defer ratRows.Close()

		for ratRows.Next() {
			var r Rating
			if err := ratRows.Scan(&r.ID, &r.ConversationID, &r.TempConversationID, &r.Rating, &r.Note, &r.Analysis, &r.CreatedAt); err != nil {
				return nil, fmt.Errorf("scan rating: %w", err)
			}
			if c, ok := convMap[r.ConversationID]; ok {
				c.Ratings = append(c.Ratings, r)
			}
		}
		if err := ratRows.Err(); err != nil {
			return nil, fmt.Errorf("iterate ratings: %w", err)
		}

		// Fetch file paths edited per conversation by parsing diff headers from message content.
		// This covers all agents (Claude's tool use results and Codex's apply_patch).
		diffRe := regexp.MustCompile(`diff --git a/(.+?) b/`)

		fileRows, err := db.QueryContext(ctx,
			fmt.Sprintf(`SELECT conversation_id, content
				FROM messages
				WHERE conversation_id IN (%s)
				  AND content LIKE '%%diff --git a/%%'`, placeholders),
			idArgs...,
		)
		if err != nil {
			return nil, fmt.Errorf("query files edited: %w", err)
		}
		defer fileRows.Close()

		// Collect unique file paths per conversation, stripping the project path prefix.
		projectPrefix := p.Path
		if projectPrefix != "" && !strings.HasSuffix(projectPrefix, "/") {
			projectPrefix += "/"
		}
		for fileRows.Next() {
			var convID, content string
			if err := fileRows.Scan(&convID, &content); err != nil {
				return nil, fmt.Errorf("scan file content: %w", err)
			}
			c, ok := convMap[convID]
			if !ok {
				continue
			}
			for _, match := range diffRe.FindAllStringSubmatch(content, -1) {
				fp := strings.TrimSpace(match[1])
				if fp == "" {
					continue
				}
				if projectPrefix != "" {
					fp = strings.TrimPrefix(fp, projectPrefix)
				}
				// Deduplicate.
				found := false
				for _, existing := range c.FilesEdited {
					if existing == fp {
						found = true
						break
					}
				}
				if !found {
					c.FilesEdited = append(c.FilesEdited, fp)
				}
			}
		}
		if err := fileRows.Err(); err != nil {
			return nil, fmt.Errorf("iterate files edited: %w", err)
		}
	}

	return &p, nil
}

// SetProjectPath sets the path on a project.
func SetProjectPath(ctx context.Context, db *sql.DB, projectID, path string) error {
	res, err := db.ExecContext(ctx, "UPDATE projects SET path = ? WHERE id = ?", path, projectID)
	if err != nil {
		return fmt.Errorf("update project path: %w", err)
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

// GetProjectOldPaths returns old_paths for a project ID.
func GetProjectOldPaths(ctx context.Context, db *sql.DB, projectID string) (string, error) {
	var oldPaths string
	err := db.QueryRowContext(ctx, "SELECT old_paths FROM projects WHERE id = ?", projectID).Scan(&oldPaths)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("project %s: %w", projectID, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("query project old_paths: %w", err)
	}
	return oldPaths, nil
}

// SetProjectOldPaths sets old_paths on a project.
func SetProjectOldPaths(ctx context.Context, db *sql.DB, projectID, oldPaths string) error {
	res, err := db.ExecContext(ctx, "UPDATE projects SET old_paths = ? WHERE id = ?", oldPaths, projectID)
	if err != nil {
		return fmt.Errorf("update project old_paths: %w", err)
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

// ReassignProjectDataByPath moves conversation/message rows from a project that
// currently uses `path` to `targetProjectID`. Returns number of conversations moved.
func ReassignProjectDataByPath(ctx context.Context, db *sql.DB, targetProjectID, path string) (int64, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, nil
	}

	var sourceProjectID string
	err := db.QueryRowContext(ctx, "SELECT id FROM projects WHERE path = ? AND id <> ?", path, targetProjectID).Scan(&sourceProjectID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("query source project by path: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, "UPDATE conversations SET project_id = ? WHERE project_id = ?", targetProjectID, sourceProjectID)
	if err != nil {
		return 0, fmt.Errorf("reassign conversations: %w", err)
	}
	movedConversations, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reassign conversations rows affected: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "UPDATE messages SET project_id = ? WHERE project_id = ?", targetProjectID, sourceProjectID); err != nil {
		return 0, fmt.Errorf("reassign messages: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit reassignment: %w", err)
	}
	return movedConversations, nil
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
	rows, err := db.QueryContext(ctx, "SELECT id, path, old_paths, label, git_id, default_branch, remote, local_user, local_email, ignored, ignore_diff_paths, ignore_default_diff_paths FROM projects WHERE git_id = ''")
	if err != nil {
		return nil, fmt.Errorf("query projects without git_id: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Path, &p.OldPaths, &p.Label, &p.GitID, &p.DefaultBranch, &p.Remote, &p.LocalUser, &p.LocalEmail, &p.Ignored, &p.IgnoreDiffPaths, &p.IgnoreDefaultDiffPaths); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// ListAllProjects returns all projects (used for label backfill).
func ListAllProjects(ctx context.Context, db *sql.DB) ([]Project, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, path, old_paths, label, git_id, default_branch, remote, local_user, local_email, ignored, ignore_diff_paths, ignore_default_diff_paths FROM projects")
	if err != nil {
		return nil, fmt.Errorf("query all projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Path, &p.OldPaths, &p.Label, &p.GitID, &p.DefaultBranch, &p.Remote, &p.LocalUser, &p.LocalEmail, &p.Ignored, &p.IgnoreDiffPaths, &p.IgnoreDefaultDiffPaths); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// UpdateProjectLocalUser sets the local_user and local_email on a project.
func UpdateProjectLocalUser(ctx context.Context, db *sql.DB, projectID, localUser, localEmail string) error {
	res, err := db.ExecContext(ctx, "UPDATE projects SET local_user = ?, local_email = ? WHERE id = ?", localUser, localEmail, projectID)
	if err != nil {
		return fmt.Errorf("update project local_user: %w", err)
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

// DeleteProject removes a project and all its associated data (conversations,
// messages, ratings, commits, commit_agent_coverage, commit_sync_state).
func DeleteProject(ctx context.Context, database *sql.DB, projectID string) error {
	// Verify project exists.
	var exists int
	err := database.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE id = ?", projectID).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("project %s: %w", projectID, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("check project exists: %w", err)
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete ratings via conversation subquery.
	if _, err := tx.ExecContext(ctx, "DELETE FROM ratings WHERE conversation_id IN (SELECT id FROM conversations WHERE project_id = ?)", projectID); err != nil {
		return fmt.Errorf("delete ratings: %w", err)
	}
	// Delete messages.
	if _, err := tx.ExecContext(ctx, "DELETE FROM messages WHERE project_id = ?", projectID); err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}
	// Delete conversations.
	if _, err := tx.ExecContext(ctx, "DELETE FROM conversations WHERE project_id = ?", projectID); err != nil {
		return fmt.Errorf("delete conversations: %w", err)
	}
	// Delete commit_agent_coverage via commit subquery.
	if _, err := tx.ExecContext(ctx, "DELETE FROM commit_agent_coverage WHERE commit_id IN (SELECT id FROM commits WHERE project_id = ?)", projectID); err != nil {
		return fmt.Errorf("delete commit_agent_coverage: %w", err)
	}
	// Delete commits.
	if _, err := tx.ExecContext(ctx, "DELETE FROM commits WHERE project_id = ?", projectID); err != nil {
		return fmt.Errorf("delete commits: %w", err)
	}
	// Delete commit_sync_state.
	if _, err := tx.ExecContext(ctx, "DELETE FROM commit_sync_state WHERE project_id = ?", projectID); err != nil {
		return fmt.Errorf("delete commit_sync_state: %w", err)
	}
	// Delete the project itself.
	if _, err := tx.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", projectID); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}

	return tx.Commit()
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
