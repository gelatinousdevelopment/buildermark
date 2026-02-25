# Buildermark Local – rate coding agent developer workflows

## Architecture

- `web/frontend`: web frontend to view and manage ratings (in sveltekit)
- `web/server`: server to receive rating from plugins and full API (in golang)
- `plugins/*`: plugins for coding agents, like claude code and codex, for the user to rate their current conversation
- `.data/local.db`: sqlite database for the server

## Important Rules

- Never edit or even read `TODO.md`, `TODO-archive.md`, or `ARCHITECTURE.md`
- This is a greenfield project, so we do not need to maintain backwards compatibility for anything right now.
