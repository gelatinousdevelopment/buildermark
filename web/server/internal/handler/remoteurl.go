package handler

import (
	"fmt"
	"net/url"
	"strings"
)

type parsedRemote struct {
	Domain string
	Owner  string
	Repo   string
}

// parseRemoteURL extracts domain, owner, and repo from a git remote URL.
// Supported formats:
//   - https://domain/owner/repo.git
//   - git@domain:owner/repo.git
//   - ssh://git@domain/owner/repo.git
func parseRemoteURL(raw string) (parsedRemote, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return parsedRemote{}, false
	}

	var domain, path string

	switch {
	case strings.HasPrefix(raw, "ssh://"):
		u, err := url.Parse(raw)
		if err != nil {
			return parsedRemote{}, false
		}
		domain = u.Hostname()
		path = strings.TrimPrefix(u.Path, "/")

	case strings.Contains(raw, "://"):
		u, err := url.Parse(raw)
		if err != nil {
			return parsedRemote{}, false
		}
		domain = u.Hostname()
		path = strings.TrimPrefix(u.Path, "/")

	default:
		// git@domain:owner/repo.git
		at := strings.Index(raw, "@")
		colon := strings.Index(raw, ":")
		if at < 0 || colon < 0 || colon <= at {
			return parsedRemote{}, false
		}
		domain = raw[at+1 : colon]
		path = raw[colon+1:]
	}

	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimSuffix(path, "/")

	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return parsedRemote{}, false
	}

	// For paths with more than 2 segments (e.g. gitlab subgroups), join them
	// as owner keeping the last segment as repo.
	owner := parts[0]
	repo := parts[1]
	if len(parts) == 3 && parts[2] != "" {
		owner = parts[0] + "/" + parts[1]
		repo = parts[2]
	}

	return parsedRemote{
		Domain: strings.ToLower(domain),
		Owner:  owner,
		Repo:   repo,
	}, true
}

// commitURL produces a web URL for viewing a commit on a known forge.
func commitURL(remoteRaw, hash string) string {
	parsed, ok := parseRemoteURL(remoteRaw)
	if !ok || hash == "" {
		return ""
	}

	switch parsed.Domain {
	case "github.com":
		return fmt.Sprintf("https://github.com/%s/%s/commit/%s", parsed.Owner, parsed.Repo, hash)
	case "gitlab.com":
		return fmt.Sprintf("https://gitlab.com/%s/%s/-/commit/%s", parsed.Owner, parsed.Repo, hash)
	case "codeberg.org":
		return fmt.Sprintf("https://codeberg.org/%s/%s/commit/%s", parsed.Owner, parsed.Repo, hash)
	case "bitbucket.org":
		return fmt.Sprintf("https://bitbucket.org/%s/%s/commits/%s", parsed.Owner, parsed.Repo, hash)
	default:
		// Covers GitHub Enterprise, Gitea, Forgejo, etc.
		return fmt.Sprintf("https://%s/%s/%s/commit/%s", parsed.Domain, parsed.Owner, parsed.Repo, hash)
	}
}
