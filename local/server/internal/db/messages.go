package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/gitutil"
)

type replaceDerivedDiffsContextKey struct{}

// Message holds the data for a single conversation message to be inserted.
type Message struct {
	Timestamp      int64
	ProjectID      string
	ConversationID string
	Role           string
	MessageType    string
	Model          string
	Content        string
	RawJSON        string
}

// RepoLabel returns the name of the git repository root directory for the
// given path. It walks up the directory tree looking for a .git entry. If no
// git root is found it falls back to the last path component.
func RepoLabel(path string) string {
	dir := filepath.Clean(path)
	for {
		gitPath := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			// If .git is a file, check if it's a worktree pointing to a parent repo.
			if !info.IsDir() {
				if parentRoot, ok := gitutil.ResolveWorktreeParent(gitPath); ok {
					return filepath.Base(parentRoot)
				}
			}
			return filepath.Base(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Base(path)
}

// EnsureProject inserts a project if it doesn't already exist and returns its ID.
func EnsureProject(ctx context.Context, db *sql.DB, path string) (string, error) {
	return EnsureProjectWithAliases(ctx, db, path)
}

// EnsureProjectWithAliases inserts a project if it doesn't already exist and
// returns its ID. Aliases are added to old_paths when the project resolves to
// an existing canonical row.
func EnsureProjectWithAliases(ctx context.Context, db *sql.DB, path string, aliases ...string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("project path is required")
	}
	path = filepath.Clean(path)

	var id string
	// Check aliases first so renamed/moved projects continue to map to the
	// canonical project even if a legacy exact-path project row still exists.
	if existingID, err := findProjectIDByOldPath(ctx, db, path); err != nil {
		return "", fmt.Errorf("query project by old path: %w", err)
	} else if existingID != "" {
		if err := addProjectPathAliases(ctx, db, existingID, path, aliases...); err != nil {
			return "", err
		}
		return existingID, nil
	}

	// Exact match on canonical path.
	err := db.QueryRowContext(ctx, "SELECT id FROM projects WHERE path = ?", path).Scan(&id)
	if err == nil {
		if err := addProjectPathAliases(ctx, db, id, path, aliases...); err != nil {
			return "", err
		}
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("query project: %w", err)
	}

	// Check if the path is a subdirectory of an existing project.
	if existingID, err := findProjectIDByParentPath(ctx, db, path); err != nil {
		return "", fmt.Errorf("query project by parent path: %w", err)
	} else if existingID != "" {
		if err := addProjectPathAliases(ctx, db, existingID, path, aliases...); err != nil {
			return "", err
		}
		return existingID, nil
	}

	if existingID, err := findProjectIDByRepoIdentity(ctx, db, path); err != nil {
		return "", fmt.Errorf("query project by repo identity: %w", err)
	} else if existingID != "" {
		if err := addProjectPathAliases(ctx, db, existingID, path, aliases...); err != nil {
			return "", err
		}
		return existingID, nil
	}

	id = newID()
	_, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO projects (id, path, label) VALUES (?, ?, ?)", id, path, RepoLabel(path))
	if err != nil {
		return "", fmt.Errorf("insert project: %w", err)
	}

	// Re-query in case another goroutine inserted first.
	err = db.QueryRowContext(ctx, "SELECT id FROM projects WHERE path = ?", path).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("re-query project: %w", err)
	}
	if err := addProjectPathAliases(ctx, db, id, path, aliases...); err != nil {
		return "", err
	}
	return id, nil
}

