#!/bin/sh
# GOSO E2E: healthz → agents → sessions → chat → webhook
# Starts a throwaway gateway on a random port and tears it down on exit.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN="$ROOT/bin/goso-gateway"
TOKEN="${GOSO_E2E_TOKEN:-e2e-test-token}"
HOST="127.0.0.1"

if [ ! -x "$BIN" ]; then
  echo "==> e2e: building gateway"
  go build -o "$BIN" ./gateway/cmd/goso-gateway
fi

if command -v python3 >/dev/null 2>&1; then
  PY=python3
elif command -v python >/dev/null 2>&1; then
  PY=python
else
  echo "ERROR: python3 is required to parse JSON in e2e" >&2
  exit 1
fi

json_get() {
  # json_get FIELD  — read JSON object from stdin, print FIELD
  "$PY" -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

LOG="$(mktemp "${TMPDIR:-/tmp}/goso-e2e.XXXXXX")"
PID=""
cleanup() {
  if [ -n "$PID" ]; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -f "$LOG"
}
trap cleanup EXIT INT TERM

echo "==> e2e: starting gateway"
GOSO_ADMIN_TOKEN="$TOKEN" \
GOSO_RATE_LIMIT=0 \
GOSO_DB_PATH= \
GOSO_ENV=test \
  "$BIN" gateway --host "$HOST" --port 0 >"$LOG" 2>&1 &
PID=$!

i=0
ADDR=""
while [ "$i" -lt 60 ]; do
  if ! kill -0 "$PID" 2>/dev/null; then
    echo "ERROR: gateway exited before listen" >&2
    cat "$LOG" >&2
    exit 1
  fi
  if grep -q "listening on " "$LOG" 2>/dev/null; then
    ADDR="$(sed -n 's/.*listening on \([^ ]*\).*/\1/p' "$LOG" | head -n 1)"
    break
  fi
  i=$((i + 1))
  sleep 0.1
done

if [ -z "$ADDR" ]; then
  echo "ERROR: gateway did not print listen address" >&2
  cat "$LOG" >&2
  exit 1
fi

BASE="http://$ADDR"
echo "==> e2e: gateway at $BASE"

fail() {
  echo "FAIL: $1" >&2
  echo "--- gateway log ---" >&2
  cat "$LOG" >&2
  exit 1
}

expect_code() {
  got="$1"
  want="$2"
  step="$3"
  if [ "$got" != "$want" ]; then
    fail "$step: expected HTTP $want, got $got"
  fi
}

# curl_json METHOD PATH [DATA] → sets BODY and CODE
BODY=""
CODE=""
curl_json() {
  method="$1"
  path="$2"
  data="${3:-}"
  tmp="$(mktemp "${TMPDIR:-/tmp}/goso-e2e-body.XXXXXX")"
  if [ -n "$data" ]; then
    CODE="$(curl -sS -o "$tmp" -w "%{http_code}" \
      -X "$method" "$BASE$path" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      --data "$data")" || fail "curl $method $path"
  else
    CODE="$(curl -sS -o "$tmp" -w "%{http_code}" \
      -X "$method" "$BASE$path" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json")" || fail "curl $method $path"
  fi
  BODY="$(cat "$tmp")"
  rm -f "$tmp"
}

echo "==> e2e: GET /healthz (no auth)"
tmp="$(mktemp "${TMPDIR:-/tmp}/goso-e2e-body.XXXXXX")"
CODE="$(curl -sS -o "$tmp" -w "%{http_code}" "$BASE/healthz")" || fail "curl GET /healthz"
BODY="$(cat "$tmp")"
rm -f "$tmp"
expect_code "$CODE" "200" "healthz"
ok="$(printf '%s' "$BODY" | json_get ok)"
if [ "$ok" != "True" ] && [ "$ok" != "true" ]; then
  fail "healthz ok=$ok body=$BODY"
fi
echo "    healthz OK"

echo "==> e2e: GET /api/agents without token → 401"
tmp="$(mktemp "${TMPDIR:-/tmp}/goso-e2e-body.XXXXXX")"
CODE="$(curl -sS -o "$tmp" -w "%{http_code}" "$BASE/api/agents")" || fail "curl unauth"
rm -f "$tmp"
expect_code "$CODE" "401" "unauth agents"
echo "    unauth 401 OK"

echo "==> e2e: POST /api/agents"
curl_json POST /api/agents '{"agent_key":"e2e-agent","display_name":"E2E Agent","model":"echo"}'
expect_code "$CODE" "201" "create agent"
AGENT_ID="$(printf '%s' "$BODY" | json_get id)"
if [ -z "$AGENT_ID" ]; then
  fail "create agent missing id: $BODY"
fi
echo "    agent id=$AGENT_ID"

echo "==> e2e: GET /api/agents"
curl_json GET /api/agents
expect_code "$CODE" "200" "list agents"

echo "==> e2e: POST /api/sessions"
curl_json POST /api/sessions "{\"agent_id\":\"$AGENT_ID\",\"label\":\"e2e\"}"
expect_code "$CODE" "201" "create session"
SESS_ID="$(printf '%s' "$BODY" | json_get id)"
if [ -z "$SESS_ID" ]; then
  fail "create session missing id: $BODY"
fi
echo "    session id=$SESS_ID"

echo "==> e2e: GET /api/sessions"
curl_json GET /api/sessions
expect_code "$CODE" "200" "list sessions"

echo "==> e2e: POST /api/chat"
curl_json POST /api/chat "{\"session_id\":\"$SESS_ID\",\"message\":\"hello e2e\"}"
expect_code "$CODE" "200" "chat"
REPLY="$(printf '%s' "$BODY" | json_get reply)"
case "$REPLY" in
  echo:\ hello\ e2e) ;;
  *) fail "chat reply='$REPLY' body=$BODY" ;;
esac
echo "    chat reply OK"

echo "==> e2e: GET /api/sessions/{id}/messages"
curl_json GET "/api/sessions/$SESS_ID/messages"
expect_code "$CODE" "200" "list messages"

echo "==> e2e: POST /api/channels/telegram/webhook"
curl_json POST /api/channels/telegram/webhook \
  '{"update_id":1,"message":{"message_id":1,"chat":{"id":4242},"text":"e2e ping"}}'
expect_code "$CODE" "200" "telegram webhook"
ok="$(printf '%s' "$BODY" | json_get ok)"
if [ "$ok" != "True" ] && [ "$ok" != "true" ]; then
  fail "telegram webhook ok=$ok body=$BODY"
fi
echo "    telegram webhook OK"

echo "==> e2e: POST /api/channels/zalo-oa/webhook"
curl_json POST /api/channels/zalo-oa/webhook \
  '{"event_name":"user_send_text","sender":{"id":"u1"},"message":{"text":"e2e oa"}}'
expect_code "$CODE" "200" "zalo-oa webhook"
echo "    zalo-oa webhook OK"

echo "==> e2e: POST /api/channels/zalo-personal/webhook"
curl_json POST /api/channels/zalo-personal/webhook \
  '{"thread_id":"t-e2e","message":{"text":"e2e personal"}}'
expect_code "$CODE" "200" "zalo-personal webhook"
echo "    zalo-personal webhook OK"

echo "==> e2e: OK"
