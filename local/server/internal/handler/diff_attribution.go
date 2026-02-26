package handler

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"
)

// countDiffAddedRemoved counts the total lines added and removed from a unified diff.
func countDiffAddedRemoved(diffText string) (added, removed int) {
	for _, line := range strings.Split(diffText, "\n") {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			added++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			removed++
		}
	}
	return added, removed
}

func tokenTotals(tokens []diffToken) (int, int) {
	lines := 0
	chars := 0
	for _, tok := range tokens {
		if !tok.Attributable {
			continue
		}
		lines++
		chars += tok.Chars
	}
	return lines, chars
}

func attributeCommitToMessages(
	commitTokens []diffToken,
	messages []messageDiff,
	windowStart, windowEnd int64,
) ([]commitContributionMessage, int, int, map[string]commitFileCoverage, map[string]int) {
	// Sort messages newest-first so that when multiple messages contain the
	// same token, the most recent message wins attribution.
	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].Timestamp != messages[j].Timestamp {
			return messages[i].Timestamp > messages[j].Timestamp
		}
		return messages[i].ID > messages[j].ID
	})

	matchedLines := 0
	matchedChars := 0
	tokenSources := make(map[string][]tokenSource)
	messageTokensByBucket := make(map[int]map[string][]int)
	messageTokenUsed := make(map[int][]bool)
	commitMatched := make([]bool, len(commitTokens))
	// Keep a full multiset of normalized message lines for copied-file detection.
	// This must not be decremented by exact path matches, otherwise copied files
	// can be severely under-attributed when the same lines also appear elsewhere.
	normSources := make(map[string]int)
	for i, msg := range messages {
		if msg.Timestamp <= windowStart || msg.Timestamp > windowEnd {
			continue
		}
		pathTokens := make(map[string][]int)
		messageTokenUsed[i] = make([]bool, len(msg.Tokens))
		for pos, tok := range msg.Tokens {
			if !tok.Attributable {
				continue
			}
			tokenSources[tok.Key] = append(tokenSources[tok.Key], tokenSource{msgIdx: i, tokenPos: pos})
			pathTokens[tokenBucketKey(tok.Path, tok.Sign)] = append(pathTokens[tokenBucketKey(tok.Path, tok.Sign)], pos)
			if tok.Norm != "" {
				normSources[tok.Norm]++
			}
		}
		messageTokensByBucket[i] = pathTokens
	}

	contribByIndex := make(map[int]*commitContributionMessage)
	fileCoverageByPath := make(map[string]commitFileCoverage)
	type fileAgentStats struct {
		lines int
		chars int
	}
	fileAgentByPath := make(map[string]map[string]*fileAgentStats)
	recordFileAgentMatch := func(filePath string, msgIdx int, chars int) {
		if filePath == "" {
			filePath = "(unknown)"
		}
		agent := strings.TrimSpace(messages[msgIdx].Agent)
		if agent == "" {
			agent = "unknown"
		}
		byAgent := fileAgentByPath[filePath]
		if byAgent == nil {
			byAgent = make(map[string]*fileAgentStats)
			fileAgentByPath[filePath] = byAgent
		}
		stats := byAgent[agent]
		if stats == nil {
			stats = &fileAgentStats{}
			byAgent[agent] = stats
		}
		stats.lines++
		stats.chars += chars
	}
	for tokIdx, tok := range commitTokens {
		if !tok.Attributable {
			continue
		}
		path := tok.Path
		if path == "" {
			path = "(unknown)"
		}
		fileCov := fileCoverageByPath[path]
		fileCov.Path = path
		fileCov.Added++

		sources := tokenSources[tok.Key]
		if len(sources) == 0 {
			fileCoverageByPath[path] = fileCov
			continue
		}
		source := sources[0]
		tokenSources[tok.Key] = sources[1:]
		messageTokenUsed[source.msgIdx][source.tokenPos] = true
		commitMatched[tokIdx] = true

		matchedLines++
		matchedChars += tok.Chars
		fileCov.Removed++
		fileCoverageByPath[path] = fileCov
		recordFileAgentMatch(path, source.msgIdx, tok.Chars)
		contrib := contribByIndex[source.msgIdx]
		if contrib == nil {
			msg := messages[source.msgIdx]
			contrib = &commitContributionMessage{
				ID:                msg.ID,
				Timestamp:         msg.Timestamp,
				ConversationID:    msg.ConversationID,
				ConversationTitle: msg.ConversationTitle,
				Agent:             msg.Agent,
				Model:             msg.Model,
				Content:           msg.Content,
			}
			contribByIndex[source.msgIdx] = contrib
		}
		contrib.LinesMatched++
		contrib.CharsMatched += tok.Chars
	}

	// Second pass: recover attribution for formatting-only changes that alter
	// line breaks. We compare normalized windows (up to 5 lines on either side)
	// within the same file path and allow different line counts when the joined
	// normalized content is identical.
	type tokenBucket struct {
		path string
		sign byte
	}
	commitByPath := make(map[tokenBucket][]int)
	for i, tok := range commitTokens {
		if tok.Path == "" || tok.Norm == "" || commitMatched[i] || !tok.Attributable {
			continue
		}
		commitByPath[tokenBucket{path: tok.Path, sign: tok.Sign}] = append(commitByPath[tokenBucket{path: tok.Path, sign: tok.Sign}], i)
	}

	for bucket, indices := range commitByPath {
		path := bucket.path
		bucketKey := tokenBucketKey(path, bucket.sign)
		for cursor := 0; cursor < len(indices); {
			matchedWindow := false
			maxCommitWindow := maxFormattingWindowLines
			if remaining := len(indices) - cursor; remaining < maxCommitWindow {
				maxCommitWindow = remaining
			}

			for commitWindow := maxCommitWindow; commitWindow >= 1 && !matchedWindow; commitWindow-- {
				commitNorm := concatCommitNorms(commitTokens, indices[cursor:cursor+commitWindow])
				if commitNorm == "" {
					continue
				}

				for msgIdx, msg := range messages {
					positions := messageTokensByBucket[msgIdx][bucketKey]
					if len(positions) == 0 {
						continue
					}
					maxMessageWindow := maxFormattingWindowLines
					if len(positions) < maxMessageWindow {
						maxMessageWindow = len(positions)
					}
					for messageWindow := 1; messageWindow <= maxMessageWindow && !matchedWindow; messageWindow++ {
						for start := 0; start+messageWindow <= len(positions); start++ {
							windowPositions := positions[start : start+messageWindow]
							if !messageWindowAvailable(messageTokenUsed[msgIdx], windowPositions) {
								continue
							}
							if concatMessageNorms(msg.Tokens, windowPositions) != commitNorm {
								continue
							}

							for _, idx := range indices[cursor : cursor+commitWindow] {
								commitMatched[idx] = true
								matchedLines++
								matchedChars += commitTokens[idx].Chars
								fileCov := fileCoverageByPath[path]
								fileCov.Path = path
								fileCov.Removed++
								fileCoverageByPath[path] = fileCov
								recordFileAgentMatch(path, msgIdx, commitTokens[idx].Chars)
							}

							for _, pos := range windowPositions {
								messageTokenUsed[msgIdx][pos] = true
							}

							contrib := contribByIndex[msgIdx]
							if contrib == nil {
								contrib = &commitContributionMessage{
									ID:                msg.ID,
									Timestamp:         msg.Timestamp,
									ConversationID:    msg.ConversationID,
									ConversationTitle: msg.ConversationTitle,
									Agent:             msg.Agent,
									Model:             msg.Model,
									Content:           msg.Content,
								}
								contribByIndex[msgIdx] = contrib
							}
							for _, idx := range indices[cursor : cursor+commitWindow] {
								contrib.LinesMatched++
								contrib.CharsMatched += commitTokens[idx].Chars
							}

							cursor += commitWindow
							matchedWindow = true
							break
						}
					}
					if matchedWindow {
						break
					}
				}
			}

			if !matchedWindow {
				cursor++
			}
		}
	}

	out := make([]commitContributionMessage, 0, len(contribByIndex))
	for _, contrib := range contribByIndex {
		out = append(out, *contrib)
	}
	// Sort output by ascending timestamp for consistent chronological display.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp < out[j].Timestamp
		}
		return out[i].ID < out[j].ID
	})
	for filePath, byAgent := range fileAgentByPath {
		fileCov := fileCoverageByPath[filePath]
		agents := make([]string, 0, len(byAgent))
		for agent := range byAgent {
			agents = append(agents, agent)
		}
		sort.Strings(agents)
		segments := make([]agentCoverageSegment, 0, len(agents))
		for _, agent := range agents {
			stats := byAgent[agent]
			segments = append(segments, agentCoverageSegment{
				Agent:          agent,
				LinesFromAgent: stats.lines,
				CharsFromAgent: stats.chars,
			})
		}
		fileCov.AgentSegments = segments
		fileCoverageByPath[filePath] = fileCov
	}

	return out, matchedLines, matchedChars, fileCoverageByPath, normSources
}

