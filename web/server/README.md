# zrate server

A Go HTTP server that stores conversation ratings in SQLite.

## Quick Start

```bash
cd web/server
go run ./cmd/zrate
```

Server starts on [http://localhost:7022](http://localhost:7022).

## Options

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `-addr` | — | `:7022` | Listen address |
| `-db` | `ZRATE_DB_PATH` | `../../.data/local.db` | SQLite database path |

## API

### `POST /api/v1/rating`

Create a rating.

```bash
curl -X POST http://localhost:7022/api/v1/rating \
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

### `GET /`

HTML dashboard showing recent ratings. Auto-refreshes every 30s.

## Build

```bash
go build -o zrate-server ./cmd/zrate
./zrate-server
```

## Test

```bash
go test ./...
```
