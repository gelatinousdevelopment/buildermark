# Buildermark

**How much of your code is written by agents?**

[Buildermark](https://buildermark.dev) matches your coding agent diffs with commits. It runs locally in the background archiving your agent conversations and serves a frontend on localhost. Your data never leaves your machine. No accounts, no cloud, no analytics.

- **Coding agent tracking** - Get per-commit percentages of code written by your agents.
- **Archive coding agent conversations** - Import conversations from Claude Code, Codex, Gemini, Cursor, Claude Code Cloud, and Codex Cloud. It can import your conversation history that's still on your machine. [Request more agents](https://github.com/gelatinousdevelopment/buildermark/issues)
- **Formatting-agnostic diff matching** - Buildermark matches agent output to your commits even when formatting differs or code is moved/copied. It analyzes the content of conversations without relying on hooks for each agent. 
- **Rate conversations** - Rate conversations manually or have the agent rate itself with the `/rate-buildermark` skill.
- **Native notifications** - See agent attribution for each commit in your system notification center.

### Online Demo

[Browse all 364 agent conversations that wrote 94% of Buildermark's code](https://demo.buildermark.dev/projects/u020uhEFtuWwPei6z6nbN)

[![Buildermark project view](https://buildermark.dev/images/screenshot-project-transparent.avif)](https://demo.buildermark.dev/projects/u020uhEFtuWwPei6z6nbN)

### Install

Download from [buildermark.dev](https://buildermark.dev/download) or [GitHub Releases](https://gelatinousdevelopment/buildermark/releases).

- macOS 15 (Sequoia) or later
- Windows 10 or later
- Linux CLI

### How it works

1. Imports conversation history from your coding agents.
2. Imports git commit history from your local repository.
3. Buildermark matches conversation diffs to commit diffs and calculates agent percentages.

A local app container manages a Go server on `localhost:55022`. Everything runs on your machine.

### Browser Extensions

Browser extensions let you view Buildermark data alongside your workflow.

- **Chrome** (and Chrome-based browsers: Edge, Brave, Helium)
- **Firefox**
- **Safari**

### Support

- GitHub Issues: <https://github.com/gelatinousdevelopment/buildermark/issues>
- GitHub Discussions: <https://github.com/gelatinousdevelopment/buildermark/discussions>
- Email: support@buildermark.dev
- Security: security@buildermark.dev

### Future Work

- Add support for more agents
- More charts and advanced insights
- Skill for an agent to search conversation history in the sqlite database
- Team Server (coming soon, with a revenue model to sustain this project)

## Documentation

### Database

The local sqlite database is stored in `~/.buildermark/local.db`.

### Configuration

Buildermark stores persistent settings in `~/.buildermark/config.json`.

Schema:

```json
{
  "updateMode": "check",
  "extraAgentHomes": ["/home/alice", "/volumes/debianvm/home/user"],
  "extraCORSOrigins": ["http://localhost:5173"]
}
```

| Field | Type | Default | Description |
|------|------|---------|-------------|
| `extraAgentHomes` | string[] | `[]` | Additional user home directories to watch for agent activity. |
| `extraCORSOrigins` | string[] | `[]` | Additional origins allowed to make cross-origin requests to the API (e.g. `"http://localhost:5173"` for a dev frontend). |
| `updateMode` | string | `"check"` | Linux CLI only. Update behavior for updates. Allowed values: `"auto"`, `"check"`, `"off"`. |

## License

MIT
