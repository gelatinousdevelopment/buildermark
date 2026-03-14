package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DailyActivityRow holds the daily counts for conversations, user prompts, and
// user answers (responses to tool questions / permission prompts).
type DailyActivityRow struct {
	Date          string `json:"date"`
	Conversations int    `json:"conversations"`
	UserPrompts   int    `json:"userPrompts"`
	UserAnswers   int    `json:"userAnswers"`
}

// GetDailyActivity returns daily conversation and user-prompt counts for a
// project within the given time range. Each conversation is counted at most
// once, on the local day of its latest role=user message. User prompts and
// answers are counted by message timestamp, while excluding the first message
// in child conversations since those are plan-generated handoff prompts.
func GetDailyActivity(ctx context.Context, db *sql.DB, projectID string, startMs, endExclusiveMs int64, timeZone string, tzOffsetMin int) ([]DailyActivityRow, error) {
	loc, err := activityLocation(timeZone, tzOffsetMin)
	if err != nil {
		return nil, err
	}

	// Build the set of dates in the range so we include zero-count days.
	startTime := time.UnixMilli(startMs).In(loc)
	endTime := time.UnixMilli(endExclusiveMs - 1).In(loc)
	startDay := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, loc)
	endDay := time.Date(endTime.Year(), endTime.Month(), endTime.Day(), 0, 0, 0, 0, loc)

	dateMap := make(map[string]*DailyActivityRow)
	var dates []string
	for d := startDay; !d.After(endDay); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		dates = append(dates, ds)
		dateMap[ds] = &DailyActivityRow{Date: ds}
	}

	// Count non-hidden conversations once each by the day of their latest user message.
	convRows, err := db.QueryContext(ctx,
		`SELECT MAX(m.timestamp) AS latest_user_ts
		 FROM conversations c
		 JOIN messages m ON m.conversation_id = c.id
		 WHERE c.project_id = ? AND c.hidden = 0 AND m.role = 'user'
		 GROUP BY c.id
		 HAVING latest_user_ts >= ? AND latest_user_ts < ?`,
		projectID, startMs, endExclusiveMs,
	)
	if err != nil {
		return nil, fmt.Errorf("query daily conversations: %w", err)
	}
	defer convRows.Close()

	for convRows.Next() {
		var latestUserTs int64
		if err := convRows.Scan(&latestUserTs); err != nil {
			return nil, fmt.Errorf("scan daily conversations: %w", err)
		}
		day := time.UnixMilli(latestUserTs).In(loc).Format("2006-01-02")
		if row, ok := dateMap[day]; ok {
			row.Conversations++
		}
	}
	if err := convRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate daily conversations: %w", err)
	}

	// Count user prompts and answers per day. Exclude the first message of child
	// conversations (parent_conversation_id != '').
	msgRows, err := db.QueryContext(ctx,
		`SELECT m.timestamp, m.message_type
		 FROM messages m
		 JOIN conversations c ON m.conversation_id = c.id
		 WHERE m.project_id = ?
		   AND m.message_type IN ('prompt', 'answer')
		   AND m.timestamp >= ? AND m.timestamp < ?
		   AND c.hidden = 0
		   AND NOT (
		     c.parent_conversation_id != ''
		     AND m.id = (
		       SELECT m2.id FROM messages m2
		       WHERE m2.conversation_id = c.id
		       ORDER BY m2.timestamp ASC, m2.id ASC
		       LIMIT 1
		     )
		   )`,
		projectID, startMs, endExclusiveMs,
	)
	if err != nil {
		return nil, fmt.Errorf("query daily messages: %w", err)
	}
	defer msgRows.Close()

	for msgRows.Next() {
		var timestamp int64
		var messageType string
		if err := msgRows.Scan(&timestamp, &messageType); err != nil {
			return nil, fmt.Errorf("scan daily messages: %w", err)
		}
		day := time.UnixMilli(timestamp).In(loc).Format("2006-01-02")
		row, ok := dateMap[day]
		if !ok {
			continue
		}
		switch messageType {
		case "prompt":
			row.UserPrompts++
		case "answer":
			row.UserAnswers++
		}
	}
	if err := msgRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate daily messages: %w", err)
	}

	result := make([]DailyActivityRow, len(dates))
	for i, d := range dates {
		result[i] = *dateMap[d]
	}
	return result, nil
}

func activityLocation(timeZone string, tzOffsetMin int) (*time.Location, error) {
	if tz := timeZone; tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return nil, fmt.Errorf("load activity timezone %q: %w", tz, err)
		}
		return loc, nil
	}
	return time.FixedZone(fmt.Sprintf("utc%+d", -tzOffsetMin/60), -tzOffsetMin*60), nil
}