func concatCommitNorms(tokens []diffToken, indices []int) string {
	if len(indices) == 0 {
		return ""
	}
	var b strings.Builder
	for _, idx := range indices {
		norm := tokens[idx].Norm
		if norm == "" {
			return ""
		}
		b.WriteString(norm)
	}
	return b.String()
}

func concatMessageNorms(tokens []diffToken, positions []int) string {
	if len(positions) == 0 {
		return ""
	}
	var b strings.Builder
	for _, pos := range positions {
		norm := tokens[pos].Norm
		if norm == "" {
			return ""
		}
		b.WriteString(norm)
	}
	return b.String()
}

func tokenBucketKey(path string, sign byte) string {
	return path + "\x1f" + string(sign)
}

func messageWindowAvailable(used []bool, positions []int) bool {
	for _, pos := range positions {
		if used[pos] {
			return false
		}
	}
	return true
}

func detectModelFromJSON(rawJSON string) string {
	rawJSON = strings.TrimSpace(rawJSON)
	if rawJSON == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
		return ""
	}
	return findModelInJSON(v)
}

func findModelInJSON(v any) string {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range []string{"model", "modelName", "model_name", "model_slug", "modelSlug"} {
			if s, ok := t[k].(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s
				}
			}
		}
		for _, nested := range t {
			if m := findModelInJSON(nested); m != "" {
				return m
			}
		}
	case []any:
		for _, item := range t {
			if m := findModelInJSON(item); m != "" {
				return m
			}
		}
	}
	return ""
}

