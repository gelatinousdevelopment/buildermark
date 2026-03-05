package codex

import (
	"database/sql"
	"os"
	"path/filepath"

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
// for Codex CLI.
type Agent struct {
	agent.Base
	sessionsDir string // full path to ~/.codex/sessions/
}

// New creates a Codex CLI agent that monitors ~/.codex/sessions/.
func New(database *sql.DB) (*Agent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return NewForHome(database, home), nil
}

// NewForHome creates a Codex CLI agent for the provided home directory.
func NewForHome(database *sql.DB, home string) *Agent {
	return &Agent{
		Base:        agent.NewBase(database, home, "codex"),
		sessionsDir: filepath.Join(home, ".codex", "sessions"),
	}
}

// newAgent is an internal constructor for testing with custom paths.
func newAgent(database *sql.DB, sessionsDir, home string) *Agent {
	return &Agent{
		Base:        agent.NewBase(database, home, "codex"),
		sessionsDir: sessionsDir,
	}
}
