package handler

import (
	"encoding/json"
	"sort"
	"strings"
)

type exactTokenCandidate struct {
	source    tokenSource
	exact     bool
	preferred bool
}

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

func tokenTotals(tokens []diffToken) int {
	lines := 0
	for _, tok := range tokens {
		if !tok.Attributable {
			continue
		}
		lines++
	}
	return lines
}

// buildMessageIndex builds a messageIndex from messages for reuse across commits.
// Messages are sorted newest-first internally.
func buildMessageIndex(messages []messageDiff, windowStart, windowEnd int64) *messageIndex {
	// Sort messages newest-first so that when multiple messages contain the
	// same token, the most recent message wins attribution.
	sorted := make([]messageDiff, len(messages))
	copy(sorted, messages)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Timestamp != sorted[j].Timestamp {
			return sorted[i].Timestamp > sorted[j].Timestamp
		}
		return sorted[i].ID > sorted[j].ID
	})

	idx := &messageIndex{
		messages:                   sorted,
		tokenSources:               make(map[string][]tokenSource),
		styleTokenSources:          make(map[string][]tokenSource),
		tokensByBucket:             make(map[int]map[string][]int),
		normSources:                make(map[string]int),
		normAgents:                 make(map[string]map[string]int),
		normConversationCounts:     make(map[string]map[string]int),
		pathNormConversationCounts: make(map[string]map[string]map[string]int),
		conversationMeta:           make(map[string]fallbackConversationMeta),
	}

	for i, msg := range sorted {
		if msg.Timestamp <= windowStart || msg.Timestamp > windowEnd {
			continue
		}
		agent := strings.TrimSpace(msg.Agent)
		if agent == "" {
			agent = "unknown"
		}
		if msg.ConversationID != "" {
			idx.conversationMeta[msg.ConversationID] = fallbackConversationMeta{
				Title: msg.ConversationTitle,
				Agent: agent,
				Model: msg.Model,
			}
		}
		pathTokens := make(map[string][]int)
		for pos, tok := range msg.Tokens {
			if !tok.Attributable {
				continue
			}
			for _, matchKey := range tok.MatchKeys {
				idx.tokenSources[matchKey] = append(idx.tokenSources[matchKey], tokenSource{
					msgIdx:   i,
					tokenPos: pos,
					matchLen: len(strings.Split(strings.SplitN(matchKey, "\x1f", 2)[0], "/")),
				})
			}
			for _, matchKey := range tok.StyleMatchKeys {
				idx.styleTokenSources[matchKey] = append(idx.styleTokenSources[matchKey], tokenSource{
					msgIdx:   i,
					tokenPos: pos,
					matchLen: len(strings.Split(strings.SplitN(matchKey, "\x1f", 2)[0], "/")),
				})
			}
			pathTokens[tokenBucketKey(tok.Path, tok.Sign)] = append(pathTokens[tokenBucketKey(tok.Path, tok.Sign)], pos)
			if tok.Norm != "" {
				idx.normSources[tok.Norm]++
				agents := idx.normAgents[tok.Norm]
				if agents == nil {
					agents = make(map[string]int)
					idx.normAgents[tok.Norm] = agents
				}
				agents[agent]++
				if msg.ConversationID != "" {
					conversations := idx.normConversationCounts[tok.Norm]
					if conversations == nil {
						conversations = make(map[string]int)
						idx.normConversationCounts[tok.Norm] = conversations
					}
					conversations[msg.ConversationID]++
					for _, alias := range buildDiffPathAliases(tok.Path) {
						byPath := idx.pathNormConversationCounts[alias]
						if byPath == nil {
							byPath = make(map[string]map[string]int)
							idx.pathNormConversationCounts[alias] = byPath
						}
						byNorm := byPath[tok.Norm]
						if byNorm == nil {
							byNorm = make(map[string]int)
							byPath[tok.Norm] = byNorm
						}
						byNorm[msg.ConversationID]++
					}
				}
			}
		}
		idx.tokensByBucket[i] = pathTokens
	}
	return idx
}

func attributeCommitToMessages(
	commitTokens []diffToken,
	messages []messageDiff,
	windowStart, windowEnd int64,
) ([]commitContributionMessage, int, map[string]commitFileCoverage, map[string]map[string]int, map[string]int, map[string][]string) {
	idx := buildMessageIndex(messages, windowStart, windowEnd)
	return attributeCommitToMessagesWithIndex(commitTokens, idx, windowStart, windowEnd)
}

