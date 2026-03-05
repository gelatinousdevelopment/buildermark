package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// ExtractReliableDiff extracts unified diff output when it can be identified
// with high confidence. It prefers fenced diff/patch blocks and otherwise
// accepts only standalone unified diffs.
func ExtractReliableDiff(content string) (string, bool) {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	if content == "" {
		return "", false
	}

	if diff, ok := extractDirectUnifiedDiff(content); ok {
		return diff, true
	}
	if converted, ok := extractApplyPatchDiff(content); ok {
		return converted, true
	}
	if converted, ok := extractShellHeredocWriteDiff(content); ok {
		return converted, true
	}
	return "", false
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
	diffs := ExtractReliableDiffsFromJSON(raw)
	if len(diffs) == 0 {
		return "", false
	}
	best := diffs[0]
	for _, diff := range diffs[1:] {
		if len(diff) > len(best) {
			best = diff
		}
	}
	return best, true
}

// ExtractReliableDiffsFromJSON scans all string fields in a JSON document and
// returns all reliable unified diffs found in stable order (deduplicated).
func ExtractReliableDiffsFromJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil
	}

	out := make([]string, 0, 4)
	seen := make(map[string]struct{}, 8)
	add := func(diff string) {
		diff = strings.TrimSpace(diff)
		if diff == "" {
			return
		}
		if _, exists := seen[diff]; exists {
			return
		}
		seen[diff] = struct{}{}
		out = append(out, diff)
	}

	walkStrings(value, func(s string) {
		diff, ok := extractDirectUnifiedDiff(s)
		if !ok {
			return
		}
		add(diff)
	})

	for _, diff := range extractStructuredPatchDiffsFromValue(value) {
		add(diff)
	}
	for _, diff := range extractOldNewEditDiffsFromValue(value) {
		add(diff)
	}
	for _, diff := range extractApplyPatchDiffsFromValue(value) {
		add(diff)
	}
	for _, diff := range extractShellHeredocWriteDiffsFromValue(value) {
		add(diff)
	}

	if len(out) == 0 {
		for _, diff := range extractFileSnapshotDiffsFromValue(value) {
			add(diff)
		}
	}

	return out
}

func isToolUseBlock(m map[string]any) bool {
	typ, _ := m["type"].(string)
	return typ == "tool_use"
}

func extractOldNewEditDiffFromValue(v any) (string, bool) {
	diffs := extractOldNewEditDiffsFromValue(v)
	if len(diffs) == 0 {
		return "", false
	}
	return diffs[0], true
}

func extractOldNewEditDiffsFromValue(v any) []string {
	out := make([]string, 0, 2)
	seen := make(map[string]struct{}, 4)
	add := func(diff string) {
		diff = strings.TrimSpace(diff)
		if diff == "" {
			return
		}
		if _, exists := seen[diff]; exists {
			return
		}
		seen[diff] = struct{}{}
		out = append(out, diff)
	}

	var walk func(node any, inheritedCWD string)
	walk = func(node any, inheritedCWD string) {
		switch x := node.(type) {
		case map[string]any:
			if isToolUseBlock(x) {
				return // skip — the tool_result will have better data
			}
			cwd := inheritedCWD
			if s, ok := x["cwd"].(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					cwd = trimmed
				}
			}
			if diff, ok := extractOldNewEditDiffFromMap(x, cwd); ok {
				add(diff)
			}
			for _, key := range sortedMapKeys(x) {
				walk(x[key], cwd)
			}
		case []any:
			for _, nested := range x {
				walk(nested, inheritedCWD)
			}
		}
	}

	walk(v, "")
	return out
}

func extractOldNewEditDiffFromMap(obj map[string]any, cwd string) (string, bool) {
	if hasAnyKey(obj, "structuredPatch") {
		return "", false
	}

	filePath := ""
	for _, key := range []string{"filePath", "file_path", "path"} {
		if s, ok := obj[key].(string); ok {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				filePath = trimmed
				break
			}
		}
	}
	if filePath == "" {
		return "", false
	}

	oldText := ""
	newText := ""
	for _, key := range []string{"oldString", "old_string"} {
		if s, ok := obj[key].(string); ok {
			oldText = strings.ReplaceAll(s, "\r\n", "\n")
			break
		}
	}
	for _, key := range []string{"newString", "new_string"} {
		if s, ok := obj[key].(string); ok {
			newText = strings.ReplaceAll(s, "\r\n", "\n")
			break
		}
	}
	if oldText == "" && newText == "" {
		return "", false
	}
	if oldText == newText {
		return "", false
	}

	paths := buildStructuredPatchPathCandidates(filePath, cwd)
	if len(paths) == 0 {
		return "", false
	}

	for _, path := range paths {
		diff, ok := buildUnifiedDiffFromStrings(oldText, newText, path)
		if !ok {
			continue
		}
		return diff, true
	}
	return "", false
}

