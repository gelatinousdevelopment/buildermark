package agent

import (
	"context"
	"time"
)

// DefaultScanWindow is how far back the initial scan looks (1 week).
const DefaultScanWindow = 7 * 24 * time.Hour

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