func attributeCommitToMessagesWithIndex(
	commitTokens []diffToken,
	idx *messageIndex,
	windowStart, windowEnd int64,
) ([]commitContributionMessage, int, map[string]commitFileCoverage, map[string]map[string]int, map[string]int, map[string][]string) {
	messages := idx.messages

	matchedLines := 0
	messageTokenUsed := make(map[int][]bool)
	commitMatched := make([]bool, len(commitTokens))
	// Copy normSources so per-commit consumption doesn't affect the shared index.
	normSources := make(map[string]int, len(idx.normSources))
	for k, v := range idx.normSources {
		normSources[k] = v
	}

	contribByIndex := make(map[int]*commitContributionMessage)
	fileCoverageByPath := make(map[string]commitFileCoverage)
	exactConversationByPath := make(map[string]map[string]int)
	type fileAgentStats struct{ lines int }
	fileAgentByPath := make(map[string]map[string]*fileAgentStats)
	recordFileAgentMatch := func(filePath string, msgIdx int) {
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
		if convID := messages[msgIdx].ConversationID; convID != "" {
			byConv := exactConversationByPath[filePath]
			if byConv == nil {
				byConv = make(map[string]int)
				exactConversationByPath[filePath] = byConv
			}
			byConv[convID]++
		}
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

		source, ok := selectExactTokenSource(tok, idx, messages, messageTokenUsed, windowStart, windowEnd)
		if !ok {
			fileCoverageByPath[path] = fileCov
			continue
		}
		if messageTokenUsed[source.msgIdx] == nil {
			messageTokenUsed[source.msgIdx] = make([]bool, len(messages[source.msgIdx].Tokens))
		}
		messageTokenUsed[source.msgIdx][source.tokenPos] = true
		commitMatched[tokIdx] = true

		matchedLines++
		fileCov.Removed++
		fileCoverageByPath[path] = fileCov
		recordFileAgentMatch(path, source.msgIdx)
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
					if msg.Timestamp <= windowStart || msg.Timestamp > windowEnd {
						continue
					}
					positions := idx.tokensByBucket[msgIdx][bucketKey]
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
							if messageTokenUsed[msgIdx] == nil {
								messageTokenUsed[msgIdx] = make([]bool, len(msg.Tokens))
							}
							if !messageWindowAvailable(messageTokenUsed[msgIdx], windowPositions) {
								continue
							}
							if concatMessageNorms(msg.Tokens, windowPositions) != commitNorm {
								continue
							}

							for _, ci := range indices[cursor : cursor+commitWindow] {
								commitMatched[ci] = true
								matchedLines++
								fileCov := fileCoverageByPath[path]
								fileCov.Path = path
								fileCov.Removed++
								fileCoverageByPath[path] = fileCov
								recordFileAgentMatch(path, msgIdx)
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
							for range indices[cursor : cursor+commitWindow] {
								contrib.LinesMatched++
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

	// Third pass: recover style-equivalent changes where the authored line only
	// differs in safe quote style, such as CSS attribute selectors using
	// single-vs-double quotes. This pass is lower priority than strict exact
	// matches and formatting-window matches.
	for tokIdx, tok := range commitTokens {
		if commitMatched[tokIdx] || !tok.Attributable || tok.StyleNorm == "" {
			continue
		}
		path := tok.Path
		if path == "" {
			continue
		}
		preferredConversationID := dominantConversationFromCounts(exactConversationByPath[path])
		source, ok := selectStyleTokenSource(tok, preferredConversationID, idx, messages, messageTokenUsed, windowStart, windowEnd)
		if !ok {
			continue
		}
		if messageTokenUsed[source.msgIdx] == nil {
			messageTokenUsed[source.msgIdx] = make([]bool, len(messages[source.msgIdx].Tokens))
		}
		messageTokenUsed[source.msgIdx][source.tokenPos] = true
		commitMatched[tokIdx] = true
		matchedLines++

		fileCov := fileCoverageByPath[path]
		fileCov.Path = path
		fileCov.Removed++
		fileCoverageByPath[path] = fileCov
		recordFileAgentMatch(path, source.msgIdx)

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
			})
		}
		fileCov.AgentSegments = segments
		fileCoverageByPath[filePath] = fileCov
	}

	unmatchedNormsByPath := make(map[string][]string)
	for i, tok := range commitTokens {
		if tok.Path == "" || tok.Norm == "" || !tok.Attributable || commitMatched[i] {
			continue
		}
		unmatchedNormsByPath[tok.Path] = append(unmatchedNormsByPath[tok.Path], tok.Norm)
	}

	return out, matchedLines, fileCoverageByPath, exactConversationByPath, normSources, unmatchedNormsByPath
}

