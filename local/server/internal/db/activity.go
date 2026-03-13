package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DailyActivityRow holds the daily counts for conversations and user prompts.
type DailyActivityRow struct {
	Date          string `json:"date"`
	Conversations int    `json:"conversations"`
	UserPrompts   int    `json:"userPrompts"`
}

// GetDailyActivity returns daily conversation and user-prompt counts for a
// project within the given time range. Conversations are counted by
// started_at (non-hidden only). User prompts count message_type IN
// ('prompt','answer') but exclude the first message in child conversations
// (where parent_conversation_id is set) since those are plan-generated.
func GetDailyActivity(ctx context.Context, db *sql.DB, projectID string, startMs, endExclusiveMs int64, tzOffsetMin int) ([]DailyActivityRow, error) {
	offsetSec := -tzOffsetMin * 60
	sign := "+"
	if offsetSec < 0 {
		sign = "-"
		offsetSec = -offsetSec
	}
	offsetStr := fmt.Sprintf("%s%02d:%02d", sign, offsetSec/3600, (offsetSec%3600)/60)

	// startSec/endSec for conversations (started_at is in ms).
	startSec := startMs / 1000
	endSec := endExclusiveMs / 1000

	// Build the set of dates in the range so we include zero-count days.
	loc := time.FixedZone("tz", -tzOffsetMin*60)
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

	// Count non-hidden conversations per day by started_at.
	convRows, err := db.QueryContext(ctx,
		fmt.Sprintf(`SELECT DATE(started_at / 1000, 'unixepoch', '%s') AS day, COUNT(*) AS cnt
		 FROM conversations
		 WHERE project_id = ? AND hidden = 0
		   AND started_at / 1000 >= ? AND started_at / 1000 < ?
		 GROUP BY day`, offsetStr),
		projectID, startSec, endSec,
	)
	if err != nil {
		return nil, fmt.Errorf("query daily conversations: %w", err)
	}
	defer convRows.Close()

	for convRows.Next() {
		var day string
		var cnt int
		if err := convRows.Scan(&day, &cnt); err != nil {
			return nil, fmt.Errorf("scan daily conversations: %w", err)
		}
		if row, ok := dateMap[day]; ok {
			row.Conversations = cnt
		}
	}
	if err := convRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate daily conversations: %w", err)
	}

	// Count user prompts per day.
	// Exclude the first message of child conversations (parent_conversation_id != '').
	promptRows, err := db.QueryContext(ctx,
		fmt.Sprintf(`SELECT DATE(m.timestamp / 1000, 'unixepoch', '%s') AS day, COUNT(*) AS cnt
		 FROM messages m
		 JOIN conversations c ON m.conversation_id = c.id
		 WHERE m.project_id = ? AND m.message_type IN ('prompt', 'answer')
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
		   )
		 GROUP BY day`, offsetStr),
		projectID, startMs, endExclusiveMs,
	)
	if err != nil {
		return nil, fmt.Errorf("query daily prompts: %w", err)
	}
	defer promptRows.Close()

	for promptRows.Next() {
		var day string
		var cnt int
		if err := promptRows.Scan(&day, &cnt); err != nil {
			return nil, fmt.Errorf("scan daily prompts: %w", err)
		}
		if row, ok := dateMap[day]; ok {
			row.UserPrompts = cnt
		}
	}
	if err := promptRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate daily prompts: %w", err)
	}

	result := make([]DailyActivityRow, len(dates))
	for i, d := range dates {
		result[i] = *dateMap[d]
	}
	return result, nil
}
