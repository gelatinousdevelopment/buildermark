package agent

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// FmtDuration formats a duration for log output. Sub-millisecond durations
// are displayed as "<1ms" for easier visual parsing. Seconds are shown as
// e.g. "1.5s" or "32s".
func FmtDuration(d time.Duration) string {
	if d < time.Millisecond {
		return "<1ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	secs := d.Seconds()
	if secs == float64(int(secs)) {
		return fmt.Sprintf("%ds", int(secs))
	}
	return fmt.Sprintf("%.1fs", secs)
}

const (
	MinPollInterval       = 15 * time.Second
	MaxPollInterval       = 60 * time.Second
	backfillGitIDInterval = 5 * time.Minute
)

// Base provides shared fields and methods that all agent implementations embed.
type Base struct {
	DB        *sql.DB
	Home      string
	Interval  time.Duration
	agentName string

	// SkipStatOptimization disables os.Stat-based short-circuits (file size
	// and mtime checks) used to skip unchanged files during polling. This
	// should be set to true for agents monitoring network-mounted homes
	// (SMB, NFS, etc.) where the OS may cache stale file metadata.
	SkipStatOptimization bool

	lastBackfillTime time.Time
	lastPollTime     time.Time
	idleTicks        int
}

// NewBase creates a Base with sensible defaults.
func NewBase(database *sql.DB, home, name string) Base {
	return Base{
		DB:        database,
		Home:      home,
		Interval:  MinPollInterval,
		agentName: name,
	}
}

// MarkIdle increments the idle counter and returns the new poll interval.
// Each idle tick adds 1/10th of the (max-min) range, reaching MaxPollInterval
// after 10 consecutive idle ticks.
func (b *Base) MarkIdle() time.Duration {
	b.idleTicks++
	step := (MaxPollInterval - MinPollInterval) / 10
	interval := MinPollInterval + step*time.Duration(b.idleTicks)
	if interval > MaxPollInterval {
		interval = MaxPollInterval
	}
	b.Interval = interval
	return interval
}

// MarkActive resets the idle counter and returns MinPollInterval.
func (b *Base) MarkActive() time.Duration {
	b.idleTicks = 0
	b.Interval = MinPollInterval
	return MinPollInterval
}

// Name returns the agent name (implements Agent interface).
func (b *Base) Name() string { return b.agentName }

// HomePath returns the agent home directory this instance is scoped to.
func (b *Base) HomePath() string { return b.Home }

// LastPollTime returns when this watcher last completed a poll cycle.
func (b *Base) LastPollTime() time.Time { return b.lastPollTime }

// RecordPoll records the current time as the last poll time.
func (b *Base) RecordPoll() { b.lastPollTime = time.Now() }

// BackfillGitIDs finds all projects without a git_id and attempts to
// resolve it from the git root commit.
func (b *Base) BackfillGitIDs(ctx context.Context) {
	projects, err := db.ListProjectsWithoutGitID(ctx, b.DB)
	if err != nil {
		log.Printf("%s watcher: list projects without git_id: %v", b.agentName, err)
		return
	}

	updated := 0
	for _, p := range projects {
		if _, err := os.Stat(p.Path); err != nil {
			continue
		}
		if gitID := ResolveGitID(p.Path); gitID != "" {
			if err := db.UpdateProjectGitID(ctx, b.DB, p.ID, gitID); err != nil {
				log.Printf("%s watcher: update git_id for %s: %v", b.agentName, p.ID, err)
				continue
			}
			updated++
		}
	}
	if updated > 0 {
		log.Printf("%s watcher: backfilled %d project git_ids", b.agentName, updated)
	}
}

// BackfillLabels updates project labels from the last path component to the
// git repository root directory name for projects whose label was auto-generated.
func (b *Base) BackfillLabels(ctx context.Context) {
	projects, err := db.ListAllProjects(ctx, b.DB)
	if err != nil {
		log.Printf("%s watcher: list projects for label backfill: %v", b.agentName, err)
		return
	}

	updated := 0
	for _, p := range projects {
		if _, err := os.Stat(p.Path); err != nil {
			continue
		}
		repoName := db.RepoLabel(p.Path)
		if repoName != p.Label && p.Label == filepath.Base(p.Path) {
			if err := db.SetProjectLabel(ctx, b.DB, p.ID, repoName); err != nil {
				log.Printf("%s watcher: update label for %s: %v", b.agentName, p.ID, err)
				continue
			}
			updated++
		}
	}
	if updated > 0 {
		log.Printf("%s watcher: backfilled %d project labels", b.agentName, updated)
	}
}

// BackfillGitIDsThrottled calls BackfillGitIDs at most once per backfillGitIDInterval.
func (b *Base) BackfillGitIDsThrottled(ctx context.Context) {
	if time.Since(b.lastBackfillTime) < backfillGitIDInterval {
		return
	}
	b.BackfillGitIDs(ctx)
	b.lastBackfillTime = time.Now()
}
