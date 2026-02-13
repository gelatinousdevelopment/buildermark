# Architecture

Possible folder structure:

- `/plugins`
  - `/claudecode`
  - `/codex`
  - `/gemini`
- `/server`
  - `/_shared`
    - `/db`
    - `/handler`
  - `/local`:
    - `/internal/handler`: endpoints for `/api/v1/local/*`
  - `/team`
    - `/internal/handler`: endpoints for `/api/v1/team/*`
  - `/cloud`
- `/frontend/src`
  - `/lib`
  - `/routes`
    - `/local`
    - `/team`
    - `/cloud`

## Extensions

- claude
- codex
- gemini
- opencode
- VS Code and Cursor

## Local (dev machine binary)

Local Server:

- go binary
- collects, displays, and forwards local user's data
- self-updating
- settings in web UI

Local Frontend:

- no user login
- localhost:7022 only

## Team (docker container)

Team Server:

- docker container
- receives data from local servers, displays team stats

Team Frontend:

- user accounts? or just one API key per user?

## Cloud

- team server + extra features
- email reports
