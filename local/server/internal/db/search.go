package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

type ProjectSearchMatch struct {
	Project             Project `json:"project"`
	ConversationMatches int     `json:"conversationMatches"`
	CommitMatches       int     `json:"commitMatches"`
}

func tokenizeSearchTerms(term string) []string {
	fields := strings.Fields(strings.TrimSpace(term))
	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		trimmed := strings.TrimSpace(f)
		if trimmed == "" {
			continue
		}
		tokens = append(tokens, trimmed)
	}
	return tokens
}

func buildFTSMatchQuery(term string) string {
	tokens := tokenizeSearchTerms(term)
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if utf8.RuneCountInString(token) < 3 {
			continue
		}
		escaped := strings.ReplaceAll(token, `"`, `""`)
		parts = append(parts, `"`+escaped+`"`)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " AND ")
}

func isShortSearchOnly(term string) bool {
	tokens := tokenizeSearchTerms(term)
	if len(tokens) == 0 {
		return true
	}
	for _, token := range tokens {
		if utf8.RuneCountInString(token) >= 3 {
			return false
		}
	}
	return true
}

func FilterCommitHashesBySearch(ctx context.Context, database *sql.DB, projectID string, hashes []string, term string) ([]string, error) {
	term = strings.TrimSpace(term)
	if term == "" || len(hashes) == 0 {
		return hashes, nil
	}

	matches := make(map[string]bool, len(hashes))
	ftsQuery := buildFTSMatchQuery(term)
	useFallback := !supportsFTS5(ctx, database) || isShortSearchOnly(term)

	for i := 0; i < len(hashes); i += sqliteBatchSize - 4 {
		end := i + sqliteBatchSize - 4
		if end > len(hashes) {
			end = len(hashes)
		}
		batch := hashes[i:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")

		if useFallback || ftsQuery == "" {
			query := fmt.Sprintf(
				`SELECT DISTINCT commit_hash
				 FROM commits
				 WHERE project_id = ?
				   AND commit_hash IN (%s)
				   AND (
				     instr(lower(subject), lower(?)) > 0 OR
				     instr(lower(diff_content), lower(?)) > 0 OR
				     instr(lower(commit_hash), lower(?)) > 0
				   )`,
				placeholders,
			)
			args := make([]any, 0, len(batch)+4)
			args = append(args, projectID)
			for _, h := range batch {
				args = append(args, h)
			}
			args = append(args, term, term, term)
			rows, err := database.QueryContext(ctx, query, args...)
			if err != nil {
				return nil, fmt.Errorf("query short commit search matches: %w", err)
			}
			for rows.Next() {
				var hash string
				if err := rows.Scan(&hash); err != nil {
					rows.Close()
					return nil, fmt.Errorf("scan short commit search hash: %w", err)
				}
				matches[hash] = true
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return nil, fmt.Errorf("iterate short commit search hash rows: %w", err)
			}
			rows.Close()
			continue
		}

		query := fmt.Sprintf(
			`SELECT DISTINCT commit_hash
			 FROM commits_fts
			 WHERE project_id = ?
			   AND commit_hash IN (%s)
			   AND commits_fts MATCH ?`,
			placeholders,
		)
		args := make([]any, 0, len(batch)+2)
		args = append(args, projectID)
		for _, h := range batch {
			args = append(args, h)
		}
		args = append(args, ftsQuery)
		rows, err := database.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query commit search matches: %w", err)
		}
		for rows.Next() {
			var hash string
			if err := rows.Scan(&hash); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan commit search hash: %w", err)
			}
			matches[hash] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterate commit search hash rows: %w", err)
		}
		rows.Close()
	}

	filtered := make([]string, 0, len(matches))
	for _, hash := range hashes {
		if matches[hash] {
			filtered = append(filtered, hash)
		}
	}
	return filtered, nil
}

