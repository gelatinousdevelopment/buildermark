package handler

import (
	"sort"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func dbCommitToCoverage(c db.Commit, repoProject *db.Project) projectCommitCoverage {
	lp := percentage(c.LinesFromAgent, c.LinesTotal)
	if c.OverrideLinePercent != nil {
		lp = *c.OverrideLinePercent
	}
	return projectCommitCoverage{
		ProjectID:           repoProject.ID,
		ProjectLabel:        repoProject.Label,
		ProjectPath:         repoProject.Path,
		ProjectGitID:        repoProject.GitID,
		CommitHash:          c.CommitHash,
		Subject:             c.Subject,
		UserName:            c.UserName,
		UserEmail:           c.UserEmail,
		AuthoredAtUnixMs:    c.AuthoredAt * 1000,
		LinesTotal:          c.LinesTotal,
		LinesFromAgent:      c.LinesFromAgent,
		LinePercent:         lp,
		CharsTotal:          c.CharsTotal,
		CharsFromAgent:      c.CharsFromAgent,
		CharacterPercent:    percentage(c.CharsFromAgent, c.CharsTotal),
		LinesAdded:          c.LinesAdded,
		LinesRemoved:        c.LinesRemoved,
		OverrideLinePercent: c.OverrideLinePercent,
		NeedsParent:         c.NeedsParent,
	}
}

func summarizeCommitCoverage(commits []projectCommitCoverage) projectCommitSummary {
	s := projectCommitSummary{CommitCount: len(commits)}
	agentTotals := make(map[string][2]int) // agent -> [lines, chars]
	for _, c := range commits {
		s.LinesTotal += c.LinesTotal
		s.LinesFromAgent += c.LinesFromAgent
		s.CharsTotal += c.CharsTotal
		s.CharsFromAgent += c.CharsFromAgent
		for _, seg := range c.AgentSegments {
			t := agentTotals[seg.Agent]
			t[0] += seg.LinesFromAgent
			t[1] += seg.CharsFromAgent
			agentTotals[seg.Agent] = t
		}
	}
	s.LinePercent = percentage(s.LinesFromAgent, s.LinesTotal)
	s.CharacterPercent = percentage(s.CharsFromAgent, s.CharsTotal)
	if len(agentTotals) > 0 {
		agents := make([]string, 0, len(agentTotals))
		for a := range agentTotals {
			agents = append(agents, a)
		}
		sort.Strings(agents)
		for _, a := range agents {
			t := agentTotals[a]
			s.AgentSegments = append(s.AgentSegments, agentCoverageSegment{
				Agent:          a,
				LinesFromAgent: t[0],
				CharsFromAgent: t[1],
				LinePercent:    percentage(t[0], s.LinesTotal),
			})
		}
	}
	return s
}

// buildDailySummary buckets commits by date in the given location and produces
// a trailing daily window ending today, newest last.
//
// The chart UX relies on at least 30 day buckets, so shorter requested windows
// are expanded to 30 days.
func buildDailySummary(allCoverage []projectCommitCoverage, days int, loc *time.Location) []dailyCommitSummary {
	return buildDailySummaryWindow(allCoverage, days, loc, nil, true)
}

func buildDailySummaryWindow(
	allCoverage []projectCommitCoverage,
	days int,
	loc *time.Location,
	windowEnd *time.Time,
	enforceMinWindow bool,
) []dailyCommitSummary {
	if enforceMinWindow && days < 30 {
		days = 30
	}
	if days < 1 {
		days = 1
	}

	end := time.Now().In(loc)
	if windowEnd != nil {
		end = windowEnd.In(loc)
	}
	today := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, loc)

	// Build date-keyed buckets from commits.
	type bucket struct {
		linesTotal     int
		linesFromAgent int
		agentTotals    map[string]int // agent -> lines
		commits        []dailyCommitRef
	}
	buckets := make(map[string]*bucket)
	for _, c := range allCoverage {
		date := time.UnixMilli(c.AuthoredAtUnixMs).In(loc).Format("2006-01-02")
		b := buckets[date]
		if b == nil {
			b = &bucket{agentTotals: make(map[string]int)}
			buckets[date] = b
		}
		b.linesTotal += c.LinesTotal
		b.linesFromAgent += c.LinesFromAgent
		for _, seg := range c.AgentSegments {
			b.agentTotals[seg.Agent] += seg.LinesFromAgent
		}
		b.commits = append(b.commits, dailyCommitRef{
			CommitHash: c.CommitHash,
			Subject:    c.Subject,
			ProjectID:  c.ProjectID,
		})
	}

	out := make([]dailyCommitSummary, days)
	for i := 0; i < days; i++ {
		d := today.AddDate(0, 0, -(days - 1 - i))
		dateStr := d.Format("2006-01-02")
		ds := dailyCommitSummary{
			Date:    dateStr,
			Commits: []dailyCommitRef{},
		}
		if b, ok := buckets[dateStr]; ok {
			ds.LinesTotal = b.linesTotal
			ds.LinesFromAgent = b.linesFromAgent
			ds.LinePercent = percentage(b.linesFromAgent, b.linesTotal)
			ds.Commits = b.commits

			if len(b.agentTotals) > 0 {
				agents := make([]string, 0, len(b.agentTotals))
				for a := range b.agentTotals {
					agents = append(agents, a)
				}
				sort.Strings(agents)
				for _, a := range agents {
					ds.AgentSegments = append(ds.AgentSegments, agentCoverageSegment{
						Agent:          a,
						LinesFromAgent: b.agentTotals[a],
						LinePercent:    percentage(b.agentTotals[a], b.linesTotal),
					})
				}
			}
		}
		out[i] = ds
	}
	return out
}

// agentSegmentsFromDBCoverage converts DB agent coverage rows into API segments.
func agentSegmentsFromDBCoverage(rows []db.CommitAgentCoverage, linesTotal int) []agentCoverageSegment {
	if len(rows) == 0 {
		return nil
	}
	out := make([]agentCoverageSegment, 0, len(rows))
	for _, r := range rows {
		out = append(out, agentCoverageSegment{
			Agent:          r.Agent,
			LinesFromAgent: r.LinesFromAgent,
			CharsFromAgent: r.CharsFromAgent,
			LinePercent:    percentage(r.LinesFromAgent, linesTotal),
		})
	}
	return out
}

func percentage(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return (float64(part) * 100) / float64(total)
}