func addProjectPathAliases(ctx context.Context, db *sql.DB, projectID, canonicalPath string, aliases ...string) error {
	var currentPath, oldPaths string
	err := db.QueryRowContext(ctx, "SELECT path, old_paths FROM projects WHERE id = ?", projectID).Scan(&currentPath, &oldPaths)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("query project aliases: %w", err)
	}

	seen := make(map[string]struct{})
	combined := make([]string, 0)
	for _, candidate := range strings.Split(oldPaths, "\n") {
		candidate = normalizeProjectAliasPath(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		combined = append(combined, candidate)
	}

	allCandidates := append([]string{canonicalPath}, aliases...)
	for _, candidate := range allCandidates {
		candidate = normalizeProjectAliasPath(candidate)
		if candidate == "" || candidate == currentPath {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		combined = append(combined, candidate)
	}

	sort.Strings(combined)
	nextOldPaths := strings.Join(combined, "\n")
	if nextOldPaths == oldPaths {
		return nil
	}
	if err := SetProjectOldPaths(ctx, db, projectID, nextOldPaths); err != nil {
		return fmt.Errorf("update project aliases: %w", err)
	}
	return nil
}

func normalizeProjectAliasPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if path == "." {
		return ""
	}
	return path
}

func findProjectIDByOldPath(ctx context.Context, db *sql.DB, path string) (string, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, old_paths FROM projects WHERE old_paths <> ''")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var oldPaths string
		if err := rows.Scan(&id, &oldPaths); err != nil {
			return "", err
		}
		for _, oldPath := range strings.Split(oldPaths, "\n") {
			oldPath = strings.TrimSpace(oldPath)
			if oldPath == "" {
				continue
			}
			// Exact match or subdirectory of an old path.
			if path == oldPath || strings.HasPrefix(path, oldPath+string(filepath.Separator)) {
				return id, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", nil
}

type projectRepoIdentity struct {
	GitID   string
	RepoKey string
}

func findProjectIDByRepoIdentity(ctx context.Context, db *sql.DB, path string) (string, error) {
	identity := resolveProjectRepoIdentity(path)
	if identity.GitID == "" && identity.RepoKey == "" {
		return "", nil
	}

	rows, err := db.QueryContext(ctx, "SELECT id, git_id, remote, alt_remotes FROM projects WHERE git_id <> '' OR remote <> '' OR alt_remotes <> ''")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var id, gitID, remote, altRemotes string
		if err := rows.Scan(&id, &gitID, &remote, &altRemotes); err != nil {
			return "", err
		}
		if identity.GitID != "" && gitID == identity.GitID {
			return id, nil
		}
		if identity.RepoKey == "" {
			continue
		}
		if normalized := normalizeRemoteToRepoKey(remote); normalized != "" && normalized == identity.RepoKey {
			return id, nil
		}
		for _, alt := range strings.Split(altRemotes, "\n") {
			if normalized := normalizeRemoteToRepoKey(alt); normalized != "" && normalized == identity.RepoKey {
				return id, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func resolveProjectRepoIdentity(path string) projectRepoIdentity {
	path = strings.TrimSpace(path)
	if path == "" {
		return projectRepoIdentity{}
	}
	if _, err := os.Stat(path); err != nil {
		return projectRepoIdentity{}
	}
	return projectRepoIdentity{
		GitID:   gitutil.ResolveRootID(path),
		RepoKey: normalizeRemoteToRepoKey(resolveGitRemote(path)),
	}
}

func resolveGitRemote(path string) string {
	cmd := exec.Command("git", "-C", path, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func normalizeRemoteToRepoKey(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}

	if strings.Contains(remote, "@") && strings.Contains(remote, ":") {
		parts := strings.SplitN(remote, "@", 2)
		if len(parts) == 2 {
			hostAndPath := strings.TrimSuffix(parts[1], ".git")
			hostAndPath = strings.Replace(hostAndPath, ":", "/", 1)
			segments := strings.Split(hostAndPath, "/")
			if len(segments) >= 3 {
				return strings.ToLower(segments[0] + "/" + segments[1] + "/" + segments[2])
			}
		}
	}

	remote = strings.TrimPrefix(remote, "git://")
	remote = strings.TrimPrefix(remote, "https://")
	remote = strings.TrimPrefix(remote, "http://")
	remote = strings.TrimSuffix(remote, ".git")
	remote = strings.TrimSuffix(remote, "/")

	segments := strings.Split(remote, "/")
	if len(segments) >= 3 {
		return strings.ToLower(segments[0] + "/" + segments[1] + "/" + segments[2])
	}
	return ""
}

// findProjectIDByParentPath checks if the given path is a subdirectory of any
// existing project's canonical path.
func findProjectIDByParentPath(ctx context.Context, db *sql.DB, path string) (string, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, path FROM projects")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var id, projectPath string
		if err := rows.Scan(&id, &projectPath); err != nil {
			return "", err
		}
		projectPath = strings.TrimSpace(projectPath)
		if projectPath == "" {
			continue
		}
		if strings.HasPrefix(path, projectPath+string(filepath.Separator)) {
			return id, nil
		}
	}
	return "", rows.Err()
}

// EnsureConversation inserts a conversation if it doesn't already exist.
// If the conversation exists but is linked to a different (possibly stale)
// project, the project_id is updated to the current value.
func EnsureConversation(ctx context.Context, db *sql.DB, conversationID, projectID, agent string) error {
	_, err := db.ExecContext(ctx,
		"INSERT INTO conversations (id, project_id, agent, family_root_id) VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET project_id = ? WHERE project_id <> ?",
		conversationID, projectID, agent, conversationID, projectID, projectID,
	)
	if err != nil {
		return fmt.Errorf("insert conversation: %w", err)
	}
	return nil
}

// dupWindowMs is the maximum timestamp difference (in milliseconds) within which
// two messages with the same conversation, role, and content are considered duplicates.
const dupWindowMs = 10_000 // 10 seconds

const hiddenIngestMessageMaxLen = 256

var hiddenIngestMessageRe = regexp.MustCompile(`(?s)^[\[<].*[\]>]$`)

const derivedDiffRawJSON = `{"source":"derived_diff"}`

// WithReplaceDerivedDiffs marks a DB write context so synthetic derived diff
// rows are replaced only when an incoming replacement row is being inserted.
func WithReplaceDerivedDiffs(ctx context.Context) context.Context {
	return context.WithValue(ctx, replaceDerivedDiffsContextKey{}, true)
}

func shouldReplaceDerivedDiffs(ctx context.Context) bool {
	v, _ := ctx.Value(replaceDerivedDiffsContextKey{}).(bool)
	return v
}

// InsertMessages inserts multiple messages in a single transaction, skipping duplicates.
// Duplicates are detected both within the batch (same conversation + role + content
// within dupWindowMs) and against existing rows in the database.
func InsertMessages(ctx context.Context, db *sql.DB, messages []Message) error {
	for i := range messages {
		messages[i].MessageType = canonicalMessageType(messages[i].Role, messages[i].MessageType, messages[i].Content)
	}
	messages = filterMessagesForIngest(messages)
	messages = deduplicateMessages(messages)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO messages (id, timestamp, project_id, conversation_id, role, message_type, model, content, raw_json)
		 SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?
		 WHERE NOT EXISTS (
		     SELECT 1 FROM messages
		     WHERE conversation_id = ? AND role = ? AND message_type = ? AND model = ? AND content = ?
		     AND ABS(timestamp - ?) < ?
		 )`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert message: %w", err)
	}
	defer stmt.Close()

	var replaceDerivedStmt *sql.Stmt
	if shouldReplaceDerivedDiffs(ctx) {
		replaceDerivedStmt, err = tx.PrepareContext(ctx,
			`DELETE FROM messages
			 WHERE conversation_id = ?
			   AND role = ?
			   AND timestamp = ?
			   AND message_type = ?
			   AND raw_json = ?`,
		)
		if err != nil {
			return fmt.Errorf("prepare replace derived diff delete: %w", err)
		}
		defer replaceDerivedStmt.Close()
	}

	updateModelStmt, err := tx.PrepareContext(ctx,
		`UPDATE messages
		 SET model = ?
		 WHERE conversation_id = ? AND timestamp = ? AND model = ''`,
	)
	if err != nil {
		return fmt.Errorf("prepare update message model: %w", err)
	}
	defer updateModelStmt.Close()

	conversationIDs := make(map[string]struct{}, len(messages))
	for _, m := range messages {
		conversationIDs[m.ConversationID] = struct{}{}
		if replaceDerivedStmt != nil && m.MessageType == MessageTypeDiff && m.RawJSON == derivedDiffRawJSON {
			if _, err := replaceDerivedStmt.ExecContext(
				ctx,
				m.ConversationID,
				m.Role,
				m.Timestamp,
				MessageTypeDiff,
				derivedDiffRawJSON,
			); err != nil {
				return fmt.Errorf("replace derived diff message: %w", err)
			}
		}
		if _, err := stmt.ExecContext(ctx,
			newID(), m.Timestamp, m.ProjectID, m.ConversationID, m.Role, m.MessageType, m.Model, m.Content, m.RawJSON,
			m.ConversationID, m.Role, m.MessageType, m.Model, m.Content, m.Timestamp, dupWindowMs,
		); err != nil {
			return fmt.Errorf("insert message: %w", err)
		}
		if m.Model != "" {
			if _, err := updateModelStmt.ExecContext(ctx, m.Model, m.ConversationID, m.Timestamp); err != nil {
				return fmt.Errorf("update message model: %w", err)
			}
		}
	}

	updateConversationBoundsStmt, err := tx.PrepareContext(ctx,
		`UPDATE conversations
		 SET
		     started_at = CASE
		         WHEN started_at = 0 THEN ?
		         WHEN ? = 0 THEN started_at
		         ELSE MIN(started_at, ?)
		     END,
		     ended_at = CASE
		         WHEN ? = 0 THEN ended_at
		         ELSE MAX(ended_at, ?)
		     END
		 WHERE id = ?`,
	)
	if err != nil {
		return fmt.Errorf("prepare update conversation bounds: %w", err)
	}
	defer updateConversationBoundsStmt.Close()

	// Track started_at from ALL messages (min timestamp).
	conversationStartBounds := make(map[string]int64, len(messages))
	// Track ended_at from USER messages only (max timestamp).
	conversationEndBounds := make(map[string]int64, len(messages))
	for _, m := range messages {
		if prev, ok := conversationStartBounds[m.ConversationID]; !ok || m.Timestamp < prev {
			conversationStartBounds[m.ConversationID] = m.Timestamp
		}
		if m.MessageType == MessageTypePrompt || m.MessageType == MessageTypeAnswer {
			if prev, ok := conversationEndBounds[m.ConversationID]; !ok || m.Timestamp > prev {
				conversationEndBounds[m.ConversationID] = m.Timestamp
			}
		}
	}

	for conversationID := range conversationIDs {
		batchMin, ok := conversationStartBounds[conversationID]
		if !ok {
			continue
		}
		batchMaxUser := conversationEndBounds[conversationID] // 0 if no user messages
		if _, err := updateConversationBoundsStmt.ExecContext(ctx, batchMin, batchMin, batchMin, batchMaxUser, batchMaxUser, conversationID); err != nil {
			return fmt.Errorf("update conversation bounds: %w", err)
		}
	}

	// Recalculate user_prompt_count for each affected conversation.
	updatePromptCountStmt, err := tx.PrepareContext(ctx,
		`UPDATE conversations SET user_prompt_count = (
			SELECT COUNT(*) FROM messages
			WHERE conversation_id = ? AND message_type = 'prompt'
		) WHERE id = ?`,
	)
	if err != nil {
		return fmt.Errorf("prepare update user_prompt_count: %w", err)
	}
	defer updatePromptCountStmt.Close()

	for conversationID := range conversationIDs {
		if _, err := updatePromptCountStmt.ExecContext(ctx, conversationID, conversationID); err != nil {
			return fmt.Errorf("update user_prompt_count: %w", err)
		}
	}

	// Recalculate files_edited for each affected conversation.
	for conversationID := range conversationIDs {
		if err := RecalcFilesEdited(ctx, tx, conversationID); err != nil {
			return fmt.Errorf("recalc files edited: %w", err)
		}
	}

	return tx.Commit()
}

// DeleteDerivedDiffMessages removes synthetic diff messages matching the
// optional project and agent scope. Empty projectID or agent values act as
// wildcards. It returns the number of rows deleted.
func DeleteDerivedDiffMessages(ctx context.Context, db *sql.DB, projectID, agent string) (int64, error) {
	query := `DELETE FROM messages
		WHERE message_type = ?
		  AND raw_json = ?`
	args := []any{MessageTypeDiff, derivedDiffRawJSON}
	if strings.TrimSpace(projectID) != "" || strings.TrimSpace(agent) != "" {
		query += `
		  AND conversation_id IN (
		    SELECT id FROM conversations
		    WHERE 1 = 1`
		if strings.TrimSpace(projectID) != "" {
			query += ` AND project_id = ?`
			args = append(args, projectID)
		}
		if strings.TrimSpace(agent) != "" {
			query += ` AND agent = ?`
			args = append(args, agent)
		}
		query += `
		  )`
	}

	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete derived diff messages: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("derived diff rows affected: %w", err)
	}
	return n, nil
}

func filterMessagesForIngest(messages []Message) []Message {
	result := make([]Message, 0, len(messages))
	for _, m := range messages {
		if shouldSkipMessageOnIngest(m) {
			continue
		}
		result = append(result, m)
	}
	return result
}

func shouldSkipMessageOnIngest(m Message) bool {
	trimmed := strings.TrimSpace(m.Content)
	if trimmed == "" {
		return false
	}
	if utf8.RuneCountInString(trimmed) >= hiddenIngestMessageMaxLen {
		return false
	}
	return hiddenIngestMessageRe.MatchString(trimmed)
}

// deduplicateMessages removes messages within the same conversation that have the same
// role and content within dupWindowMs, keeping only the first occurrence.
func deduplicateMessages(messages []Message) []Message {
	type key struct {
		conversationID string
		role           string
		messageType    string
		model          string
		content        string
	}
	seen := make(map[key]int64) // key -> first-seen timestamp
	result := make([]Message, 0, len(messages))
	for _, m := range messages {
		k := key{m.ConversationID, m.Role, m.MessageType, m.Model, m.Content}
		if prevTs, ok := seen[k]; ok && absInt64(m.Timestamp-prevTs) < dupWindowMs {
			continue
		}
		seen[k] = m.Timestamp
		result = append(result, m)
	}
	return result
}

func absInt64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

var diffHeaderRe = regexp.MustCompile(`diff --git a/(.+?) b/`)

// RecalcFilesEdited recomputes the files_edited column for a single conversation.
// It accepts either a *sql.Tx or *sql.DB as the executor.
func RecalcFilesEdited(ctx context.Context, execer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}, conversationID string) error {
	rows, err := execer.QueryContext(ctx,
		`SELECT content FROM messages WHERE conversation_id = ? AND content LIKE '%diff --git a/%'`,
		conversationID,
	)
	if err != nil {
		return fmt.Errorf("query diff messages: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	var files []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return fmt.Errorf("scan diff content: %w", err)
		}
		for _, match := range diffHeaderRe.FindAllStringSubmatch(content, -1) {
			fp := strings.TrimSpace(match[1])
			if fp == "" || fp == "(.+?)" {
				continue
			}
			r := []rune(fp)
			if len(r) > 1024 {
				fp = string(r[:1024])
			}
			if _, ok := seen[fp]; !ok {
				seen[fp] = struct{}{}
				files = append(files, fp)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate diff messages: %w", err)
	}

	filesEdited := strings.Join(files, "\n")
	if _, err := execer.ExecContext(ctx,
		`UPDATE conversations SET files_edited = ? WHERE id = ?`,
		filesEdited, conversationID,
	); err != nil {
		return fmt.Errorf("update files_edited: %w", err)
	}
	return nil
}

// backfillAllFilesEdited recalculates files_edited for all conversations.
func backfillAllFilesEdited(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT conversation_id FROM messages WHERE content LIKE '%diff --git a/%'`)
	if err != nil {
		return fmt.Errorf("query conversations with diffs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan conversation id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate conversation ids: %w", err)
	}

	for _, id := range ids {
		if err := RecalcFilesEdited(ctx, db, id); err != nil {
			return fmt.Errorf("backfill files_edited for %s: %w", id, err)
		}
	}
	return nil
}
