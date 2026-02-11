# zrate – rate coding agent developer workflows

## Architecture

- `web/frontend`: web frontend to view and manage ratings (in sveltekit)
- `web/server`: server to receive rating from plugins and full API (in golang)
- `plugins/*`: plugins for coding agents, like claude code and codex, for the user to rate their current conversation
