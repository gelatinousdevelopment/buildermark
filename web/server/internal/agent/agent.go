package agent

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"
)

// DefaultScanWindow is how far back the initial scan looks (default 90 days).
// Override with ZRATE_SCAN_WINDOW_HOURS environment variable.
var DefaultScanWindow = func() time.Duration {
	if v := os.Getenv("ZRATE_SCAN_WINDOW_HOURS"); v != "" {
		if hours, err := strconv.ParseInt(v, 10, 64); err == nil && hours > 0 {
			log.Printf("using custom scan window: %d hours", hours)
			return time.Duration(hours) * time.Hour
		}
	}
	return 90 * 24 * time.Hour
}()

// Agent is the base interface every coding agent must implement.
type Agent interface {
	Name() string // e.g. "claude", "codex"
}

// Watcher monitors an agent's log files and imports data into the database.
type Watcher interface {
	Agent
	Run(ctx context.Context)
	ScanSince(ctx context.Context, since time.Time) int
}

// PathFilteredWatcher can scan only entries/files that belong to specific project paths.
type PathFilteredWatcher interface {
	Watcher
	ScanPathsSince(ctx context.Context, since time.Time, paths []string) int
}

// ProjectPathDiscoverer returns likely project paths touched by an agent since
// the given cutoff without importing conversation data.
type ProjectPathDiscoverer interface {
	Agent
	DiscoverProjectPathsSince(ctx context.Context, since time.Time) []string
}

// SessionResolver resolves a rating to a real session with conversation entries.
type SessionResolver interface {
	Agent
	ResolveSession(rating int, note string, fallbackID string) *SessionResult
}

// Entry holds a single parsed history entry.
type Entry struct {
	Timestamp int64
	SessionID string
	Project   string
	Role      string // "user" or "agent"
	Model     string
	Display   string
	RawJSON   string
}

// SessionResult is returned by ResolveSession with the matched sessionId
// and all history entries belonging to that session.
type SessionResult struct {
	SessionID string
	Project   string
	Entries   []Entry
}
