package db

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

var pastedTextRe = regexp.MustCompile(`\[Pasted text #\d+.*\]`)
var hiddenMessagePrefixes = []string{
	"<command-message",
	"<command-name",
	"<command-args",
	"<local-command",
	"<system-reminder>",
	"<user-prompt-submit-hook>",
	"[Request interrupted",
}

// Conversation represents a row in the conversations table.
type Conversation struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Agent     string `json:"agent"`
	Title     string `json:"title"`
	StartedAt int64  `json:"startedAt"`
	EndedAt   int64  `json:"endedAt"`
}

// MessageRead is a message as returned by read queries.
type MessageRead struct {
	ID             string `json:"id"`
	Timestamp      int64  `json:"timestamp"`
	ConversationID string `json:"conversationId"`
	Role           string `json:"role"`
	Model          string `json:"model"`
	Content        string `json:"content"`
	RawJSON        string `json:"rawJson"`
}

// ConversationDetail is a conversation with all its messages and ratings.
type ConversationDetail struct {
	ID        string        `json:"id"`
	ProjectID string        `json:"projectId"`
	Agent     string        `json:"agent"`
	Title     string        `json:"title"`
	StartedAt int64         `json:"startedAt"`
	EndedAt   int64         `json:"endedAt"`
	Messages  []MessageRead `json:"messages"`
	Ratings   []Rating      `json:"ratings"`
}

// ListConversations returns conversations, up to limit.
func ListConversations(ctx context.Context, db *sql.DB, limit int) ([]Conversation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := db.QueryContext(ctx, "SELECT id, project_id, agent, title, started_at, ended_at FROM conversations ORDER BY id LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer rows.Close()

	conversations := []Conversation{}
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Agent, &c.Title, &c.StartedAt, &c.EndedAt); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		conversations = append(conversations, c)
	}
	return conversations, rows.Err()
}