func selectExactTokenSource(
	tok diffToken,
	idx *messageIndex,
	messages []messageDiff,
	messageTokenUsed map[int][]bool,
	windowStart, windowEnd int64,
) (tokenSource, bool) {
	var best exactTokenCandidate
	bestSet := false
	seen := make(map[tokenSource]struct{}, len(tok.MatchKeys))

	for _, matchKey := range tok.MatchKeys {
		for _, source := range idx.tokenSources[matchKey] {
			if _, ok := seen[source]; ok {
				continue
			}
			seen[source] = struct{}{}

			msg := messages[source.msgIdx]
			if msg.Timestamp <= windowStart || msg.Timestamp > windowEnd {
				continue
			}
			if messageTokenUsed[source.msgIdx] != nil && messageTokenUsed[source.msgIdx][source.tokenPos] {
				continue
			}

			msgTok := msg.Tokens[source.tokenPos]
			current := exactTokenCandidate{
				source: source,
				exact:  msgTok.Path == tok.Path,
			}
			if !bestSet || betterExactTokenSource(current, best, messages) {
				best = current
				bestSet = true
			}
		}
	}

	if !bestSet {
		return tokenSource{}, false
	}
	return best.source, true
}

func betterExactTokenSource(current, best exactTokenCandidate, messages []messageDiff) bool {
	if current.exact != best.exact {
		return current.exact
	}
	if current.preferred != best.preferred {
		return current.preferred
	}
	if current.source.matchLen != best.source.matchLen {
		return current.source.matchLen > best.source.matchLen
	}

	currentMsg := messages[current.source.msgIdx]
	bestMsg := messages[best.source.msgIdx]
	if currentMsg.Timestamp != bestMsg.Timestamp {
		return currentMsg.Timestamp > bestMsg.Timestamp
	}
	if currentMsg.ID != bestMsg.ID {
		return currentMsg.ID > bestMsg.ID
	}
	if current.source.msgIdx != best.source.msgIdx {
		return current.source.msgIdx < best.source.msgIdx
	}
	return current.source.tokenPos < best.source.tokenPos
}

func selectStyleTokenSource(
	tok diffToken,
	preferredConversationID string,
	idx *messageIndex,
	messages []messageDiff,
	messageTokenUsed map[int][]bool,
	windowStart, windowEnd int64,
) (tokenSource, bool) {
	if len(tok.StyleMatchKeys) == 0 || tok.StyleNorm == "" {
		return tokenSource{}, false
	}

	var best exactTokenCandidate
	bestSet := false
	seen := make(map[tokenSource]struct{}, len(tok.StyleMatchKeys))

	for _, matchKey := range tok.StyleMatchKeys {
		for _, source := range idx.styleTokenSources[matchKey] {
			if _, ok := seen[source]; ok {
				continue
			}
			seen[source] = struct{}{}

			msg := messages[source.msgIdx]
			if msg.Timestamp <= windowStart || msg.Timestamp > windowEnd {
				continue
			}
			if messageTokenUsed[source.msgIdx] != nil && messageTokenUsed[source.msgIdx][source.tokenPos] {
				continue
			}

			msgTok := msg.Tokens[source.tokenPos]
			if msgTok.StyleNorm == "" || msgTok.StyleNorm != tok.StyleNorm {
				continue
			}
			current := exactTokenCandidate{
				source:    source,
				exact:     msgTok.Path == tok.Path,
				preferred: preferredConversationID != "" && msg.ConversationID == preferredConversationID,
			}
			if !bestSet || betterStyleTokenSource(current, best, messages) {
				best = current
				bestSet = true
			}
		}
	}

	if !bestSet {
		return tokenSource{}, false
	}
	return best.source, true
}

func betterStyleTokenSource(current, best exactTokenCandidate, messages []messageDiff) bool {
	if current.preferred != best.preferred {
		return current.preferred
	}
	return betterExactTokenSource(current, best, messages)
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
	idx := buildMessageIndex(messages, windowStart, windowEnd)
	return attributeCopiedFromAgentFilesWithIndex(files, commitTokens, idx, linesTotal)
}