func buildUnifiedDiffFromStrings(oldText, newText, repoPath string) (string, bool) {
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return "", false
	}

	oldNorm := strings.ReplaceAll(oldText, "\r\n", "\n")
	newNorm := strings.ReplaceAll(newText, "\r\n", "\n")
	if oldNorm == newNorm {
		return "", false
	}
	if !strings.HasSuffix(oldNorm, "\n") {
		oldNorm += "\n"
	}
	if !strings.HasSuffix(newNorm, "\n") {
		newNorm += "\n"
	}

	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(oldNorm),
		B:        difflib.SplitLines(newNorm),
		FromFile: "a/" + repoPath,
		ToFile:   "b/" + repoPath,
		Context:  3,
	})
	if err != nil {
		return "", false
	}
	diff = strings.TrimSpace(diff)
	if diff == "" {
		return "", false
	}
	diff = "diff --git a/" + repoPath + " b/" + repoPath + "\n" + diff
	if !looksLikeUnifiedDiff(diff) {
		return "", false
	}
	return diff, true
}

func extractStructuredPatchDiffFromValue(v any) (string, bool) {
	diffs := extractStructuredPatchDiffsFromValue(v)
	if len(diffs) == 0 {
		return "", false
	}
	return diffs[0], true
}

func extractFileSnapshotDiffFromValue(v any) (string, bool) {
	diffs := extractFileSnapshotDiffsFromValue(v)
	if len(diffs) == 0 {
		return "", false
	}
	return diffs[0], true
}

func extractApplyPatchDiffsFromValue(v any) []string {
	return extractContextualStringDiffs(v, extractApplyPatchDiffWithCWD)
}

func extractShellHeredocWriteDiffsFromValue(v any) []string {
	return extractContextualStringDiffs(v, extractShellHeredocWriteDiffWithCWD)
}

func extractStructuredPatchDiffsFromValue(v any) []string {
	out := make([]string, 0, 2)
	seen := make(map[string]struct{}, 4)
	add := func(diff string) {
		diff = strings.TrimSpace(diff)
		if diff == "" {
			return
		}
		if _, exists := seen[diff]; exists {
			return
		}
		seen[diff] = struct{}{}
		out = append(out, diff)
	}

	var walk func(node any, inheritedCWD string)
	walk = func(node any, inheritedCWD string) {
		switch x := node.(type) {
		case map[string]any:
			if isToolUseBlock(x) {
				return // skip — the tool_result will have better data
			}
			cwd := inheritedCWD
			if s, ok := x["cwd"].(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					cwd = trimmed
				}
			}

			if diff, ok := extractStructuredPatchDiffFromMap(x, cwd); ok {
				add(diff)
			}
			for _, key := range sortedMapKeys(x) {
				walk(x[key], cwd)
			}
		case []any:
			for _, nested := range x {
				walk(nested, inheritedCWD)
			}
		}
	}

	walk(v, "")
	return out
}

func extractFileSnapshotDiffsFromValue(v any) []string {
	out := make([]string, 0, 2)
	seen := make(map[string]struct{}, 4)
	add := func(diff string) {
		diff = strings.TrimSpace(diff)
		if diff == "" {
			return
		}
		if _, exists := seen[diff]; exists {
			return
		}
		seen[diff] = struct{}{}
		out = append(out, diff)
	}

	var walk func(node any, inheritedCWD string)
	walk = func(node any, inheritedCWD string) {
		switch x := node.(type) {
		case map[string]any:
			if isToolUseBlock(x) {
				return // skip — the tool_result will have better data
			}
			cwd := inheritedCWD
			if s, ok := x["cwd"].(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					cwd = trimmed
				}
			}
			if diff, ok := extractFileSnapshotDiffFromMap(x, cwd); ok {
				add(diff)
			}
			for _, key := range sortedMapKeys(x) {
				walk(x[key], cwd)
			}
		case []any:
			for _, nested := range x {
				walk(nested, inheritedCWD)
			}
		}
	}

	walk(v, "")
	return out
}

type structuredPatchHunk struct {
	oldStart int
	oldLines int
	newStart int
	newLines int
	lines    []string
}

