# Buildermark Local

## `config.json`

Buildermark stores persistent settings in `~/.buildermark/config.json`.

### Schema

```json
{
  "updateMode": "check",
  "extraAgentHomes": ["/home/alice", "/home/bob/.codex"]
}
```

### Fields

| Field | Type | Default | Description |
|------|------|---------|-------------|
| `updateMode` | string | `"check"` | Update behavior for CLI updates. Allowed values: `"auto"`, `"check"`, `"off"`. |
| `extraAgentHomes` | string[] | `[]` | Additional user home directories to watch for agent activity. |

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