func attributeCopiedFromAgentFilesWithIndex(
	files []commitFileCoverage,
	commitTokens []diffToken,
	idx *messageIndex,
	linesTotal int,
) []agentCoverageSegment {
	dominantAgent := func(norm string) string {
		agents := idx.normAgents[norm]
		if len(agents) == 0 {
			return ""
		}
		best, bestN := "", 0
		for a, n := range agents {
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
	diffFiles []diffFileInfo,
	fileAgent map[string]commitFileCoverage,
) []commitFileCoverage {
	coverageByPath := make(map[string]commitFileCoverage, len(diffFiles))
	for _, fi := range diffFiles {
		coverageByPath[fi.Path] = commitFileCoverage{
			Path:      fi.Path,
			Added:     fi.Added,
			Removed:   fi.Removed,
			Ignored:   fi.Ignored,
			Moved:     fi.Moved,
			MovedFrom: fi.OldPath,
		}
	}

	filePaths := make([]string, 0, len(coverageByPath))
	for filePath := range coverageByPath {
		filePaths = append(filePaths, filePath)
	}
	sort.Strings(filePaths)

	out := make([]commitFileCoverage, 0, len(filePaths))
	for _, filePath := range filePaths {
		c := coverageByPath[filePath]
		c.LinesTotal = c.Added + c.Removed
		if agent, ok := fileAgent[filePath]; ok {
			c.AttributableLines = agent.Added
		}
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
		}
		out = append(out, c)
	}
	return out
}

func applyFallbackFileCoverage(
	files []commitFileCoverage,
	fileAgent map[string]commitFileCoverage,
	exactConversationByPath map[string]map[string]int,
	unmatchedNormsByPath map[string][]string,
	remainingNorms map[string]int,
	idx *messageIndex,
) ([]commitFileCoverage, int, []string) {
	if len(files) == 0 || len(unmatchedNormsByPath) == 0 || len(remainingNorms) == 0 || idx == nil {
		return files, 0, nil
	}

	conversationCountsByNorm := make(map[string]map[string]int, len(idx.normConversationCounts))
	for norm, counts := range idx.normConversationCounts {
		cloned := make(map[string]int, len(counts))
		for convID, n := range counts {
			cloned[convID] = n
		}
		conversationCountsByNorm[norm] = cloned
	}
	pathConversationCountsByNorm := make(map[string]map[string]map[string]int, len(idx.pathNormConversationCounts))
	for alias, byNorm := range idx.pathNormConversationCounts {
		clonedByNorm := make(map[string]map[string]int, len(byNorm))
		for norm, counts := range byNorm {
			clonedCounts := make(map[string]int, len(counts))
			for convID, n := range counts {
				clonedCounts[convID] = n
			}
			clonedByNorm[norm] = clonedCounts
		}
		pathConversationCountsByNorm[alias] = clonedByNorm
	}

	convSeen := make(map[string]bool)
	var convIDs []string
	extraLines := 0

	for i := range files {
		c := &files[i]
		if c.Ignored || c.Moved {
			continue
		}

		norms := unmatchedNormsByPath[c.Path]
		minMatch := fallbackMinMatch(c.LinesTotal, len(norms))
		if len(norms) < minMatch {
			continue
		}

		type consumedLine struct {
			norm   string
			convID string
		}
		consumed := make([]consumedLine, 0, len(norms))
		agentLines := make(map[string]int)
		convLines := make(map[string]int)
		fallbackMatched := 0
		preferredConvID := dominantConversationFromCounts(exactConversationByPath[c.Path])
		pathAliases := buildDiffPathAliases(c.Path)

		for _, norm := range norms {
			if remainingNorms[norm] <= 0 {
				continue
			}
			remainingNorms[norm]--
			convID := dominantConversationForFileNorm(norm, preferredConvID, pathAliases, pathConversationCountsByNorm, conversationCountsByNorm)
			if convID != "" {
				convLines[convID]++
				if meta, ok := idx.conversationMeta[convID]; ok {
					agent := strings.TrimSpace(meta.Agent)
					if agent == "" {
						agent = "unknown"
					}
					agentLines[agent]++
				}
			}
			consumed = append(consumed, consumedLine{norm: norm, convID: convID})
			fallbackMatched++
		}

		if fallbackMatched < minMatch {
			for _, item := range consumed {
				remainingNorms[item.norm]++
				restoreConversationCount(item.norm, item.convID, pathAliases, pathConversationCountsByNorm, conversationCountsByNorm)
			}
			continue
		}

		attributableTotal := len(norms)
		if exact, ok := fileAgent[c.Path]; ok && exact.Added > 0 {
			attributableTotal = exact.Added
		} else if attributableTotal == 0 {
			attributableTotal = c.LinesTotal
		}
		c.LinesFromAgent += fallbackMatched
		if c.AttributableLines == 0 {
			c.AttributableLines = attributableTotal
		}
		c.CopiedFromAgent = true
		c.LinePercent = percentage(c.LinesFromAgent, attributableTotal)
		c.AgentSegments = mergeFileAgentSegments(c.AgentSegments, agentLines, attributableTotal)
		extraLines += fallbackMatched

		for convID, lines := range convLines {
			if lines < 2 || convSeen[convID] {
				continue
			}
			convSeen[convID] = true
			convIDs = append(convIDs, convID)
		}
	}

	sort.Strings(convIDs)
	return files, extraLines, convIDs
}

func summarizeCommitAgentSegments(files []commitFileCoverage, linesTotal int) []agentCoverageSegment {
	overallAgentLines := make(map[string]int)
	for _, f := range files {
		if f.Ignored || f.Moved {
			continue
		}
		for _, seg := range f.AgentSegments {
			if seg.LinesFromAgent <= 0 {
				continue
			}
			overallAgentLines[seg.Agent] += seg.LinesFromAgent
		}
	}
	if len(overallAgentLines) == 0 {
		return nil
	}
	agents := make([]string, 0, len(overallAgentLines))
	for agent := range overallAgentLines {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	out := make([]agentCoverageSegment, 0, len(agents))
	for _, agent := range agents {
		out = append(out, agentCoverageSegment{
			Agent:          agent,
			LinesFromAgent: overallAgentLines[agent],
			LinePercent:    percentage(overallAgentLines[agent], linesTotal),
		})
	}
	return out
}

func fallbackMinMatch(linesTotal, normCount int) int {
	minMatch := 10
	if linesTotal < 10 && normCount >= 2 && normCount < 10 {
		minMatch = normCount
	}
	return minMatch
}

func dominantConversationForNorm(norm string, conversationCountsByNorm map[string]map[string]int) string {
	counts := conversationCountsByNorm[norm]
	return dominantConversationFromCounts(counts)
}

func dominantConversationFromCounts(counts map[string]int) string {
	bestID := ""
	bestCount := 0
	for convID, n := range counts {
		if n <= 0 {
			continue
		}
		if n > bestCount || (n == bestCount && (bestID == "" || convID < bestID)) {
			bestID = convID
			bestCount = n
		}
	}
	return bestID
}

func dominantConversationForFileNorm(
	norm string,
	preferredConvID string,
	pathAliases []string,
	pathConversationCountsByNorm map[string]map[string]map[string]int,
	conversationCountsByNorm map[string]map[string]int,
) string {
	if preferredConvID != "" {
		for _, alias := range pathAliases {
			if counts := pathConversationCountsByNorm[alias][norm]; counts != nil && counts[preferredConvID] > 0 {
				counts[preferredConvID]--
				if global := conversationCountsByNorm[norm]; global != nil && global[preferredConvID] > 0 {
					global[preferredConvID]--
				}
				return preferredConvID
			}
		}
		if counts := conversationCountsByNorm[norm]; counts != nil && counts[preferredConvID] > 0 {
			counts[preferredConvID]--
			return preferredConvID
		}
	}

	for _, alias := range pathAliases {
		if counts := pathConversationCountsByNorm[alias][norm]; counts != nil {
			if convID := dominantConversationFromCounts(counts); convID != "" {
				counts[convID]--
				if global := conversationCountsByNorm[norm]; global != nil && global[convID] > 0 {
					global[convID]--
				}
				return convID
			}
		}
	}

	counts := conversationCountsByNorm[norm]
	convID := dominantConversationFromCounts(counts)
	if convID != "" {
		counts[convID]--
	}
	return convID
}

func restoreConversationCount(
	norm, convID string,
	pathAliases []string,
	pathConversationCountsByNorm map[string]map[string]map[string]int,
	conversationCountsByNorm map[string]map[string]int,
) {
	if convID == "" {
		return
	}
	if counts := conversationCountsByNorm[norm]; counts != nil {
		counts[convID]++
	}
	for _, alias := range pathAliases {
		if counts := pathConversationCountsByNorm[alias][norm]; counts != nil {
			counts[convID]++
			return
		}
	}
}

func mergeFileAgentSegments(
	existing []agentCoverageSegment,
	extra map[string]int,
	attributableTotal int,
) []agentCoverageSegment {
	combined := make(map[string]int, len(existing)+len(extra))
	for _, seg := range existing {
		if seg.LinesFromAgent > 0 {
			combined[seg.Agent] += seg.LinesFromAgent
		}
	}
	for agent, lines := range extra {
		if lines > 0 {
			combined[agent] += lines
		}
	}
	if len(combined) == 0 {
		return nil
	}
	agents := make([]string, 0, len(combined))
	for agent := range combined {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	out := make([]agentCoverageSegment, 0, len(agents))
	for _, agent := range agents {
		out = append(out, agentCoverageSegment{
			Agent:          agent,
			LinesFromAgent: combined[agent],
			LinePercent:    percentage(combined[agent], attributableTotal),
		})
	}
	return out
}