// attributeCopiedFromAgentFiles assigns per-agent segments to files marked as
// copiedFromAgent by cross-referencing the file's normalized tokens against a
// norm→agent map built from messages in the commit's time window.
// It also returns the aggregate agent segments across all such files.
func attributeCopiedFromAgentFiles(
	files []commitFileCoverage,
	commitTokens []diffToken,
	messages []messageDiff,
	windowStart, windowEnd int64,
	linesTotal int,
) []agentCoverageSegment {
	// Build norm→agent map: for each normalized line, track the dominant agent.
	type agentCount struct{ counts map[string]int }
	normAgents := make(map[string]*agentCount)
	for _, msg := range messages {
		if msg.Timestamp <= windowStart || msg.Timestamp > windowEnd {
			continue
		}
		agent := strings.TrimSpace(msg.Agent)
		if agent == "" {
			agent = "unknown"
		}
		for _, tok := range msg.Tokens {
			if !tok.Attributable || tok.Norm == "" {
				continue
			}
			ac := normAgents[tok.Norm]
			if ac == nil {
				ac = &agentCount{counts: make(map[string]int)}
				normAgents[tok.Norm] = ac
			}
			ac.counts[agent]++
		}
	}
	dominantAgent := func(norm string) string {
		ac := normAgents[norm]
		if ac == nil {
			return ""
		}
		best, bestN := "", 0
		for a, n := range ac.counts {
			if n > bestN {
				best, bestN = a, n
			}
		}
		return best
	}

	// Build per-file norm lists from commit tokens.
	fileNorms := make(map[string][]string)
	for _, tok := range commitTokens {
		if tok.Path == "" || tok.Norm == "" || !tok.Attributable {
			continue
		}
		fileNorms[tok.Path] = append(fileNorms[tok.Path], tok.Norm)
	}

	overallAgentLines := make(map[string]int)
	for i, f := range files {
		if !f.CopiedFromAgent || f.LinesFromAgent == 0 || len(f.AgentSegments) > 0 {
			continue
		}
		norms := fileNorms[f.Path]
		if len(norms) == 0 {
			continue
		}
		agentLines := make(map[string]int)
		for _, norm := range norms {
			if a := dominantAgent(norm); a != "" {
				agentLines[a]++
			}
		}
		if len(agentLines) == 0 {
			continue
		}
		agents := make([]string, 0, len(agentLines))
		for a := range agentLines {
			agents = append(agents, a)
		}
		sort.Strings(agents)
		segments := make([]agentCoverageSegment, 0, len(agents))
		for _, a := range agents {
			segments = append(segments, agentCoverageSegment{
				Agent:          a,
				LinesFromAgent: agentLines[a],
				LinePercent:    percentage(agentLines[a], len(norms)),
			})
			overallAgentLines[a] += agentLines[a]
		}
		files[i].AgentSegments = segments
	}

	// Also aggregate from non-copied files that already have segments.
	for _, f := range files {
		if f.CopiedFromAgent || f.Ignored || f.Moved {
			continue
		}
		for _, seg := range f.AgentSegments {
			overallAgentLines[seg.Agent] += seg.LinesFromAgent
		}
	}

	if len(overallAgentLines) == 0 {
		return nil
	}
	agents := make([]string, 0, len(overallAgentLines))
	for a := range overallAgentLines {
		agents = append(agents, a)
	}
	sort.Strings(agents)
	out := make([]agentCoverageSegment, 0, len(agents))
	for _, a := range agents {
		out = append(out, agentCoverageSegment{
			Agent:          a,
			LinesFromAgent: overallAgentLines[a],
			LinePercent:    percentage(overallAgentLines[a], linesTotal),
		})
	}
	return out
}

