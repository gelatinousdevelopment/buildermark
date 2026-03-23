package cursor

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
)

// Compile-time interface assertions.
var (
	_ agent.Watcher               = (*Agent)(nil)
	_ agent.PathFilteredWatcher   = (*Agent)(nil)
	_ agent.ProjectPathDiscoverer = (*Agent)(nil)
)

// Agent implements the agent.Watcher and agent.ProjectPathDiscoverer interfaces
// for Cursor IDE.
type Agent struct {
	agent.Base
	globalDBPath string // path to global state.vscdb
	workspaceDir string // path to workspaceStorage/

	cachedWorkspaceMap     map[string]string // composerID -> projectPath
	cachedWorkspaceMapTime time.Time
}

// cursorUserDataDir returns the platform-specific Cursor user data directory.
func cursorUserDataDir(home string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "Cursor", "User")
	}
	return filepath.Join(home, ".config", "Cursor", "User")
}

// NewForHome creates a Cursor agent for the provided home directory.
func NewForHome(database *sql.DB, home string) *Agent {
	userDataDir := cursorUserDataDir(home)
	return newAgent(database, home, userDataDir)
}

// NewForHomeImportAll creates a Cursor agent for an extra mounted home.
// These homes may be on network filesystems (SMB, NFS) where os.Stat metadata
// can be stale, so stat-based polling optimizations are disabled.
func NewForHomeImportAll(database *sql.DB, home string) *Agent {
	a := NewForHome(database, home)
	a.SkipStatOptimization = true
	return a
}

// newAgent is an internal constructor for testing with custom paths.
func newAgent(database *sql.DB, home, userDataDir string) *Agent {
	return &Agent{
		Base:         agent.NewBase(database, home, "cursor"),
		globalDBPath: filepath.Join(userDataDir, "globalStorage", "state.vscdb"),
		workspaceDir: filepath.Join(userDataDir, "workspaceStorage"),
	}
}

// globalDBExists returns true if the global Cursor database file exists.
func (a *Agent) globalDBExists() bool {
	_, err := os.Stat(a.globalDBPath)
	return err == nil
}
