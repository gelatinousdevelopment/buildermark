package agent

import (
	"strings"
	"unicode/utf8"
)

// MaxTitleLen is the maximum character length for a title derived from the first prompt.
const MaxTitleLen = 1000

// TitleFromPrompt extracts a title from a user prompt by taking the first
// MaxTitleLen characters, appending an ellipsis when truncated.
func TitleFromPrompt(text string) string {
	return truncateTitle(strings.TrimSpace(text))
}

// truncateTitle truncates a string to MaxTitleLen runes, appending "..." if truncated.
func truncateTitle(s string) string {
	if utf8.RuneCountInString(s) > MaxTitleLen {
		return string([]rune(s)[:MaxTitleLen]) + "..."
	}
	return s
}

// TitleFromPlanPrompt extracts a plan heading from a user message that starts
// with "Implement the following plan:". It returns the text after "# Plan: " or
// "# " from the first heading line, or "" if no match.
func TitleFromPlanPrompt(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "Implement the following plan:") {
		return ""
	}
	for _, line := range strings.Split(text, "\n")[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# Plan: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# Plan: "))
		}
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
		break
	}
	return ""
}

// FirstNonEmpty returns the first non-empty (after trimming) value, or "".
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}