func summarizeDiffFiles(
	diffText string,
	ignorePatterns []string,
	commitTokens []diffToken,
	fileAgent map[string]commitFileCoverage,
	remainingNorms map[string]int,
) ([]commitFileCoverage, int, int) {
	diffText = strings.ReplaceAll(diffText, "\r\n", "\n")
	lines := strings.Split(diffText, "\n")

	oldPath := ""
	newPath := ""
	coverageByPath := make(map[string]commitFileCoverage)

	ensure := func(path string) string {
		if path == "" {
			return ""
		}
		c := coverageByPath[path]
		c.Path = path
		c.Ignored = shouldIgnoreDiffPath(path, ignorePatterns)
		coverageByPath[path] = c
		return path
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				oldPath = parseDiffPath(parts[2])
				newPath = parseDiffPath(parts[3])
				if newPath != "" {
					ensure(newPath)
				} else if oldPath != "" {
					ensure(oldPath)
				}
			}
		case strings.HasPrefix(line, "rename from "):
			oldPath = parseDiffPath(strings.TrimPrefix(line, "rename from "))
			if oldPath != "" {
				ensure(oldPath)
			}
		case strings.HasPrefix(line, "rename to "):
			newPath = parseDiffPath(strings.TrimPrefix(line, "rename to "))
			if newPath != "" {
				newPath = ensure(newPath)
				c := coverageByPath[newPath]
				c.Moved = true
				c.MovedFrom = oldPath
				coverageByPath[newPath] = c
			}
		case strings.HasPrefix(line, "--- "):
			oldPath = parseDiffPath(strings.TrimPrefix(line, "--- "))
			if oldPath != "" {
				ensure(oldPath)
			}
		case strings.HasPrefix(line, "+++ "):
			newPath = parseDiffPath(strings.TrimPrefix(line, "+++ "))
			if newPath != "" {
				ensure(newPath)
			}
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			p := newPath
			if p == "" {
				p = oldPath
			}
			p = ensure(p)
			if p == "" {
				continue
			}
			c := coverageByPath[p]
			c.Added++
			coverageByPath[p] = c
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			p := oldPath
			if p == "" {
				p = newPath
			}
			p = ensure(p)
			if p == "" {
				continue
			}
			c := coverageByPath[p]
			c.Removed++
			coverageByPath[p] = c
		}
	}

	filePaths := make([]string, 0, len(coverageByPath))
	for filePath := range coverageByPath {
		filePaths = append(filePaths, filePath)
	}
	sort.Strings(filePaths)

	fileNorms := make(map[string][]string)
	for _, tok := range commitTokens {
		path := tok.Path
		if path == "" || tok.Norm == "" || !tok.Attributable {
			continue
		}
		fileNorms[path] = append(fileNorms[path], tok.Norm)
	}

	var extraLines, extraChars int
	out := make([]commitFileCoverage, 0, len(filePaths))
	for _, filePath := range filePaths {
		c := coverageByPath[filePath]
		c.LinesTotal = c.Added + c.Removed
		if !c.Ignored {
			if agent, ok := fileAgent[filePath]; ok {
				c.LinesFromAgent = agent.Removed
				// Exact attribution uses normalized token totals so whitespace-only
				// diff lines do not lower percentages for otherwise exact matches.
				c.LinePercent = percentage(c.LinesFromAgent, agent.Added)
				if len(agent.AgentSegments) > 0 {
					segments := make([]agentCoverageSegment, 0, len(agent.AgentSegments))
					for _, seg := range agent.AgentSegments {
						if seg.LinesFromAgent <= 0 {
							continue
						}
						seg.LinePercent = percentage(seg.LinesFromAgent, agent.Added)
						segments = append(segments, seg)
					}
					c.AgentSegments = segments
				}
			}
			// Fallback: detect relocated/copied agent code by matching normalized
			// lines independent of file path. For large diffs (>=10 lines) require
			// at least 10 matched lines; for small diffs (<10 lines) require ALL
			// attributable lines to match with a minimum of 2.
			if !c.Moved && c.LinesFromAgent == 0 {
				norms := fileNorms[filePath]
				minMatch := 10
				if c.LinesTotal < 10 && len(norms) >= 2 && len(norms) < 10 {
					minMatch = len(norms)
				}
				if len(norms) >= minMatch {
					fallbackMatched := 0
					fallbackMatchedChars := 0
					for _, norm := range norms {
						if remainingNorms[norm] <= 0 {
							continue
						}
						remainingNorms[norm]--
						fallbackMatched++
						fallbackMatchedChars += utf8.RuneCountInString(norm)
					}
					if fallbackMatched >= minMatch {
						c.LinesFromAgent = fallbackMatched
						c.LinePercent = percentage(c.LinesFromAgent, len(norms))
						c.CopiedFromAgent = true
						extraLines += fallbackMatched
						extraChars += fallbackMatchedChars
					}
				}
			}
		}
		out = append(out, c)
	}
	return out, extraLines, extraChars
}
