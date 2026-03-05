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
	return TruncateTitle(strings.TrimSpace(text))
}

// TruncateTitle truncates a string to MaxTitleLen runes, appending "..." if truncated.
func TruncateTitle(s string) string {
	if utf8.RuneCountInString(s) > MaxTitleLen {
		return string([]rune(s)[:MaxTitleLen]) + "..."
	}
	return s
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
