package claude

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"
)

// Agent implements the agent.Watcher and agent.SessionResolver interfaces
// for Claude Code.
type Agent struct {
	db       *sql.DB
	path     string // full path to history.jsonl
	home     string // user home dir (for paste-cache resolution)
	offset   int64
	interval time.Duration
}

// New creates a Claude Code agent that monitors ~/.claude/history.jsonl.
func New(database *sql.DB) (*Agent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &Agent{
		db:       database,
		path:     filepath.Join(home, ".claude", "history.jsonl"),
		home:     home,
		interval: 2 * time.Second,
	}, nil
}

// newAgent is an internal constructor for testing with custom paths.
func newAgent(database *sql.DB, path, home string) *Agent {
	return &Agent{
		db:       database,
		path:     path,
		home:     home,
		interval: 2 * time.Second,
	}
}

// Name returns "claude".
func (a *Agent) Name() string { return "claude" }
