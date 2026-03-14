package db

import (
	"context"
	"database/sql"
	"fmt"
)

// AgentRatingDistribution holds the rating distribution for a single agent.
type AgentRatingDistribution struct {
	Agent              string         `json:"agent"`
	TotalConversations int            `json:"totalConversations"`
	RatedConversations int            `json:"ratedConversations"`
	AverageRating      float64        `json:"averageRating"`
	Distribution       map[string]int `json:"distribution"`
}

// GetRatingsByAgent returns the rating distribution grouped by agent for
// non-hidden conversations in the given project and time range.
func GetRatingsByAgent(ctx context.Context, db *sql.DB, projectID string, startMs, endExclusiveMs int64) ([]AgentRatingDistribution, error) {
	startSec := startMs / 1000
	endSec := endExclusiveMs / 1000

	query := `
WITH conv_ratings AS (
  SELECT
    c.id AS conv_id,
    c.agent,
    CASE
      WHEN COUNT(r.id) = 0 THEN 'unrated'
      ELSE CAST(CAST(MIN(MAX(ROUND(AVG(r.rating)), 1), 5) AS INTEGER) AS TEXT)
    END AS bucket
  FROM conversations c
  LEFT JOIN ratings r ON r.conversation_id = c.id
  WHERE c.project_id = ?
    AND c.hidden = 0
    AND c.started_at / 1000 >= ?
    AND c.started_at / 1000 < ?
  GROUP BY c.id
)
SELECT agent, bucket, COUNT(*) AS cnt
FROM conv_ratings
GROUP BY agent, bucket
ORDER BY agent, bucket`

	rows, err := db.QueryContext(ctx, query, projectID, startSec, endSec)
	if err != nil {
		return nil, fmt.Errorf("query ratings by agent: %w", err)
	}
	defer rows.Close()

	agentMap := make(map[string]*AgentRatingDistribution)
	var agentOrder []string

	for rows.Next() {
		var agent, bucket string
		var cnt int
		if err := rows.Scan(&agent, &bucket, &cnt); err != nil {
			return nil, fmt.Errorf("scan ratings by agent: %w", err)
		}
		dist, ok := agentMap[agent]
		if !ok {
			dist = &AgentRatingDistribution{
				Agent:        agent,
				Distribution: make(map[string]int),
			}
			agentMap[agent] = dist
			agentOrder = append(agentOrder, agent)
		}
		dist.Distribution[bucket] = cnt
		dist.TotalConversations += cnt
		if bucket != "unrated" {
			dist.RatedConversations += cnt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ratings by agent: %w", err)
	}

	// Compute average rating per agent from the distribution buckets.
	for _, dist := range agentMap {
		if dist.RatedConversations == 0 {
			continue
		}
		var sum float64
		for bucket, cnt := range dist.Distribution {
			if bucket == "unrated" {
				continue
			}
			// bucket is "1".."5"
			var val int
			fmt.Sscanf(bucket, "%d", &val)
			sum += float64(val) * float64(cnt)
		}
		dist.AverageRating = sum / float64(dist.RatedConversations)
	}

	result := make([]AgentRatingDistribution, 0, len(agentOrder))
	for _, agent := range agentOrder {
		result = append(result, *agentMap[agent])
	}
	return result, nil
}
