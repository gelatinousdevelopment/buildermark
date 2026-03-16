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
| `diff_messages.go` | `AppendDiffEntries`, `AppendDiffDBMessages`, `AppendDiffDBMessagesWithOptions`, `DiffAppendOptions` |
| `diff.go`          | `ExtractReliableDiff`, `ExtractReliableDiffFromJSON`, `ExtractReliableDiffsFromJSON`, `FormatDiffMessage`, etc. |
| `gitroot.go`       | `FindGitRoot`, `ListGitWorktrees`, `GitRootCache`           |

### Diff Derivation Notes

- `AppendDiffDBMessages(messages)` keeps legacy behavior: single high-confidence diff per source message (content first, then JSON fallback).
- `AppendDiffDBMessagesWithOptions(messages, opts)` enables importer-specific behavior:
  - `UseAllJSONDiffs=true` emits all reliable diffs found in JSON payloads (deduped per message).
  - `Deduplicate=true` suppresses repeated synthetic diffs with identical `(conversation_id, role, diff)` keys.
- For best path matching, importers should enrich raw event JSON with context such as `cwd` before appending diff messages.

## Message Classification Conventions

Watchers now classify imported messages with `messages.message_type`:

| `message_type` | Meaning |
|----------------|---------|
| `prompt`       | Real user prompt text (not slash commands like `/clear` or rating commands like `$bb`) |
| `question`     | Model-originated structured question shown to the user for input |
| `answer`       | User response payload for a structured question |
| `diff`         | Synthetic diff-only message derived from a message or import payload |
| `log`          | Everything else (events, metadata, tool logs, summaries, etc.) |

Conventions to follow when implementing/updating an agent importer:

1. Set `db.Message.MessageType` on insert whenever possible.
2. Keep `role` semantic (`agent` for model/question, `user` for prompt/answer).
3. Prefer structured extraction over heuristic text parsing for question/answer flows.
4. Format structured question/answer content as markdown suitable for UI cards.
5. **Skip conversations with no user content.** Before calling `EnsureConversation`, verify that at least one message has `Role == "user"` or that ratings exist. Conversations with only agent/log messages appear empty in the UI and must not be imported.

Provider-specific structured question sources currently supported:

| Agent | Structured question source | Structured answer source |
|-------|-----------------------------|--------------------------|
| Claude | `AskUserQuestion` tool use blocks in conversation JSONL | `toolUseResult.questions` + `toolUseResult.answers` |
| Codex | `response_item` with `type=function_call` and `name=request_user_input` | matching `response_item` with `type=function_call_output` for the same `call_id` |

## Implementing a New Agent

### Step 1: Create the agent package

Create `agent/myagent/` with at least these files:

**`myagent.go`** — struct, constructors, compile-time assertions:

```go
package myagent

import (
    "database/sql"
    "github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
)

var (
    _ agent.Watcher               = (*Agent)(nil)
    _ agent.PathFilteredWatcher   = (*Agent)(nil)  // optional
    _ agent.ProjectPathDiscoverer = (*Agent)(nil)  // optional
    _ agent.SessionResolver       = (*Agent)(nil)  // optional
)

type Agent struct {
    agent.Base
    // agent-specific fields (paths to history files, caches, etc.)
}

func NewForHome(database *sql.DB, home string) *Agent {
    return &Agent{
        Base: agent.NewBase(database, home, "myagent"),
        // ...
    }
}
```

Provide an internal `newAgent()` constructor that accepts explicit paths so tests can use temp directories without touching the real filesystem.

**`watcher.go`** — `Run()`, `ScanSince()`, and optionally `ScanPathsSince()` / `DiscoverProjectPathsSince()`:

The `Run()` method follows this pattern:

```go
func (a *Agent) Run(ctx context.Context) {
    // 1. Determine startup scan window
    scanWindow := agent.DefaultScanWindow
    if latestMs, err := db.LatestWatcherScanTimestamp(ctx, a.DB, a.Name()); err == nil {
        scanWindow = agent.StartupScanWindow(latestMs)
    }

    // 2. Initial scan
    trackedFilter := agent.TrackedProjectFilter(ctx, a.DB, nil)
    a.scanSince(ctx, time.Now().Add(-scanWindow), trackedFilter)
    _ = db.UpsertWatcherScanState(ctx, a.DB, db.WatcherScanState{
        Agent: a.Name(), SourceKind: "scan_marker", SourceKey: "startup",
    })
    a.BackfillGitIDs(ctx)
    a.BackfillLabels(ctx)

    // 3. Poll loop
    ticker := time.NewTicker(a.Interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            trackedFilter = agent.TrackedProjectFilter(ctx, a.DB, nil)
            count := a.poll(ctx, trackedFilter)
            a.BackfillGitIDsThrottled(ctx)
            var newInterval time.Duration
            if count > 0 {
                newInterval = a.MarkActive()
            } else {
                newInterval = a.MarkIdle()
            }
            a.RecordPoll()
            ticker.Reset(newInterval)
        }
    }
}
```