func SearchProjectMatches(ctx context.Context, database *sql.DB, term, projectID string) ([]ProjectSearchMatch, error) {
	term = strings.TrimSpace(term)
	projectID = strings.TrimSpace(projectID)
	if term == "" {
		return []ProjectSearchMatch{}, nil
	}

	projects, err := ListProjects(ctx, database, false)
	if err != nil {
		return nil, err
	}
	projectMap := make(map[string]Project, len(projects))
	for _, p := range projects {
		if projectID != "" && p.ID != projectID {
			continue
		}
		projectMap[p.ID] = p
	}
	if len(projectMap) == 0 {
		return []ProjectSearchMatch{}, nil
	}

	conversationCounts := make(map[string]int)
	commitCounts := make(map[string]int)

	if err := loadConversationMatchCounts(ctx, database, term, projectID, conversationCounts); err != nil {
		return nil, err
	}
	if err := loadCommitMatchCounts(ctx, database, term, projectID, commitCounts); err != nil {
		return nil, err
	}

	results := make([]ProjectSearchMatch, 0, len(conversationCounts)+len(commitCounts))
	for pid, project := range projectMap {
		cm := conversationCounts[pid]
		km := commitCounts[pid]
		if cm == 0 && km == 0 {
			continue
		}
		results = append(results, ProjectSearchMatch{
			Project:             project,
			ConversationMatches: cm,
			CommitMatches:       km,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		iTotal := results[i].ConversationMatches + results[i].CommitMatches
		jTotal := results[j].ConversationMatches + results[j].CommitMatches
		if iTotal != jTotal {
			return iTotal > jTotal
		}
		iLabel := strings.TrimSpace(results[i].Project.Label)
		jLabel := strings.TrimSpace(results[j].Project.Label)
		if iLabel == "" {
			iLabel = results[i].Project.Path
		}
		if jLabel == "" {
			jLabel = results[j].Project.Path
		}
		return strings.ToLower(iLabel) < strings.ToLower(jLabel)
	})

	return results, nil
}

func loadConversationMatchCounts(ctx context.Context, database *sql.DB, term, projectID string, counts map[string]int) error {
	ftsQuery := buildFTSMatchQuery(term)
	useFallback := !supportsFTS5(ctx, database) || isShortSearchOnly(term)
	if useFallback || ftsQuery == "" {
		query := `SELECT mf.project_id, COUNT(DISTINCT mf.conversation_id)
			FROM messages_fts mf
			JOIN conversations c ON c.id = mf.conversation_id
			WHERE c.hidden = 0
			  AND instr(lower(mf.content), lower(?)) > 0`
		args := []any{term}
		if projectID != "" {
			query += " AND mf.project_id = ?"
			args = append(args, projectID)
		}
		query += " GROUP BY mf.project_id"
		rows, err := database.QueryContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("query fallback conversation match counts: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var pid string
			var count int
			if err := rows.Scan(&pid, &count); err != nil {
				return fmt.Errorf("scan fallback conversation match count: %w", err)
			}
			counts[pid] = count
		}
		return rows.Err()
	}

	query := `SELECT mf.project_id, COUNT(DISTINCT mf.conversation_id)
		FROM messages_fts mf
		JOIN conversations c ON c.id = mf.conversation_id
		WHERE c.hidden = 0
		  AND messages_fts MATCH ?`
	args := []any{ftsQuery}
	if projectID != "" {
		query += " AND mf.project_id = ?"
		args = append(args, projectID)
	}
	query += " GROUP BY mf.project_id"

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query conversation match counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pid string
		var count int
		if err := rows.Scan(&pid, &count); err != nil {
			return fmt.Errorf("scan conversation match count: %w", err)
		}
		counts[pid] = count
	}
	return rows.Err()
}

func loadCommitMatchCounts(ctx context.Context, database *sql.DB, term, projectID string, counts map[string]int) error {
	ftsQuery := buildFTSMatchQuery(term)
	useFallback := !supportsFTS5(ctx, database) || isShortSearchOnly(term)
	if useFallback || ftsQuery == "" {
		query := `SELECT project_id, COUNT(DISTINCT commit_hash)
			FROM commits_fts
			WHERE (
			     instr(lower(subject), lower(?)) > 0
			  OR instr(lower(diff_content), lower(?)) > 0
			  OR instr(lower(commit_hash), lower(?)) > 0
			)`
		args := []any{term, term, term}
		if projectID != "" {
			query += " AND project_id = ?"
			args = append(args, projectID)
		}
		query += " GROUP BY project_id"
		rows, err := database.QueryContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("query fallback commit match counts: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var pid string
			var count int
			if err := rows.Scan(&pid, &count); err != nil {
				return fmt.Errorf("scan fallback commit match count: %w", err)
			}
			counts[pid] = count
		}
		return rows.Err()
	}

	query := `SELECT project_id, COUNT(DISTINCT commit_hash)
		FROM commits_fts
		WHERE commits_fts MATCH ?`
	args := []any{ftsQuery}
	if projectID != "" {
		query += " AND project_id = ?"
		args = append(args, projectID)
	}
	query += " GROUP BY project_id"

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query commit match counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pid string
		var count int
		if err := rows.Scan(&pid, &count); err != nil {
			return fmt.Errorf("scan commit match count: %w", err)
		}
		counts[pid] = count
	}
	return rows.Err()
}
