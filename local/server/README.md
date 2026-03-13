# Buildermark Local Server

A Go HTTP server that stores conversation ratings in SQLite.

## Quick Start

```bash
cd web/server
go run ./cmd/buildermark
```

Server starts on [http://localhost:55022](http://localhost:55022).

## Options

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `-addr` | — | `:55022` | Listen address |
| `-db` | `BUILDERMARK_LOCAL_DB_PATH` | `../../.data/local.db` | SQLite database path |

## API

### `POST /api/v1/rating`

Create a rating.

```bash
curl -X POST http://localhost:55022/api/v1/rating \
  -H 'Content-Type: application/json' \
  -d '{"conversationId":"abc-123","rating":4,"note":"Helpful session"}'
```

Response (201):
```json
{"ok":true,"data":{"id":"uuid","conversationId":"abc-123","rating":4,"note":"Helpful session","createdAt":1735689600000}}
```

**Validation:**
- `conversationId` — required, non-empty string
- `rating` — required, integer 0–5
- `note` — optional string
- Body must be JSON, max 1MB

### `GET /api/v1/ratings?limit=50`

List recent ratings (newest first). `limit` defaults to 50, max 500.

### `POST /api/v1/history/scan`

Trigger conversation history re-import.

```bash
curl -X POST http://localhost:55022/api/v1/history/scan \
  -H 'Content-Type: application/json' \
  -d '{"agent":"codex","timeframe":"200000h","sync":true}'
```

Notes:
- `agent` is optional (`"codex"`, `"claude"`, `"gemini"`). Omit to scan all.
- `timeframe` uses Go duration format.
- `sync: true` blocks until scan completes and returns processed count.

### `GET /`

HTML dashboard showing recent ratings. Auto-refreshes every 30s.

## One-time Codex backfill helper

From repo root:

```bash
scripts/backfill-codex-diffs.sh <project_id> [branch] [timeframe]
```

This runs:
1. Synchronous Codex history scan.
2. Commit coverage recompute for the given project/branch.

## Build

```bash
go build -o buildermark-local-server ./cmd/buildermark
./buildermark-local-server
```

## Test

```bash
go test ./...
```
