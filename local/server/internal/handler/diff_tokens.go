package handler

import (
	"path"
	"strings"
	"unicode"
)

// DefaultIgnoreDiffPaths is the hardcoded list of glob patterns ignored when
// the "Ignore default paths" option is enabled for a project.
var DefaultIgnoreDiffPaths = []string{
	"**/.git/**",
	"**/.next/**",
	"**/.nuxt/**",
	"**/__pycache__/**",
	"**/node_modules/**",
	"*.map",
	"*.min.css",
	"*.min.js",
	"bun.lockb",
	"Cargo.lock",
	"composer.lock",
	"Gemfile.lock",
	"go.sum",
	"npm-shrinkwrap.json",
	"package-lock.json",
	"packages.lock.json",
	"paket.lock",
	"pdm.lock",
	"Pipfile.lock",
	"pnpm-lock.yaml",
	"poetry.lock",
	"poetry.lock",
	"yarn.lock",
}

func parseUnifiedDiffTokens(diff string, ignorePatterns []string) []diffToken {
	return parseUnifiedDiffTokensWithFiles(diff, ignorePatterns).Tokens
}

func parseUnifiedDiffTokensWithFiles(diff string, ignorePatterns []string) diffParseResult {
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	lines := strings.Split(diff, "\n")

	oldPath := ""
	newPath := ""
	tokens := make([]diffToken, 0, 64)

	// Track per-file metadata.
	fileMap := make(map[string]*diffFileInfo)
	var fileOrder []string
	ensureFile := func(p string) *diffFileInfo {
		if p == "" {
			return nil
		}
		if fi, ok := fileMap[p]; ok {
			return fi
		}
		fi := &diffFileInfo{
			Path:    p,
			Ignored: shouldIgnoreDiffPath(p, ignorePatterns),
		}
		fileMap[p] = fi
		fileOrder = append(fileOrder, p)
		return fi
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				oldPath = parseDiffPath(parts[2])
				newPath = parseDiffPath(parts[3])
				if newPath != "" {
					ensureFile(newPath)
				} else if oldPath != "" {
					ensureFile(oldPath)
				}
			}
		case strings.HasPrefix(line, "rename from "):
			oldPath = parseDiffPath(strings.TrimPrefix(line, "rename from "))
			if oldPath != "" {
				ensureFile(oldPath)
			}
		case strings.HasPrefix(line, "rename to "):
			newPath = parseDiffPath(strings.TrimPrefix(line, "rename to "))
			if newPath != "" {
				fi := ensureFile(newPath)
				if fi != nil {
					fi.Moved = true
					fi.OldPath = oldPath
				}
			}
		case strings.HasPrefix(line, "--- "):
			oldPath = parseDiffPath(strings.TrimPrefix(line, "--- "))
			if oldPath != "" {
				ensureFile(oldPath)
			}
		case strings.HasPrefix(line, "+++ "):
			newPath = parseDiffPath(strings.TrimPrefix(line, "+++ "))
			if newPath != "" {
				ensureFile(newPath)
			}
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			p := newPath
			if p == "" {
				p = oldPath
			}
			if fi := ensureFile(p); fi != nil {
				fi.Added++
			}
			if shouldIgnoreDiffPath(newPath, ignorePatterns) {
				continue
			}
			if tok, ok := makeDiffToken(newPath, '+', line[1:]); ok {
				tokens = append(tokens, tok)
			}
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			p := oldPath
			if p == "" {
				p = newPath
			}
			if fi := ensureFile(p); fi != nil {
				fi.Removed++
			}
			if shouldIgnoreDiffPath(oldPath, ignorePatterns) {
				continue
			}
			if tok, ok := makeDiffToken(oldPath, '-', line[1:]); ok {
				tokens = append(tokens, tok)
			}
		}
	}

	files := make([]diffFileInfo, 0, len(fileOrder))
	for _, p := range fileOrder {
		files = append(files, *fileMap[p])
	}

	return diffParseResult{Tokens: tokens, Files: files}
}

func parseDiffPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "/dev/null" {
		return ""
	}
	if i := strings.IndexAny(raw, "\t "); i >= 0 {
		raw = raw[:i]
	}
	raw = strings.TrimPrefix(raw, "a/")
	raw = strings.TrimPrefix(raw, "b/")
	return raw
}

func groupIgnoreDiffPatterns(group projectGroup) []string {
	patternSet := make(map[string]struct{})
	patterns := make([]string, 0, 8)

	// Include default patterns if any project in the group has the flag enabled.
	for _, p := range group.Projects {
		if p.IgnoreDefaultDiffPaths {
			for _, pattern := range DefaultIgnoreDiffPaths {
				if _, exists := patternSet[pattern]; exists {
					continue
				}
				patternSet[pattern] = struct{}{}
				patterns = append(patterns, pattern)
			}
			break
		}
	}

	for _, p := range group.Projects {
		for _, pattern := range splitIgnoreDiffPatterns(p.IgnoreDiffPaths) {
			if _, exists := patternSet[pattern]; exists {
				continue
			}
			patternSet[pattern] = struct{}{}
			patterns = append(patterns, pattern)
		}
	}
	return patterns
}

func splitIgnoreDiffPatterns(raw string) []string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		pattern := strings.TrimSpace(strings.ReplaceAll(line, "\\", "/"))
		pattern = strings.TrimPrefix(pattern, "./")
		pattern = strings.TrimPrefix(pattern, "/")
		if pattern == "" {
			continue
		}
		out = append(out, pattern)
	}
	return out
}

func shouldIgnoreDiffPath(diffPath string, patterns []string) bool {
	p := strings.TrimSpace(strings.ReplaceAll(diffPath, "\\", "/"))
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	if p == "" || len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		if globMatchPath(pattern, p) {
			return true
		}
	}
	return false
}

func globMatchPath(pattern, p string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}

	if !strings.Contains(pattern, "/") {
		for _, seg := range strings.Split(p, "/") {
			ok, err := path.Match(pattern, seg)
			if err == nil && ok {
				return true
			}
		}
	}

	return globMatchSegments(splitPathSegments(pattern), splitPathSegments(p))
}

func splitPathSegments(s string) []string {
	s = strings.Trim(strings.ReplaceAll(s, "\\", "/"), "/")
	if s == "" {
		return nil
	}
	return strings.Split(s, "/")
}

func globMatchSegments(patternSegs, pathSegs []string) bool {
	var match func(pi, si int) bool
	match = func(pi, si int) bool {
		if pi == len(patternSegs) {
			return si == len(pathSegs)
		}
		if patternSegs[pi] == "**" {
			if pi == len(patternSegs)-1 {
				return true
			}
			for skip := si; skip <= len(pathSegs); skip++ {
				if match(pi+1, skip) {
					return true
				}
			}
			return false
		}
		if si >= len(pathSegs) {
			return false
		}
		ok, err := path.Match(patternSegs[pi], pathSegs[si])
		if err != nil || !ok {
			return false
		}
		return match(pi+1, si+1)
	}
	return match(0, 0)
}

func makeDiffToken(path string, sign byte, line string) (diffToken, bool) {
	norm := normalizeWhitespace(line)
	if norm == "" {
		return diffToken{}, false
	}
	return diffToken{
		Path:         path,
		Sign:         sign,
		Norm:         norm,
		Key:          path + "\x1f" + string(sign) + "\x1f" + norm,
		Attributable: isAttributionCandidate(norm),
	}, true
}

func isAttributionCandidate(norm string) bool {
	for _, r := range norm {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func normalizeWhitespace(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