func extractStructuredPatchDiffFromMap(obj map[string]any, cwd string) (string, bool) {
	rawPatches, ok := obj["structuredPatch"].([]any)
	if !ok || len(rawPatches) == 0 {
		return "", false
	}

	filePath := ""
	for _, key := range []string{"filePath", "file_path", "path"} {
		if s, ok := obj[key].(string); ok {
			filePath = strings.TrimSpace(s)
			if filePath != "" {
				break
			}
		}
	}
	if filePath == "" {
		return "", false
	}

	hunks := make([]structuredPatchHunk, 0, len(rawPatches))
	hasChange := false
	for _, raw := range rawPatches {
		patchObj, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		lines, ok := toStringSlice(patchObj["lines"])
		if !ok || len(lines) == 0 {
			continue
		}
		for _, line := range lines {
			if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
				hasChange = true
				break
			}
		}
		hunks = append(hunks, structuredPatchHunk{
			oldStart: toIntDefault(patchObj["oldStart"], 1),
			oldLines: toIntDefault(patchObj["oldLines"], 0),
			newStart: toIntDefault(patchObj["newStart"], 1),
			newLines: toIntDefault(patchObj["newLines"], 0),
			lines:    lines,
		})
	}

	if len(hunks) == 0 || !hasChange {
		return "", false
	}

	candidates := buildStructuredPatchPathCandidates(filePath, cwd)
	if len(candidates) == 0 {
		return "", false
	}

	path := candidates[0]
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
	for _, h := range hunks {
		out.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", h.oldStart, h.oldLines, h.newStart, h.newLines))
		for _, line := range h.lines {
			if line == "" {
				out.WriteString(" \n")
				continue
			}
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
				out.WriteString(line)
			} else {
				out.WriteString(" ")
				out.WriteString(line)
			}
			out.WriteString("\n")
		}
	}

	diff := strings.TrimSpace(out.String())
	if diff == "" || !looksLikeUnifiedDiff(diff) {
		return "", false
	}
	return diff, true
}

func extractFileSnapshotDiffFromMap(obj map[string]any, cwd string) (string, bool) {
	filePath := ""
	for _, key := range []string{"filePath", "file_path", "path"} {
		if s, ok := obj[key].(string); ok {
			filePath = strings.TrimSpace(s)
			if filePath != "" {
				break
			}
		}
	}
	if filePath == "" {
		return "", false
	}

	rawContent := ""
	for _, key := range []string{"content", "fileContent", "file_content", "text"} {
		if s, ok := obj[key].(string); ok {
			rawContent = strings.ReplaceAll(s, "\r\n", "\n")
			break
		}
	}
	if strings.TrimSpace(rawContent) == "" {
		return "", false
	}

	lines := strings.Split(rawContent, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return "", false
	}

	stripped, catNumbered := stripCatNumbering(lines)
	if catNumbered {
		lines = stripped
	}
	mutationHints := hasAnyKey(obj,
		"oldString", "newString", "replaceAll", "userModified",
		"structuredPatch", "oldStart", "newStart", "oldLines", "newLines",
	)
	readHints := hasAnyKey(obj, "numLines", "totalLines", "startLine", "endLine")
	if readHints && !mutationHints && !catNumbered {
		// Read-style file snapshots (line-count metadata without mutation hints)
		// should not be treated as diffs.
		return "", false
	}
	if !catNumbered && !mutationHints {
		// Restrict plain content snapshots to payloads with explicit mutation
		// signals to avoid generating pseudo-diffs from file reads.
		return "", false
	}

	hasAnyContent := false
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			hasAnyContent = true
			break
		}
	}
	if !hasAnyContent {
		return "", false
	}

	candidates := buildStructuredPatchPathCandidates(filePath, cwd)
	if len(candidates) == 0 {
		return "", false
	}

	p := candidates[0]
	var out strings.Builder
	out.WriteString("diff --git a/")
	out.WriteString(p)
	out.WriteString(" b/")
	out.WriteString(p)
	out.WriteString("\n--- /dev/null\n+++ b/")
	out.WriteString(p)
	out.WriteString("\n")
	out.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(lines)))
	for _, line := range lines {
		out.WriteString("+")
		out.WriteString(line)
		out.WriteString("\n")
	}

	diff := strings.TrimSpace(out.String())
	if diff == "" || !looksLikeUnifiedDiff(diff) {
		return "", false
	}
	return diff, true
}

var catNumberPrefixPattern = regexp.MustCompile(`^\s*\d+\s*(?:→|\t|\||:)\s?`)

func stripCatNumbering(lines []string) ([]string, bool) {
	out := make([]string, 0, len(lines))
	nonEmpty := 0
	matched := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, line)
			continue
		}
		nonEmpty++
		if loc := catNumberPrefixPattern.FindStringIndex(line); loc != nil && loc[0] == 0 {
			out = append(out, line[loc[1]:])
			matched++
			continue
		}
		out = append(out, line)
	}
	if nonEmpty == 0 {
		return out, false
	}
	// Require a strong signal before stripping; otherwise preserve content.
	if nonEmpty < 3 || matched*100 < nonEmpty*70 {
		return lines, false
	}
	return out, true
}

func hasAnyKey(obj map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := obj[key]; ok {
			return true
		}
	}
	return false
}

