package gemini

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
)

// Compile-time interface assertions.
var (
	_ agent.Watcher               = (*Agent)(nil)
	_ agent.PathFilteredWatcher   = (*Agent)(nil)
	_ agent.ProjectPathDiscoverer = (*Agent)(nil)
	_ agent.SessionResolver       = (*Agent)(nil)
)

// Agent implements the agent.Watcher and agent.SessionResolver interfaces
// for Gemini CLI.
type Agent struct {
	agent.Base
	tmpDir                 string // full path to ~/.gemini/tmp/
	cachedSessionFiles     []string
	cachedSessionFilesTime time.Time
}

// New creates a Gemini CLI agent that monitors ~/.gemini/tmp.
func New(database *sql.DB) (*Agent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return NewForHome(database, home), nil
}

// NewForHome creates a Gemini CLI agent for the provided home directory.
func NewForHome(database *sql.DB, home string) *Agent {
	return &Agent{
		Base:   agent.NewBase(database, home, "gemini"),
		tmpDir: filepath.Join(home, ".gemini", "tmp"),
	}
}

// NewForHomeImportAll creates a Gemini CLI agent for an extra mounted home.
// These homes may be on network filesystems (SMB, NFS) where os.Stat metadata
// can be stale, so stat-based polling optimizations are disabled.
func NewForHomeImportAll(database *sql.DB, home string) *Agent {
	a := NewForHome(database, home)
	a.SkipStatOptimization = true
	return a
}

// newAgent is an internal constructor for testing with custom paths.
func newAgent(database *sql.DB, tmpDir, home string) *Agent {
	return &Agent{
		Base:   agent.NewBase(database, home, "gemini"),
		tmpDir: tmpDir,
	}
}
