# Buildermark Local

## Linux CLI install

```bash
curl -fsSL https://github.com/buildermark/buildermark/releases/latest/download/buildermark-install.sh | bash
```

The installer places `buildermark` in `~/.local/bin` by default, then prints PATH help plus the next commands to run, including `buildermark service install`.

## `config.json`

Buildermark stores persistent settings in `~/.buildermark/config.json`.

### Schema

```json
{
  "updateMode": "check",
  "extraAgentHomes": ["/home/alice", "/home/bob/.codex"],
  "extraCORSOrigins": ["http://localhost:5173"]
}
```

### Fields

| Field | Type | Default | Description |
|------|------|---------|-------------|
| `updateMode` | string | `"check"` | Update behavior for CLI updates. Allowed values: `"auto"`, `"check"`, `"off"`. |
| `extraAgentHomes` | string[] | `[]` | Additional user home directories to watch for agent activity. |
| `extraCORSOrigins` | string[] | `[]` | Additional origins allowed to make cross-origin requests to the API (e.g. `"http://localhost:5173"` for a dev frontend). |

### `extraAgentHomes` behavior

- Paths are cleaned before use.
- If a path ends with `.claude`, `.codex`, or `.gemini`, Buildermark uses the parent directory as the home.
- Duplicate homes are ignored.

### How to set update mode

```bash
buildermark update mode auto
buildermark update mode check
buildermark update mode off
```
