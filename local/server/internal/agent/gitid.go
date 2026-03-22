package agent

import "github.com/gelatinousdevelopment/buildermark/local/server/internal/gitutil"

// ResolveGitID returns the root commit hash for the git repo at path,
// or "" if the path is not a git repository.
func ResolveGitID(path string) string {
	return gitutil.ResolveRootID(path)
}
