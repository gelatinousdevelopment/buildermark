package agent

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"
)

// DefaultScanWindow is how far back the initial scan looks (default 90 days).
// Override with BUILDERMARK_LOCAL_SCAN_WINDOW_HOURS environment variable.
var DefaultScanWindow = func() time.Duration {
	if v := os.Getenv("BUILDERMARK_LOCAL_SCAN_WINDOW_HOURS"); v != "" {
		if hours, err := strconv.ParseInt(v, 10, 64); err == nil && hours > 0 {
			log.Printf("using custom scan window: %d hours", hours)
			return time.Duration(hours) * time.Hour
		}
	}
	return 90 * 24 * time.Hour
}()

// StartupScanWindow computes a scan window based on how recently the server
// last ran. If latestMs > 0, the window is time.Since(latest) + 5 min buffer,
// capped at DefaultScanWindow and floored at 1 min. If latestMs == 0 (first
// run), it returns DefaultScanWindow.
func StartupScanWindow(latestMs int64) time.Duration {
	if latestMs <= 0 {
		return DefaultScanWindow
	}
	elapsed := time.Since(time.UnixMilli(latestMs))
	window := elapsed + 5*time.Minute
	if window < time.Minute {
		window = time.Minute
	}
	if window > DefaultScanWindow {
		window = DefaultScanWindow
	}
	return window
}

// Agent is the base interface every coding agent must implement.
type Agent interface {
	Name() string // e.g. "claude", "codex"
}

// ScanProgressFunc is called during scanning with the name of each file being
// processed. Implementations may call it frequently; callers should rate-limit
// if needed.
type ScanProgressFunc func(filename string)

// Watcher monitors an agent's log files and imports data into the database.
type Watcher interface {
	Agent
	Run(ctx context.Context)
	ScanSince(ctx context.Context, since time.Time, progress ScanProgressFunc) int
	LastPollTime() time.Time
}

// PathFilteredWatcher can scan only entries/files that belong to specific project paths.
type PathFilteredWatcher interface {
	Watcher
	ScanPathsSince(ctx context.Context, since time.Time, paths []string, progress ScanProgressFunc) int
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
