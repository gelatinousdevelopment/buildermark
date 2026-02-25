package gemini

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/web/server/internal/db"
)

func (a *Agent) resolveProjectPath(conv *geminiConversation) string {
	if conv == nil {
		return ""
	}

	if direct := inferProjectPath(conv); direct != "" {
		return direct
	}

	hash := strings.TrimSpace(conv.ProjectHash)
	if hash == "" {
		return ""
	}

	if path := a.resolveHashFromKnownProjects(hash); path != "" {
		return path
	}
	if path := resolveHashFromCWD(hash); path != "" {
		return path
	}

	return ""
}

func (a *Agent) resolveHashFromKnownProjects(hash string) string {
	ctx := context.Background()
	seen := map[string]struct{}{}

	for _, ignored := range []bool{false, true} {
		projects, err := db.ListProjects(ctx, a.db, ignored)
		if err != nil {
			continue
		}
		for _, p := range projects {
			for _, path := range projectPathsForHashLookup(p) {
				if _, ok := seen[path]; ok {
					continue
				}
				seen[path] = struct{}{}
				if hashProjectPath(path) == hash {
					return path
				}
			}
		}
	}
	return ""
}

func projectPathsForHashLookup(p db.Project) []string {
	paths := []string{strings.TrimSpace(p.Path)}
	for _, oldPath := range strings.Split(p.OldPaths, "\n") {
		paths = append(paths, strings.TrimSpace(oldPath))
	}

	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" || !filepath.IsAbs(path) {
			continue
		}
		out = append(out, path)
	}
	return out
}

func resolveHashFromCWD(hash string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	p := filepath.Clean(cwd)
	for {
		if hashProjectPath(p) == hash {
			return p
		}
		next := filepath.Dir(p)
		if next == p {
			break
		}
		p = next
	}
	return ""
}
