package agent

import (
	"encoding/json"
	"strings"
)

// ExtractReliableDiff extracts unified diff output when it can be identified
// with high confidence. It prefers fenced diff/patch blocks and otherwise
// accepts only standalone unified diffs.
func ExtractReliableDiff(content string) (string, bool) {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	if content == "" {
		return "", false
	}

	blocks := extractFencedDiffBlocks(content)
	if len(blocks) > 0 {
		accepted := make([]string, 0, len(blocks))
		for _, block := range blocks {
			block = strings.TrimSpace(block)
			if block == "" {
				continue
			}
			if !looksLikeUnifiedDiff(block) {
				continue
			}
			accepted = append(accepted, block)
		}
		if len(accepted) == 0 {
			return "", false
		}
		return strings.Join(accepted, "\n\n"), true
	}

	if !looksLikeUnifiedDiff(content) {
		return "", false
	}
	return content, true
}

func FormatDiffMessage(diff string) string {
	diff = strings.TrimSpace(diff)
	if diff == "" {
		return ""
	}
	return "```diff\n" + diff + "\n```"
}

// ExtractReliableDiffFromJSON scans all string fields in a JSON document and
// returns the largest reliable unified diff found.
func ExtractReliableDiffFromJSON(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", false
	}

	best := ""
	walkStrings(value, func(s string) {
		diff, ok := ExtractReliableDiff(s)
		if !ok {
			return
		}
		if len(diff) > len(best) {
			best = diff
		}
	})

	if best == "" {
		return "", false
	}
	return best, true
}

func extractFencedDiffBlocks(content string) []string {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, 2)

	inFence := false
	var fence strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			lang := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "```")))
			if !inFence {
				if lang == "diff" || lang == "patch" || lang == "udiff" {
					inFence = true
					fence.Reset()
				}
				continue
			}

			inFence = false
			block := strings.TrimSpace(fence.String())
			if block != "" {
				result = append(result, block)
			}
			continue
		}

		if inFence {
			fence.WriteString(line)
			fence.WriteString("\n")
		}
	}

	return result
}

func looksLikeUnifiedDiff(content string) bool {
	lines := strings.Split(content, "\n")

	var hasDiffHeader bool
	var hasHunk bool
	var hasOldFile bool
	var hasNewFile bool
	var added int
	var removed int

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "diff --git "):
			hasDiffHeader = true
		case strings.HasPrefix(line, "@@"):
			hasHunk = true
		case strings.HasPrefix(line, "--- "):
			hasOldFile = true
		case strings.HasPrefix(line, "+++ "):
			hasNewFile = true
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}

	if added == 0 && removed == 0 {
		return false
	}

	if hasDiffHeader && (hasHunk || (hasOldFile && hasNewFile)) {
		return true
	}
	if hasOldFile && hasNewFile && hasHunk {
		return true
	}
	return false
}

func walkStrings(v any, fn func(string)) {
	switch x := v.(type) {
	case string:
		fn(x)
		s := strings.TrimSpace(x)
		if (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) || (strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) {
			var nested any
			if err := json.Unmarshal([]byte(s), &nested); err == nil {
				walkStrings(nested, fn)
			}
		}
	case map[string]any:
		for _, nested := range x {
			walkStrings(nested, fn)
		}
	case []any:
		for _, nested := range x {
			walkStrings(nested, fn)
		}
	}
}
