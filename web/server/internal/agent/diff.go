package agent

import (
	"encoding/json"
	"fmt"
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
		if converted, ok := extractApplyPatchDiff(content); ok {
			return converted, true
		}
		if converted, ok := extractShellHeredocWriteDiff(content); ok {
			return converted, true
		}
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

func extractApplyPatchDiff(content string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	start := -1
	end := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start < 0 && trimmed == "*** Begin Patch" {
			start = i + 1
			continue
		}
		if start >= 0 && trimmed == "*** End Patch" {
			end = i
			break
		}
	}
	if start < 0 || end <= start {
		return "", false
	}

	type section struct {
		oldPath string
		newPath string
		lines   []string
	}

	sections := make([]section, 0, 2)
	var current *section

	flush := func() {
		if current == nil || current.newPath == "" || len(current.lines) == 0 {
			return
		}
		sections = append(sections, *current)
	}

	for i := start; i < end; i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "*** Update File: "):
			flush()
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
			current = &section{oldPath: path, newPath: path}
		case strings.HasPrefix(line, "*** Add File: "):
			flush()
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
			current = &section{oldPath: "", newPath: path}
		case strings.HasPrefix(line, "*** Delete File: "):
			flush()
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			current = &section{oldPath: path, newPath: ""}
		case strings.HasPrefix(line, "*** Move to: "):
			if current != nil {
				current.newPath = strings.TrimSpace(strings.TrimPrefix(line, "*** Move to: "))
			}
		case strings.HasPrefix(line, "@@"):
			if current != nil {
				current.lines = append(current.lines, line)
			}
		case strings.HasPrefix(line, " "), strings.HasPrefix(line, "+"), strings.HasPrefix(line, "-"):
			if current != nil {
				current.lines = append(current.lines, line)
			}
		}
	}
	flush()

	if len(sections) == 0 {
		return "", false
	}

	var out strings.Builder
	for _, sec := range sections {
		oldPathRaw := sec.oldPath
		newPathRaw := sec.newPath
		oldPath := oldPathRaw
		newPath := newPathRaw
		if oldPath == "" {
			oldPath = newPath
		}
		if newPath == "" {
			newPath = oldPath
		}
		if oldPath == "" || newPath == "" {
			continue
		}

		if out.Len() > 0 {
			out.WriteString("\n")
		}
		out.WriteString("diff --git a/")
		out.WriteString(oldPath)
		out.WriteString(" b/")
		out.WriteString(newPath)
		out.WriteString("\n")
		if oldPathRaw == "" {
			out.WriteString("--- /dev/null\n")
		} else {
			out.WriteString("--- a/")
			out.WriteString(oldPath)
			out.WriteString("\n")
		}
		if newPathRaw == "" {
			out.WriteString("+++ /dev/null\n")
		} else {
			out.WriteString("+++ b/")
			out.WriteString(newPath)
			out.WriteString("\n")
		}
		for _, line := range sec.lines {
			out.WriteString(line)
			out.WriteString("\n")
		}
	}

	diff := strings.TrimSpace(out.String())
	if diff == "" || !looksLikeUnifiedDiff(diff) {
		return "", false
	}
	return diff, true
}

func extractShellHeredocWriteDiff(content string) (string, bool) {
	content = strings.ReplaceAll(content, "\r\n", "\n")

	idx := strings.Index(content, "cat >")
	if idx < 0 {
		return "", false
	}
	rest := content[idx+len("cat >"):]

	rest = strings.TrimLeft(rest, " \t")
	if rest == "" {
		return "", false
	}

	path, rem, ok := parseQuotedToken(rest)
	if !ok || path == "" {
		return "", false
	}
	rem = strings.TrimLeft(rem, " \t")
	if !strings.HasPrefix(rem, "<<") {
		return "", false
	}
	rem = strings.TrimPrefix(rem, "<<")
	rem = strings.TrimLeft(rem, " \t")
	if rem == "" {
		return "", false
	}

	delim, rem, ok := parseQuotedToken(rem)
	if !ok || delim == "" {
		return "", false
	}
	if i := strings.IndexByte(rem, '\n'); i >= 0 {
		rem = rem[i+1:]
	} else {
		return "", false
	}

	bodyEnd := strings.Index(rem, "\n"+delim+"\n")
	if bodyEnd < 0 {
		if strings.HasSuffix(rem, "\n"+delim) {
			bodyEnd = len(rem) - (len(delim) + 1)
		} else {
			return "", false
		}
	}
	body := rem[:bodyEnd]
	if strings.TrimSpace(body) == "" {
		return "", false
	}
	lines := strings.Split(body, "\n")

	var out strings.Builder
	out.WriteString("diff --git a/")
	out.WriteString(path)
	out.WriteString(" b/")
	out.WriteString(path)
	out.WriteString("\n--- a/")
	out.WriteString(path)
	out.WriteString("\n+++ b/")
	out.WriteString(path)
	out.WriteString("\n")
	out.WriteString(fmt.Sprintf("@@ -1,0 +1,%d @@\n", len(lines)))
	for _, line := range lines {
		out.WriteString("+")
		out.WriteString(line)
		out.WriteString("\n")
	}
	return strings.TrimSpace(out.String()), true
}

func parseQuotedToken(s string) (token string, remainder string, ok bool) {
	if s == "" {
		return "", "", false
	}
	switch s[0] {
	case '\'', '"':
		quote := s[0]
		end := strings.IndexByte(s[1:], quote)
		if end < 0 {
			return "", "", false
		}
		end++
		return s[1:end], s[end+1:], true
	default:
		i := strings.IndexAny(s, " \t\n")
		if i < 0 {
			return s, "", true
		}
		return s[:i], s[i:], true
	}
}
