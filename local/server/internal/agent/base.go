package agent

import (
	"context"
	"database/sql"
	"log"
	"path/filepath"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// Base provides shared fields and methods that all agent implementations embed.
type Base struct {
	DB        *sql.DB
	Home      string
	Interval  time.Duration
	agentName string
}

// NewBase creates a Base with sensible defaults.
func NewBase(database *sql.DB, home, name string) Base {
	return Base{
		DB:        database,
		Home:      home,
		Interval:  2 * time.Second,
		agentName: name,
	}
}

// Name returns the agent name (implements Agent interface).
func (b *Base) Name() string { return b.agentName }

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

// CleanupEmptyConversations deletes conversations that have no messages and no
// ratings (started_at still 0 from EnsureConversation).
func (b *Base) CleanupEmptyConversations(ctx context.Context) {
	n, err := db.DeleteEmptyConversations(ctx, b.DB)
	if err != nil {
		log.Printf("%s watcher: cleanup empty conversations: %v", b.agentName, err)
		return
	}
	if n > 0 {
		log.Printf("%s watcher: cleaned up %d empty conversations", b.agentName, n)
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
