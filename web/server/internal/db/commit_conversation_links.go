package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// CommitConversationLinks holds the bidirectional mapping between commit hashes
// and conversation IDs for a project.
type CommitConversationLinks struct {
	CommitToConversations map[string][]string `json:"commitToConversations"`
	ConversationToCommits map[string][]string `json:"conversationToCommits"`
}

// FindCommitConversationLinks finds relationships between commits and
// conversations by looking for agent messages that fall within each commit's
// authoring time window. The window extends messageWindowMs before and
// lookaheadMs after the commit's authored_at timestamp.
//
// commitHashes and conversationIDs are optional filters; when both are non-empty
// only links involving those items are returned.
func FindCommitConversationLinks(
	ctx context.Context,
	database *sql.DB,
	projectIDs []string,
	commitHashes []string,
	conversationIDs []string,
	messageWindowMs int64,
	lookaheadMs int64,
) (*CommitConversationLinks, error) {
	if len(projectIDs) == 0 || len(commitHashes) == 0 {
		return &CommitConversationLinks{
			CommitToConversations: map[string][]string{},
			ConversationToCommits: map[string][]string{},
		}, nil
	}

	// Step 1: Load commit timestamps for the requested hashes.
	type commitInfo struct {
		Hash       string
		AuthoredAt int64 // unix seconds
	}
	var commits []commitInfo

	for i := 0; i < len(commitHashes); i += sqliteBatchSize {
		end := i + sqliteBatchSize
		if end > len(commitHashes) {
			end = len(commitHashes)
		}
		batch := commitHashes[i:end]
		pidPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(projectIDs)), ",")
		hashPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		query := fmt.Sprintf(
			`SELECT DISTINCT commit_hash, authored_at
			 FROM commits
			 WHERE project_id IN (%s) AND commit_hash IN (%s)`,
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
			return nil, fmt.Errorf("query commit timestamps: %w", err)
		}
		for rows.Next() {
			var ci commitInfo
			if err := rows.Scan(&ci.Hash, &ci.AuthoredAt); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan commit timestamp: %w", err)
			}
			commits = append(commits, ci)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	if len(commits) == 0 {
		return &CommitConversationLinks{
			CommitToConversations: map[string][]string{},
			ConversationToCommits: map[string][]string{},
		}, nil
	}

	// Step 2: For each commit, find conversations that have agent messages in
	// the time window. We build one query per commit to keep things simple and
	// correct (each commit has its own time window).
	c2c := make(map[string][]string)
	c2commit := make(map[string][]string)

	pidPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(projectIDs)), ",")

	// Optional conversation filter clause.
	convClause := ""
	var convArgs []any
	if len(conversationIDs) > 0 {
		convPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(conversationIDs)), ",")
		convClause = fmt.Sprintf(" AND m.conversation_id IN (%s)", convPlaceholders)
		convArgs = make([]any, len(conversationIDs))
		for i, cid := range conversationIDs {
			convArgs[i] = cid
		}
	}

	query := fmt.Sprintf(
		`SELECT DISTINCT m.conversation_id
		 FROM messages m
		 WHERE m.role = 'agent'
		   AND m.project_id IN (%s)
		   AND m.timestamp BETWEEN ? AND ?%s`,
		pidPlaceholders, convClause,
	)

	stmt, err := database.PrepareContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("prepare link query: %w", err)
	}
	defer stmt.Close()

	for _, ci := range commits {
		windowStart := ci.AuthoredAt*1000 - messageWindowMs
		windowEnd := ci.AuthoredAt*1000 + lookaheadMs

		args := make([]any, 0, len(projectIDs)+2+len(convArgs))
		for _, pid := range projectIDs {
			args = append(args, pid)
		}
		args = append(args, windowStart, windowEnd)
		args = append(args, convArgs...)

		rows, err := stmt.QueryContext(ctx, args...)
		if err != nil {
			return nil, fmt.Errorf("query conversation links for commit %s: %w", ci.Hash, err)
		}
		for rows.Next() {
			var convID string
			if err := rows.Scan(&convID); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan conversation link: %w", err)
			}
			c2c[ci.Hash] = append(c2c[ci.Hash], convID)
			c2commit[convID] = append(c2commit[convID], ci.Hash)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	return &CommitConversationLinks{
		CommitToConversations: c2c,
		ConversationToCommits: c2commit,
	}, nil
}