// GetConversationDetail returns a conversation with all its turns and ratings.
func GetConversationDetail(ctx context.Context, db *sql.DB, conversationID string) (*ConversationDetail, error) {
	resolvedID := conversationID
	var c ConversationDetail
	err := db.QueryRowContext(ctx,
		"SELECT id, project_id, agent, title, started_at, ended_at FROM conversations WHERE id = ?", resolvedID,
	).Scan(&c.ID, &c.ProjectID, &c.Agent, &c.Title, &c.StartedAt, &c.EndedAt)
	if err == sql.ErrNoRows {
		aliasConversationID, found, resolveErr := ResolveConversationIDByTempID(ctx, db, conversationID)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve conversation alias: %w", resolveErr)
		}
		if !found || aliasConversationID == "" || aliasConversationID == conversationID {
			return nil, nil
		}
		resolvedID = aliasConversationID
		err = db.QueryRowContext(ctx,
			"SELECT id, project_id, agent, title, started_at, ended_at FROM conversations WHERE id = ?", resolvedID,
		).Scan(&c.ID, &c.ProjectID, &c.Agent, &c.Title, &c.StartedAt, &c.EndedAt)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("query conversation by alias: %w", err)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("query conversation: %w", err)
	}

	// Fetch messages ordered by most recent first.
	messageRows, err := db.QueryContext(ctx,
		"SELECT id, timestamp, conversation_id, role, model, content, raw_json FROM messages WHERE conversation_id = ? ORDER BY timestamp DESC",
		resolvedID,
	)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer messageRows.Close()

	c.Messages = []MessageRead{}
	for messageRows.Next() {
		var m MessageRead
		if err := messageRows.Scan(&m.ID, &m.Timestamp, &m.ConversationID, &m.Role, &m.Model, &m.Content, &m.RawJSON); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		c.Messages = append(c.Messages, m)
	}
	if err := messageRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	// Fetch ratings.
	ratRows, err := db.QueryContext(ctx,
		"SELECT id, conversation_id, temp_conversation_id, rating, note, analysis, created_at FROM ratings WHERE conversation_id = ? ORDER BY created_at DESC",
		resolvedID,
	)
	if err != nil {
		return nil, fmt.Errorf("query ratings: %w", err)
	}
	defer ratRows.Close()

	c.Ratings = []Rating{}
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
		c.Ratings = append(c.Ratings, r)
	}
	if err := ratRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ratings: %w", err)
	}

	// Match each rating to the closest /zrate user message within 120s.
	matchedMessageIDs := make(map[string]bool)
	for i := range c.Ratings {
		ratingTime := c.Ratings[i].CreatedAt.UnixMilli()
		var bestIdx int
		var bestDelta int64 = 120_001
		for j := range c.Messages {
			msg := &c.Messages[j]
			if msg.Role != "user" {
				continue
			}
			trimmed := strings.TrimLeft(msg.Content, " \t\n\r")
			if !strings.HasPrefix(trimmed, "/zrate") && !strings.HasPrefix(trimmed, "$zrate") {
				continue
			}
			if matchedMessageIDs[msg.ID] {
				continue
			}
			delta := abs64(msg.Timestamp - ratingTime)
			if delta < bestDelta {
				bestDelta = delta
				bestIdx = j
			}
		}
		if bestDelta <= 120_000 {
			matchedMessageIDs[c.Messages[bestIdx].ID] = true
			ts := c.Messages[bestIdx].Timestamp
			c.Ratings[i].MatchedTimestamp = &ts
		}
	}

	// Filter messages: remove matched /zrate messages, empty content,
	// system/meta command markers, and /clear or /new commands.
	filtered := make([]MessageRead, 0, len(c.Messages))
	for _, msg := range c.Messages {
		if matchedMessageIDs[msg.ID] {
			continue
		}
		trimmed := strings.TrimSpace(msg.Content)
		if shouldHideMessageContent(msg.Role, trimmed) {
			continue
		}
		if msg.Role == "user" && (trimmed == "/clear" || trimmed == "/new") {
			continue
		}
		if msg.Role == "user" && pastedTextRe.MatchString(trimmed) {
			continue
		}
		filtered = append(filtered, msg)
	}
	c.Messages = filtered

	return &c, nil
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func shouldHideMessageContent(role, trimmed string) bool {
	if trimmed == "" || trimmed == "[user]" {
		return true
	}
	if strings.ToLower(strings.TrimSpace(role)) != "user" {
		return false
	}
	for _, prefix := range hiddenMessagePrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// ConversationBatchDetail holds the user messages and ratings for a single
// conversation, used by the batch-detail endpoint.
type ConversationBatchDetail struct {
	ConversationID string        `json:"conversationId"`
	UserMessages   []MessageRead `json:"userMessages"`
	Ratings        []Rating      `json:"ratings"`
}

// GetConversationsBatchDetail returns filtered user messages and ratings for
// multiple conversations in a single optimized query set.
func GetConversationsBatchDetail(ctx context.Context, db *sql.DB, conversationIDs []string) ([]ConversationBatchDetail, error) {
	if len(conversationIDs) == 0 {
		return []ConversationBatchDetail{}, nil
	}

	result := make(map[string]*ConversationBatchDetail, len(conversationIDs))
	for _, id := range conversationIDs {
		result[id] = &ConversationBatchDetail{
			ConversationID: id,
			UserMessages:   []MessageRead{},
			Ratings:        []Rating{},
		}
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(conversationIDs)), ",")
	idArgs := make([]any, len(conversationIDs))
	for i, id := range conversationIDs {
		idArgs[i] = id
	}

	// Fetch all user-role messages for these conversations.
	msgRows, err := db.QueryContext(ctx,
		fmt.Sprintf(`SELECT id, timestamp, conversation_id, role, model, content, raw_json
			FROM messages
			WHERE conversation_id IN (%s) AND role = 'user'
			ORDER BY timestamp ASC`, placeholders),
		idArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("query batch user messages: %w", err)
	}
	defer msgRows.Close()

	// Collect all user messages, plus track /zrate messages for rating matching.
	type userMsg struct {
		msg MessageRead
	}
	allUserMsgs := make(map[string][]userMsg)
	for msgRows.Next() {
		var m MessageRead
		if err := msgRows.Scan(&m.ID, &m.Timestamp, &m.ConversationID, &m.Role, &m.Model, &m.Content, &m.RawJSON); err != nil {
			return nil, fmt.Errorf("scan batch user message: %w", err)
		}
		allUserMsgs[m.ConversationID] = append(allUserMsgs[m.ConversationID], userMsg{msg: m})
	}
	if err := msgRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate batch user messages: %w", err)
	}

	// Fetch all ratings for these conversations.
	ratRows, err := db.QueryContext(ctx,
		fmt.Sprintf(`SELECT id, conversation_id, temp_conversation_id, rating, note, analysis, created_at
			FROM ratings
			WHERE conversation_id IN (%s)
			ORDER BY created_at DESC`, placeholders),
		idArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("query batch ratings: %w", err)
	}
	defer ratRows.Close()

	allRatings := make(map[string][]Rating)
	for ratRows.Next() {
		var r Rating
		var createdAt string
		if err := ratRows.Scan(&r.ID, &r.ConversationID, &r.TempConversationID, &r.Rating, &r.Note, &r.Analysis, &createdAt); err != nil {
			return nil, fmt.Errorf("scan batch rating: %w", err)
		}
		r.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse batch rating created_at: %w", err)
		}
		allRatings[r.ConversationID] = append(allRatings[r.ConversationID], r)
	}
	if err := ratRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate batch ratings: %w", err)
	}

	// For each conversation, match ratings to /zrate messages and filter.
	for convID, detail := range result {
		msgs := allUserMsgs[convID]
		ratings := allRatings[convID]

		// Match ratings to /zrate messages (same logic as GetConversationDetail).
		matchedMessageIDs := make(map[string]bool)
		for i := range ratings {
			ratingTime := ratings[i].CreatedAt.UnixMilli()
			var bestIdx int
			var bestDelta int64 = 120_001
			for j := range msgs {
				m := &msgs[j].msg
				if m.Role != "user" {
					continue
				}
				trimmed := strings.TrimLeft(m.Content, " \t\n\r")
				if !strings.HasPrefix(trimmed, "/zrate") && !strings.HasPrefix(trimmed, "$zrate") {
					continue
				}
				if matchedMessageIDs[m.ID] {
					continue
				}
				delta := abs64(m.Timestamp - ratingTime)
				if delta < bestDelta {
					bestDelta = delta
					bestIdx = j
				}
			}
			if bestDelta <= 120_000 {
				matchedMessageIDs[msgs[bestIdx].msg.ID] = true
				ts := msgs[bestIdx].msg.Timestamp
				ratings[i].MatchedTimestamp = &ts
			}
		}

		// Filter user messages using the same rules as GetConversationDetail.
		for _, um := range msgs {
			m := um.msg
			if matchedMessageIDs[m.ID] {
				continue
			}
			trimmed := strings.TrimSpace(m.Content)
			if shouldHideMessageContent(m.Role, trimmed) {
				continue
			}
			if strings.HasPrefix(trimmed, "/zrate") || strings.HasPrefix(trimmed, "$zrate") {
				continue
			}
			if trimmed == "/clear" || trimmed == "/new" {
				continue
			}
			if pastedTextRe.MatchString(trimmed) {
				continue
			}
			detail.UserMessages = append(detail.UserMessages, m)
		}

		detail.Ratings = ratings
	}

	out := make([]ConversationBatchDetail, 0, len(conversationIDs))
	for _, id := range conversationIDs {
		out = append(out, *result[id])
	}
	return out, nil
}

