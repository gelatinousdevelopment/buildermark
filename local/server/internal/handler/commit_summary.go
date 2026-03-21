package handler

import (
	"encoding/json"
	"log"
	"math"
	"sort"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func dbCommitToCoverage(c db.Commit, repoProject *db.Project) projectCommitCoverage {
	lp := percentage(c.LinesFromAgent, c.LinesTotal)
	overrideMap := parseOverrideAgentPercents(c.OverrideAgentPercents, c.CommitHash)
	if len(overrideMap) > 0 {
		lp = totalOverridePercent(overrideMap)
	}
	return projectCommitCoverage{
		ProjectID:             repoProject.ID,
		ProjectLabel:          repoProject.Label,
		ProjectPath:           repoProject.Path,
		ProjectGitID:          repoProject.GitID,
		CommitHash:            c.CommitHash,
		Subject:               c.Subject,
		UserName:              c.UserName,
		UserEmail:             c.UserEmail,
		AuthoredAtUnixMs:      c.AuthoredAt * 1000,
		LinesTotal:            c.LinesTotal,
		LinesFromAgent:        c.LinesFromAgent,
		LinePercent:           lp,
		LinesAdded:            c.LinesAdded,
		LinesRemoved:          c.LinesRemoved,
		OverrideAgentPercents: overrideMap,
		NeedsParent:           c.NeedsParent,
		Ignored:               c.Ignored,
	}
}

func parseOverrideAgentPercents(raw *string, commitHash string) map[string]int {
	if raw == nil || *raw == "" {
		return nil
	}
	var overrideMap map[string]int
	if err := json.Unmarshal([]byte(*raw), &overrideMap); err != nil {
		if commitHash != "" {
			log.Printf("warning: failed to unmarshal override_agent_percents for %s: %v", commitHash, err)
		} else {
			log.Printf("warning: failed to unmarshal override_agent_percents: %v", err)
		}
		return nil
	}
	if len(overrideMap) == 0 {
		return nil
	}
	return overrideMap
}

func totalOverridePercent(override map[string]int) float64 {
	total := 0
	for _, v := range override {
		total += v
	}
	return float64(total)
}

func allocateLinesByPercent(linesTotal int, override map[string]int) map[string]int {
	if linesTotal <= 0 || len(override) == 0 {
		return nil
	}

	type allocation struct {
		agent     string
		lines     int
		remainder float64
	}

	agents := make([]string, 0, len(override))
	for agent, pct := range override {
		if pct > 0 {
			agents = append(agents, agent)
		}
	}
	if len(agents) == 0 {
		return nil
	}
	sort.Strings(agents)

	allocations := make([]allocation, 0, len(agents))
	totalExact := 0.0
	baseSum := 0
	for _, agent := range agents {
		exact := float64(linesTotal) * float64(override[agent]) / 100
		lines := int(math.Floor(exact))
		allocations = append(allocations, allocation{
			agent:     agent,
			lines:     lines,
			remainder: exact - float64(lines),
		})
		totalExact += exact
		baseSum += lines
	}

	target := int(math.Round(totalExact))
	if target < 0 {
		target = 0
	}
	if target > linesTotal {
		target = linesTotal
	}

	sort.SliceStable(allocations, func(i, j int) bool {
		if allocations[i].remainder != allocations[j].remainder {
			return allocations[i].remainder > allocations[j].remainder
		}
		return allocations[i].agent < allocations[j].agent
	})

	for i := 0; i < target-baseSum && i < len(allocations); i++ {
		allocations[i].lines++
	}

	sort.SliceStable(allocations, func(i, j int) bool {
		return allocations[i].agent < allocations[j].agent
	})

	out := make(map[string]int, len(allocations))
	for _, alloc := range allocations {
		out[alloc.agent] = alloc.lines
	}
	return out
}

func agentSegmentsFromOverride(linesTotal int, override map[string]int) []agentCoverageSegment {
	if len(override) == 0 {
		return nil
	}

	agents := make([]string, 0, len(override))
	for agent, pct := range override {
		if pct > 0 {
			agents = append(agents, agent)
		}
	}
	if len(agents) == 0 {
		return nil
	}
	sort.Strings(agents)

	linesByAgent := allocateLinesByPercent(linesTotal, override)
	out := make([]agentCoverageSegment, 0, len(agents))
	for _, agent := range agents {
		out = append(out, agentCoverageSegment{
			Agent:          agent,
			LinesFromAgent: linesByAgent[agent],
			LinePercent:    float64(override[agent]),
		})
	}
	return out
}

func effectiveCommitCoverage(cov projectCommitCoverage, baseSegments []agentCoverageSegment) projectCommitCoverage {
	if len(cov.OverrideAgentPercents) > 0 {
		cov.AgentSegments = agentSegmentsFromOverride(cov.LinesTotal, cov.OverrideAgentPercents)
		cov.LinePercent = totalOverridePercent(cov.OverrideAgentPercents)
		cov.LinesFromAgent = 0
		for _, seg := range cov.AgentSegments {
			cov.LinesFromAgent += seg.LinesFromAgent
		}
		return cov
	}

	if len(baseSegments) > 0 {
		cov.AgentSegments = baseSegments
		return cov
	}

	if cov.LinesFromAgent > 0 {
		cov.AgentSegments = []agentCoverageSegment{{
			Agent:          "unknown",
			LinesFromAgent: cov.LinesFromAgent,
			LinePercent:    percentage(cov.LinesFromAgent, cov.LinesTotal),
		}}
	}
	return cov
}

func summarizeCommitCoverage(commits []projectCommitCoverage) projectCommitSummary {
	s := projectCommitSummary{}
	agentTotals := make(map[string]int)
	for _, c := range commits {
		if c.Ignored {
			continue
		}
		s.CommitCount++
		s.LinesTotal += c.LinesTotal
		s.LinesFromAgent += c.LinesFromAgent
		for _, seg := range c.AgentSegments {
			agentTotals[seg.Agent] += seg.LinesFromAgent
		}
	}
	s.LinePercent = percentage(s.LinesFromAgent, s.LinesTotal)
	if len(agentTotals) > 0 {
		agents := make([]string, 0, len(agentTotals))
		for a := range agentTotals {
			agents = append(agents, a)
		}
		sort.Strings(agents)
		for _, a := range agents {
			s.AgentSegments = append(s.AgentSegments, agentCoverageSegment{
				Agent:          a,
				LinesFromAgent: agentTotals[a],
				LinePercent:    percentage(agentTotals[a], s.LinesTotal),
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
		if !c.Ignored {
			b.linesTotal += c.LinesTotal
			b.linesFromAgent += c.LinesFromAgent
			for _, seg := range c.AgentSegments {
				b.agentTotals[seg.Agent] += seg.LinesFromAgent
			}
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
