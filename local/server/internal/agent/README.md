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
   - `agent.AppendDiffDBMessagesWithOptions(messages, opts)` when an agent needs multi-diff extraction or dedupe behavior
   - `agent.TitleFromPrompt(text)` for title generation
   - `agent.FirstNonEmpty(...)` for model resolution
   - `a.BackfillGitIDs(ctx)` / `a.BackfillLabels(ctx)` in your `Run()` loop
   - Follow message classification conventions above (`prompt/question/answer/log`)

6. Register the agent in `cli/run.go`.

7. If the agent doesn't support an optional interface (e.g. `ProjectPathDiscoverer`), simply don't implement it — the registry checks via type assertion.
