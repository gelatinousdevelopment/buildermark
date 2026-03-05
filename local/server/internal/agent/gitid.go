package agent

import (
	"os/exec"
	"strings"
)

// ResolveGitID returns the root commit hash for the git repo at path,
// or "" if the path is not a git repository.
func ResolveGitID(path string) string {
	cmd := exec.Command("git", "-C", path, "rev-list", "--max-parents=0", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	// If there are multiple root commits, use the first one.
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return line
}