func toStringSlice(v any) ([]string, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func toIntDefault(v any, fallback int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return fallback
	}
}

func buildStructuredPatchPathCandidates(filePath, cwd string) []string {
	normalize := func(path string) string {
		path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
		path = strings.TrimPrefix(path, "./")
		path = strings.TrimPrefix(path, "/")
		return path
	}
	addCandidate := func(candidates *[]string, seen map[string]struct{}, path string) {
		n := normalize(path)
		if n == "" {
			return
		}
		if _, exists := seen[n]; exists {
			return
		}
		seen[n] = struct{}{}
		*candidates = append(*candidates, n)
	}

	path := strings.TrimSpace(filePath)
	if path == "" {
		return nil
	}
	candidates := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)

	// Relative file paths from tool payloads are already canonical enough.
	if !filepath.IsAbs(path) {
		addCandidate(&candidates, seen, path)
		if cwd = strings.TrimSpace(cwd); cwd != "" {
			addCandidate(&candidates, seen, filepath.Join(cwd, path))
			if root, ok := FindGitRoot(cwd); ok {
				addCandidate(&candidates, seen, filepath.Join(root, path))
			}
		}
		return candidates
	}

	absFile := filepath.Clean(path)
	if cwd = strings.TrimSpace(cwd); cwd != "" {
		if root, ok := FindGitRoot(cwd); ok {
			if rel, ok := relIfContained(root, absFile); ok {
				addCandidate(&candidates, seen, rel)
			}
		}
		if rel, ok := relIfContained(cwd, absFile); ok {
			addCandidate(&candidates, seen, rel)
		}
	}
	if root, ok := FindGitRoot(filepath.Dir(absFile)); ok {
		if rel, ok := relIfContained(root, absFile); ok {
			addCandidate(&candidates, seen, rel)
		}
	}

	addCandidate(&candidates, seen, absFile)
	return candidates
}

func relIfContained(base, target string) (string, bool) {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(target))
	if err != nil {
		return "", false
	}
	if rel == "." || rel == "" {
		return "", false
	}
	prefix := ".." + string(filepath.Separator)
	if rel == ".." || strings.HasPrefix(rel, prefix) {
		return "", false
	}
	return rel, true
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
	return extractApplyPatchDiffWithCWD(content, "")
}

func extractApplyPatchDiffWithCWD(content, cwd string) (string, bool) {
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
		oldPath := resolveStructuredPath(oldPathRaw, cwd)
		newPath := resolveStructuredPath(newPathRaw, cwd)
		if oldPath == "" && newPath != "" {
			oldPath = newPath
		}
		if newPath == "" && oldPath != "" {
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
	return extractShellHeredocWriteDiffWithCWD(content, "")
}

func extractShellHeredocWriteDiffWithCWD(content, cwd string) (string, bool) {
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
	path = resolveStructuredPath(path, cwd)
	if path == "" {
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

func extractDirectUnifiedDiff(content string) (string, bool) {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	if content == "" {
		return "", false
	}

	blocks := extractFencedDiffBlocks(content)
	if len(blocks) > 0 {
		accepted := make([]string, 0, len(blocks))
		for _, block := range blocks {
			block = strings.TrimSpace(block)
			if block == "" || !looksLikeUnifiedDiff(block) {
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

func extractContextualStringDiffs(v any, extractor func(string, string) (string, bool)) []string {
	out := make([]string, 0, 2)
	seen := make(map[string]struct{}, 4)
	add := func(diff string) {
		diff = strings.TrimSpace(diff)
		if diff == "" {
			return
		}
		if _, exists := seen[diff]; exists {
			return
		}
		seen[diff] = struct{}{}
		out = append(out, diff)
	}

	var walk func(node any, inheritedCWD string)
	walk = func(node any, inheritedCWD string) {
		switch x := node.(type) {
		case string:
			if diff, ok := extractor(x, inheritedCWD); ok {
				add(diff)
			}
			s := strings.TrimSpace(x)
			if (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) || (strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) {
				var nested any
				if err := json.Unmarshal([]byte(s), &nested); err == nil {
					walk(nested, inheritedCWD)
				}
			}
		case map[string]any:
			if isToolUseBlock(x) {
				return
			}
			cwd := inheritedCWD
			if s, ok := x["cwd"].(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					cwd = trimmed
				}
			}
			for _, key := range sortedMapKeys(x) {
				walk(x[key], cwd)
			}
		case []any:
			for _, nested := range x {
				walk(nested, inheritedCWD)
			}
		}
	}

	walk(v, "")
	return out
}

func resolveStructuredPath(path, cwd string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	candidates := buildStructuredPatchPathCandidates(path, cwd)
	if len(candidates) > 0 {
		return candidates[0]
	}
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "/")
	return path
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

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
