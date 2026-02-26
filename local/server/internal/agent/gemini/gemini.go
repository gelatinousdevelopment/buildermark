package gemini

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"
)

// Agent implements the agent.Watcher and agent.SessionResolver interfaces
// for Gemini CLI.
type Agent struct {
	db       *sql.DB
	tmpDir   string // full path to ~/.gemini/tmp/
	home     string
	interval time.Duration
}

// New creates a Gemini CLI agent that monitors ~/.gemini/tmp.
func New(database *sql.DB) (*Agent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &Agent{
		db:       database,
		tmpDir:   filepath.Join(home, ".gemini", "tmp"),
		home:     home,
		interval: 2 * time.Second,
	}, nil
}

// newAgent is an internal constructor for testing with custom paths.
func newAgent(database *sql.DB, tmpDir, home string) *Agent {
	return &Agent{
		db:       database,
		tmpDir:   tmpDir,
		home:     home,
		interval: 2 * time.Second,
	}
}

// Name returns "gemini".
func (a *Agent) Name() string { return "gemini" }
