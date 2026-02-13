# Server

## Instructions

- Add appropriate tests when you add or change something significant
- Use golang best practices

## Running

```bash
cd web/server

# kill existing process to free port
PORT=7022; kill -TERM $(lsof -nP -tiTCP:$PORT -sTCP:LISTEN) 2>/dev/null || true

go run ./cmd/zrate
```
