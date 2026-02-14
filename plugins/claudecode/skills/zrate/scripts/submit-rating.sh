#!/usr/bin/env bash
set -euo pipefail

SERVER="${ZRATE_SERVER:-http://localhost:7022}"
DASHBOARD="${ZRATE_DASHBOARD:-http://localhost:5173}"

json_escape() {
  local s="${1:-}"
  s=${s//\\/\\\\}
  s=${s//\"/\\\"}
  s=${s//$'\n'/\\n}
  s=${s//$'\r'/\\r}
  s=${s//$'\t'/\\t}
  s=${s//$'\f'/\\f}
  s=${s//$'\b'/\\b}
  printf '%s' "$s"
}

# --- parse args ---
if [[ $# -lt 1 ]]; then
  echo "error: no rating provided"
  echo "usage: submit-rating.sh <0-5> [note...]"
  exit 1
fi

rating="$1"; shift
note="${*:-}"

# --- validate ---
if ! [[ "$rating" =~ ^[0-5]$ ]]; then
  echo "error: rating must be 0-5 (got '$rating')"
  exit 1
fi

# --- conversation id (throwaway; server resolves real sessionId from history) ---
cid=$(uuidgen | tr '[:upper:]' '[:lower:]')

# --- build JSON payload ---
analysis="${ANALYSIS:-}"
note_esc=$(json_escape "$note")
analysis_esc=$(json_escape "$analysis")
payload="{\"conversationId\":\"${cid}\",\"rating\":${rating},\"note\":\"${note_esc}\",\"analysis\":\"${analysis_esc}\"}"

# --- submit ---
response=$(curl -s -X POST "${SERVER}/api/v1/rating" \
  -H 'Content-Type: application/json' \
  -d "$payload" 2>/dev/null) || {
  echo "error: could not connect to zrate server at ${SERVER}"
  echo "hint: start the server with: cd web/server && go run ."
  exit 1
}

# --- check response ---
if printf '%s' "$response" | grep -q '"ok":true'; then
  conversation_url="${DASHBOARD%/}/local/conversations/${cid}"
  printf 'ok\n'
  printf 'rating: %s/5\n' "$rating"
  [[ -n "$note" ]] && printf 'note: %s\n' "$note"
  printf 'conversation: %s\n' "$cid"
  printf 'conversation_url: %s\n' "$conversation_url"
else
  echo "error: server rejected the rating"
  printf '%s\n' "$response"
  exit 1
fi
