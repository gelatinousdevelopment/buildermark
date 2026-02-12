package codex

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"
)

// Agent implements the agent.Watcher and agent.SessionResolver interfaces
// for Codex CLI.
type Agent struct {
	db          *sql.DB
	sessionsDir string // full path to ~/.codex/sessions/
	home        string
	interval    time.Duration
}

// New creates a Codex CLI agent that monitors ~/.codex/sessions/.
func New(database *sql.DB) (*Agent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &Agent{
		db:          database,
		sessionsDir: filepath.Join(home, ".codex", "sessions"),
		home:        home,
		interval:    2 * time.Second,
	}, nil
}

// newAgent is an internal constructor for testing with custom paths.
func newAgent(database *sql.DB, sessionsDir, home string) *Agent {
	return &Agent{
		db:          database,
		sessionsDir: sessionsDir,
		home:        home,
		interval:    2 * time.Second,
	}
}

// Name returns "codex".
func (a *Agent) Name() string { return "codex" }