When processing each conversation/session:
- Resolve the project path, then call `agent.FindGitRoot()` to normalize to the repo root.
- Call `db.EnsureProject()`, then verify the conversation has at least one user message before calling `db.EnsureConversation()`.
- Set a title via `db.UpdateConversationTitle()` using `agent.TitleFromPrompt()`.
- Append diff messages with `agent.AppendDiffDBMessages()` or `agent.AppendDiffDBMessagesWithOptions()`.
- Insert messages with `db.InsertMessages()`.

**Additional files** as needed for parsing/formatting (e.g. `format.go`, `session.go`).

### Step 2: Register in `cli/run.go`

1. Add the import:
   ```go
   "github.com/gelatinousdevelopment/buildermark/local/server/internal/agent/myagent"
   ```

2. Add to the registration block (alongside existing agents):
   ```go
   registry.Register(myagent.NewForHome(database, h))
   ```

3. Update the `newWatchers` slice calculation — the multiplier in `watchers[len(watchers)-len(added)*N:]` must equal the total number of registered agent types.

4. Add to `normalizeHomePath()` — if the agent has a config directory like `.myagent`, add it to the check so paths like `/home/user/.myagent` resolve to `/home/user`.

5. Add to `pluginBundleExists()` — add the plugin path to the `required` slice.

### Step 3: Handler integration

**`handler/local_settings.go`:**
- Add `"myagent"` to the `localAgentNames` slice.
- If the agent has a config directory (e.g. `.myagent`), add it to `normalizeHomeEntries()`.
- If the agent stores conversations in a predictable filesystem path (like JSONL files), add it to `collectConversationSearchPaths()`.

**`handler/plugins.go`:**
- Add a plugin definition in `pluginDefinitions()` with source paths, install paths, and any template replacements.

### Step 4: Frontend changes

**`local/frontend/src/lib/agents.ts`:**
- Add the agent name to `KNOWN_AGENT_VALUES`.
- Add an entry to `KNOWN_AGENT_INFO` with resume configuration:
  ```typescript
  myagent: {
      supportsResumeFromBuildermark: true,  // or false
      resumeCommandTemplate: 'myagent -r {{sessionId}} {{resumePrompt}}',  // or null
      resumePrompt: '/rate-buildermark'  // or null
  }
  ```

### Step 5: Plugin files

Create `plugins/myagent/` with a rating skill/command and submission script. The structure varies by agent type — see existing plugins for examples. The submission script POSTs to `http://localhost:55022/api/ratings`.

### Step 6: Tests

Add `*_test.go` files in the agent package. Use the internal `newAgent()` constructor with temp directories. Test parsing, path filtering, scanning, and session resolution.

### Shared utilities reference

| Utility | Usage |
|---------|-------|
| `agent.TrackedProjectFilter(ctx, a.DB, nil)` | Filter to only tracked projects |
| `agent.NewPathFilter(paths)` | Filter to specific project paths |
| `agent.FindGitRoot(path)` | Resolve path to git repo root |
| `agent.AppendDiffDBMessages(messages)` | Add synthetic diff messages (single best diff per message) |
| `agent.AppendDiffDBMessagesWithOptions(messages, opts)` | Add diffs with multi-diff or dedupe behavior |
| `agent.TitleFromPrompt(text)` | Generate conversation title from first prompt |
| `agent.FirstNonEmpty(...)` | Select first non-empty string |
| `a.BackfillGitIDs(ctx)` | Resolve git root commits for projects |
| `a.BackfillGitIDsThrottled(ctx)` | Throttled version for poll loops |
| `a.BackfillLabels(ctx)` | Update project labels from repo directory names |
| `a.MarkActive()` / `a.MarkIdle()` | Manage adaptive poll interval |
| `a.RecordPoll()` | Record last poll time |

### Interface summary

Only implement the interfaces your agent supports — the registry checks via type assertion.

| Interface | Methods | When to implement |
|-----------|---------|-------------------|
| `Watcher` (required) | `Run(ctx)`, `ScanSince(ctx, since, progress)` | Always — this is the core import loop |
| `PathFilteredWatcher` | `ScanPathsSince(ctx, since, paths, progress)` | When the agent can efficiently scan specific project paths |
| `ProjectPathDiscoverer` | `DiscoverProjectPathsSince(ctx, since)` | When the agent can enumerate project paths without full import |
| `SessionResolver` | `ResolveSession(rating, note, fallbackID)` | When the agent supports rating the current session from a plugin |
