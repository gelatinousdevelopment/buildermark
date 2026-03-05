package claude

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
// for Claude Code.
type Agent struct {
	agent.Base
	path   string // full path to history.jsonl
	offset int64
}

// New creates a Claude Code agent that monitors ~/.claude/history.jsonl.
func New(database *sql.DB) (*Agent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return NewForHome(database, home), nil
}

// NewForHome creates a Claude Code agent for the provided home directory.
func NewForHome(database *sql.DB, home string) *Agent {
	return &Agent{
		Base: agent.NewBase(database, home, "claude"),
		path: filepath.Join(home, ".claude", "history.jsonl"),
	}
}

// newAgent is an internal constructor for testing with custom paths.
func newAgent(database *sql.DB, path, home string) *Agent {
	return &Agent{
		Base: agent.NewBase(database, home, "claude"),
		path: path,
	}
}
