#!/bin/sh
# Optional live smoke against router9. Skip (exit 0) when the router is down.
# Does not bind or kill :8082 :8091 :3000 :18080 :18088.
# No product secrets in this file. Tokens come from the environment only.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BASE_URL="${GOSO_ROUTER9_BASE_URL:-}"
if [ -z "$BASE_URL" ]; then
  echo "e2e-router9: skip (GOSO_ROUTER9_BASE_URL unset)"
  exit 0
fi

base="${BASE_URL%/}"
case "$base" in
  */v1) MODELS_URL="$base/models" ;;
  *) MODELS_URL="$base/v1/models" ;;
esac

MODELS_CODE="$(curl -sS -o /dev/null -w "%{http_code}" --max-time 5 "$MODELS_URL" 2>/dev/null || true)"
if [ "$MODELS_CODE" != "200" ]; then
  echo "e2e-router9: skip (router /v1/models HTTP ${MODELS_CODE:-fail} at $MODELS_URL)"
  exit 0
fi

echo "==> e2e-router9: /v1/models 200 at $MODELS_URL"

BIN="$ROOT/bin/goso-gateway"
TOKEN="${GOSO_ADMIN_TOKEN:-${GOSO_E2E_TOKEN:-e2e-test-token}}"
HOST="127.0.0.1"
MODEL="${GOSO_ROUTER9_MODEL:-ocg/deepseek-v4-flash}"

if [ ! -x "$BIN" ]; then
  echo "==> e2e-router9: building gateway"
  go build -o "$BIN" ./gateway/cmd/goso-gateway
fi

if command -v python3 >/dev/null 2>&1; then
  PY=python3
elif command -v python >/dev/null 2>&1; then
  PY=python
else
  echo "ERROR: python3 is required to parse JSON" >&2
  exit 1
fi

json_get() {
  "$PY" -c 'import json,sys; print(json.load(sys.stdin).get(sys.argv[1],"") or "")' "$1"
}

LOG="$(mktemp "${TMPDIR:-/tmp}/goso-e2e-r9.XXXXXX")"
PID=""
cleanup() {
  if [ -n "$PID" ]; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -f "$LOG"
}
trap cleanup EXIT INT TERM

echo "==> e2e-router9: starting ephemeral gateway"
GOSO_ADMIN_TOKEN="$TOKEN" \
GOSO_RATE_LIMIT=0 \
GOSO_DB_PATH= \
GOSO_ENV=test \
GOSO_LLM_PROVIDER="${GOSO_LLM_PROVIDER:-router9}" \
GOSO_ROUTER9_BASE_URL="$BASE_URL" \
GOSO_ROUTER9_MODEL="$MODEL" \
GOSO_ROUTER9_API_KEY="${GOSO_ROUTER9_API_KEY:-}" \
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

GW="http://$ADDR"
echo "==> e2e-router9: gateway at $GW (ephemeral; not a demo port)"

fail() {
  echo "FAIL: $1" >&2
  echo "--- gateway log ---" >&2
  cat "$LOG" >&2
  exit 1
}

BODY=""
CODE=""
curl_json() {
  method="$1"
  path="$2"
  data="${3:-}"
  tmp="$(mktemp "${TMPDIR:-/tmp}/goso-e2e-r9-body.XXXXXX")"
  if [ -n "$data" ]; then
    CODE="$(curl -sS -o "$tmp" -w "%{http_code}" --max-time 120 \
      -X "$method" "$GW$path" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      --data "$data")" || fail "curl $method $path"
  else
    CODE="$(curl -sS -o "$tmp" -w "%{http_code}" --max-time 120 \
      -X "$method" "$GW$path" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json")" || fail "curl $method $path"
  fi
  BODY="$(cat "$tmp")"
  rm -f "$tmp"
}

curl_json POST /api/agents "{\"agent_key\":\"e2e-r9\",\"display_name\":\"E2E Router9\",\"model\":\"$MODEL\"}"
if [ "$CODE" != "201" ]; then
  fail "create agent: expected HTTP 201, got $CODE body=$BODY"
fi
AGENT_ID="$(printf '%s' "$BODY" | json_get id)"
[ -n "$AGENT_ID" ] || fail "create agent missing id"

curl_json POST /api/sessions "{\"agent_id\":\"$AGENT_ID\",\"label\":\"e2e-r9\"}"
if [ "$CODE" != "201" ]; then
  fail "create session: expected HTTP 201, got $CODE body=$BODY"
fi
SESS_ID="$(printf '%s' "$BODY" | json_get id)"
[ -n "$SESS_ID" ] || fail "create session missing id"

echo "==> e2e-router9: POST /api/chat model=$MODEL"
curl_json POST /api/chat "{\"session_id\":\"$SESS_ID\",\"message\":\"ping\"}"
if [ "$CODE" != "200" ]; then
  fail "chat: expected HTTP 200, got $CODE"
fi
REPLY="$(printf '%s' "$BODY" | json_get reply)"
if [ -z "$REPLY" ]; then
  fail "chat: empty reply body=$BODY"
fi
echo "    chat HTTP 200 non-empty reply OK"
echo "e2e-router9: OK"
