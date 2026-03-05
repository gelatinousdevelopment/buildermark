# Agent Package

The `agent` package defines the interfaces for coding agent integrations and provides shared infrastructure that all agents embed.

## Interface Hierarchy

```
Agent (Name)
├── Watcher (Run, ScanSince)
│   └── PathFilteredWatcher (ScanPathsSince)
├── ProjectPathDiscoverer (DiscoverProjectPathsSince)
└── SessionResolver (ResolveSession)
```

## What `Base` Provides

Every agent embeds `agent.Base`, which provides:

| Field/Method       | Description                                      |
|--------------------|--------------------------------------------------|
| `DB`               | Database connection                              |
| `Home`             | User home directory                              |
| `Interval`         | Poll interval (default 2s)                       |
| `Name()`           | Returns the agent name (implements `Agent`)       |
| `BackfillGitIDs()` | Resolves git root commits for projects           |
| `BackfillLabels()` | Updates project labels from repo directory names |

## Shared Utilities

| File               | Exports                                                     |
|--------------------|-------------------------------------------------------------|
| `pathfilter.go`    | `PathFilter`, `NewPathFilter`, `TrackedProjectFilter`       |
| `gitid.go`         | `ResolveGitID`                                              |
| `util.go`          | `FirstNonEmpty`, `TitleFromPrompt`                          |
| `diff_messages.go` | `AppendDiffEntries`, `AppendDiffDBMessages`                 |
| `diff.go`          | `ExtractReliableDiff`, `FormatDiffMessage`, etc.            |
| `gitroot.go`       | `FindGitRoot`, `ListGitWorktrees`, `GitRootCache`           |

## Implementing a New Agent

1. Create a new package under `agent/` (e.g. `agent/myagent/`).

2. Define an `Agent` struct that embeds `agent.Base`:
   ```go
   type Agent struct {
       agent.Base
       // agent-specific fields
   }
   ```

3. Add constructors:
   ```go
   func New(db *sql.DB) (*Agent, error)
   func NewForHome(db *sql.DB, home string) *Agent
   ```

4. Implement the required interfaces. Add compile-time assertions:
   ```go
   var (
       _ agent.Watcher             = (*Agent)(nil)
       _ agent.SessionResolver     = (*Agent)(nil)
       // etc.
   )
   ```

5. Use shared utilities:
   - `agent.TrackedProjectFilter(ctx, a.DB, nil)` for path filtering
   - `agent.AppendDiffEntries(entries)` / `agent.AppendDiffDBMessages(messages)` for diff derivation
   - `agent.TitleFromPrompt(text)` for title generation
   - `agent.FirstNonEmpty(...)` for model resolution
   - `a.BackfillGitIDs(ctx)` / `a.BackfillLabels(ctx)` in your `Run()` loop

6. Register the agent in `cli/run.go`.

7. If the agent doesn't support an optional interface (e.g. `ProjectPathDiscoverer`), simply don't implement it — the registry checks via type assertion.