// UntitledConversation is a conversation with an empty title, joined with its project path.
type UntitledConversation struct {
	ID          string
	ProjectPath string
}

// ListUntitledConversations returns conversations that have an empty title for the given agent.
func ListUntitledConversations(ctx context.Context, db *sql.DB, agent string) ([]UntitledConversation, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT c.id, p.path FROM conversations c JOIN projects p ON c.project_id = p.id WHERE c.agent = ? AND c.title = ''",
		agent,
	)
	if err != nil {
		return nil, fmt.Errorf("query untitled conversations: %w", err)
	}
	defer rows.Close()

	var result []UntitledConversation
	for rows.Next() {
		var u UntitledConversation
		if err := rows.Scan(&u.ID, &u.ProjectPath); err != nil {
			return nil, fmt.Errorf("scan untitled conversation: %w", err)
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

// UpdateConversationTitle sets the title on an existing conversation.
func UpdateConversationTitle(ctx context.Context, db *sql.DB, conversationID, title string) error {
	res, err := db.ExecContext(ctx, "UPDATE conversations SET title = ? WHERE id = ?", title, conversationID)
	if err != nil {
		return fmt.Errorf("update conversation title: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("conversation %s: %w", conversationID, ErrNotFound)
	}
	return nil
}

// UpdateConversationProject sets the project_id on an existing conversation.
func UpdateConversationProject(ctx context.Context, db *sql.DB, conversationID, projectID string) error {
	res, err := db.ExecContext(ctx, "UPDATE conversations SET project_id = ? WHERE id = ?", projectID, conversationID)
	if err != nil {
		return fmt.Errorf("update conversation project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("conversation %s: %w", conversationID, ErrNotFound)
	}
	return nil
}
