package handler

import (
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

const localUserFilterSentinel = "@me+agents"

type localUserAuthorFilter struct {
	emails   []string
	emailSet map[string]struct{}
}

func newLocalUserAuthorFilter(identity gitIdentity, extraEmails []string) localUserAuthorFilter {
	filter := localUserAuthorFilter{
		emails:   make([]string, 0, 1+len(extraEmails)),
		emailSet: make(map[string]struct{}, 1+len(extraEmails)),
	}

	appendEmail := func(email string) {
		email = strings.TrimSpace(email)
		if email == "" {
			return
		}
		lower := strings.ToLower(email)
		if _, ok := filter.emailSet[lower]; ok {
			return
		}
		filter.emailSet[lower] = struct{}{}
		filter.emails = append(filter.emails, email)
	}

	appendEmail(identity.Email)
	for _, email := range extraEmails {
		appendEmail(email)
	}

	return filter
}

func (f localUserAuthorFilter) Emails() []string {
	return append([]string(nil), f.emails...)
}

func (f localUserAuthorFilter) Matches(email string) bool {
	email = strings.TrimSpace(email)
	if email == "" {
		return false
	}
	_, ok := f.emailSet[strings.ToLower(email)]
	return ok
}

func (f localUserAuthorFilter) FilterCommits(commits []db.Commit) []db.Commit {
	filtered := make([]db.Commit, 0, len(commits))
	for _, commit := range commits {
		if f.Matches(commit.UserEmail) {
			filtered = append(filtered, commit)
		}
	}
	return filtered
}

func resolveCommitUserFilters(userEmails []string, identity gitIdentity, extraEmails []string) []string {
	filter := newLocalUserAuthorFilter(identity, extraEmails)
	resolved := make([]string, 0, len(userEmails)+len(filter.emails))
	seen := make(map[string]struct{}, len(userEmails)+len(filter.emails))

	appendEmail := func(email string) {
		email = strings.TrimSpace(email)
		if email == "" {
			return
		}
		lower := strings.ToLower(email)
		if _, ok := seen[lower]; ok {
			return
		}
		seen[lower] = struct{}{}
		resolved = append(resolved, email)
	}

	for _, email := range userEmails {
		email = strings.TrimSpace(email)
		if email == localUserFilterSentinel {
			for _, localEmail := range filter.Emails() {
				appendEmail(localEmail)
			}
			continue
		}
		appendEmail(email)
	}

	return resolved
}
