#!/bin/sh
# SPEC 014 e2e: fake ZaloCRM-style connector + POS-style connector (clean-room).
# No live Zalo, no AGPL binaries.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN="$ROOT/bin/goso-gateway"
TOKEN="${GOSO_E2E_TOKEN:-e2e-test-token}"
HOST="127.0.0.1"

if [ ! -x "$BIN" ]; then
  echo "==> e2e-connector: building gateway"
  go build -o "$BIN" ./gateway/cmd/goso-gateway
fi

if command -v python3 >/dev/null 2>&1; then
  PY=python3
elif command -v python >/dev/null 2>&1; then
  PY=python
else
  echo "ERROR: python3 is required" >&2
  exit 1
fi

json_get() {
  "$PY" -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

json_path() {
  "$PY" -c 'import json,sys
d=json.load(sys.stdin)
for p in sys.argv[1].split("."):
  if p.isdigit():
    d=d[int(p)]
  else:
    d=d[p]
print(d if not isinstance(d, (dict,list)) else json.dumps(d))' "$1"
}

CRM_LOG="$(mktemp "${TMPDIR:-/tmp}/goso-crm.XXXXXX")"
POS_LOG="$(mktemp "${TMPDIR:-/tmp}/goso-pos.XXXXXX")"
GW_LOG="$(mktemp "${TMPDIR:-/tmp}/goso-gw.XXXXXX")"
CRM_PID=""
POS_PID=""
GW_PID=""

cleanup() {
  for p in "$GW_PID" "$CRM_PID" "$POS_PID"; do
    if [ -n "$p" ]; then
      kill "$p" 2>/dev/null || true
      wait "$p" 2>/dev/null || true
    fi
  done
  rm -f "$CRM_LOG" "$POS_LOG" "$GW_LOG"
}
trap cleanup EXIT INT TERM

start_fake() {
  # $1 = kind (crm|pos)  $2 = log file
  "$PY" - "$1" <<'PY'
import json, sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

kind = sys.argv[1]
if kind == "crm":
    MANIFEST = {
        "schema_version": "1.0",
        "name": "zalocrm",
        "tools": [
            {"name": "contact_search", "description": "Search contacts", "requires_approval": False,
             "input_schema": {"type": "object", "properties": {"query": {"type": "string"}}, "required": ["query"]}},
            {"name": "message_send", "description": "Send a message", "requires_approval": True,
             "input_schema": {"type": "object", "properties": {"contact_id": {"type": "string"}, "text": {"type": "string"}},
                              "required": ["contact_id", "text"]}},
        ],
    }
else:
    MANIFEST = {
        "schema_version": "1.0",
        "name": "pos",
        "tools": [
            {"name": "order_lookup", "description": "Look up order", "requires_approval": False,
             "input_schema": {"type": "object", "properties": {"order_id": {"type": "string"}}, "required": ["order_id"]}},
            {"name": "price_change", "description": "Change price", "requires_approval": True,
             "input_schema": {"type": "object", "properties": {"sku": {"type": "string"}, "price": {"type": "number"}},
                              "required": ["sku", "price"]}},
        ],
    }

class H(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        pass
    def _json(self, code, obj):
        b = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)
    def do_GET(self):
        if self.path in ("/healthz", "/readyz"):
            self._json(200, {"ok": True})
            return
        if self.path == "/manifest":
            self._json(200, MANIFEST)
            return
        self._json(404, {"error": "not found"})
    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(n) if n else b"{}"
        try:
            args = json.loads(raw.decode() or "{}")
        except Exception:
            args = {}
        if self.path == "/tools/contact_search":
            q = args.get("query", "")
            self._json(200, {"contacts": [{"id": "c1", "name": q or "A"}]})
            return
        if self.path == "/tools/order_lookup":
            self._json(200, {"order_id": args.get("order_id", ""), "total": 42})
            return
        if self.path in ("/tools/message_send", "/tools/price_change"):
            self._json(500, {"error": "mutations must not be invoked without approval"})
            return
        self._json(404, {"error": "not found"})

httpd = ThreadingHTTPServer(("127.0.0.1", 0), H)
host, port = httpd.server_address
print(f"{host}:{port}", flush=True)
httpd.serve_forever()
PY
}

echo "==> e2e-connector: starting fake CRM + POS"
start_fake crm >"$CRM_LOG" 2>&1 &
CRM_PID=$!
start_fake pos >"$POS_LOG" 2>&1 &
POS_PID=$!

wait_addr() {
  log="$1"
  pid="$2"
  i=0
  while [ "$i" -lt 50 ]; do
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "ERROR: process $pid died" >&2
      cat "$log" >&2
      exit 1
    fi
    if grep -qE '127\.0\.0\.1:[0-9]+' "$log" 2>/dev/null; then
      sed -n 's/.*\(127\.0\.0\.1:[0-9][0-9]*\).*/\1/p' "$log" | head -n 1
      return 0
    fi
    i=$((i + 1))
    sleep 0.1
  done
  echo "ERROR: no listen address in $log" >&2
  cat "$log" >&2
  exit 1
}

CRM_ADDR="$(wait_addr "$CRM_LOG" "$CRM_PID")"
POS_ADDR="$(wait_addr "$POS_LOG" "$POS_PID")"
echo "==> fake CRM at http://$CRM_ADDR"
echo "==> fake POS at http://$POS_ADDR"

echo "==> e2e-connector: starting gateway"
GOSO_ADMIN_TOKEN="$TOKEN" \
GOSO_RATE_LIMIT=0 \
GOSO_DB_PATH= \
GOSO_ENV=test \
GOSO_PORT=0 \
  "$BIN" gateway --host "$HOST" --port 0 >"$GW_LOG" 2>&1 &
GW_PID=$!

i=0
ADDR=""
while [ "$i" -lt 60 ]; do
  if ! kill -0 "$GW_PID" 2>/dev/null; then
    echo "ERROR: gateway exited" >&2
    cat "$GW_LOG" >&2
    exit 1
  fi
  if grep -q "listening on " "$GW_LOG" 2>/dev/null; then
    ADDR="$(sed -n 's/.*listening on \([^ ]*\).*/\1/p' "$GW_LOG" | head -n 1)"
    break
  fi
  i=$((i + 1))
  sleep 0.1
done
if [ -z "$ADDR" ]; then
  echo "ERROR: gateway did not print listen address" >&2
  cat "$GW_LOG" >&2
  exit 1
fi
BASE="http://$ADDR"
AUTH="Authorization: Bearer $TOKEN"
echo "==> gateway at $BASE"

fail() {
  echo "FAIL: $1" >&2
  echo "--- gateway log ---" >&2
  cat "$GW_LOG" >&2
  exit 1
}

hz="$(curl -sf -H "$AUTH" "$BASE/healthz" || true)"
echo "$hz" | grep -q '"ok":true' || fail "healthz: $hz"

# existing routes
curl -sf -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"agent_key":"e2e-agent","display_name":"E2E"}' \
  "$BASE/api/agents" > /tmp/goso-e2e-agent.json || fail "create agent"
AGENT_ID="$(json_get id < /tmp/goso-e2e-agent.json)"

curl -sf -H "$AUTH" -H "Content-Type: application/json" \
  -d "{\"name\":\"zalocrm\",\"transport\":\"http\",\"endpoint\":\"http://$CRM_ADDR\",\"enabled\":true}" \
  "$BASE/api/connectors" > /tmp/goso-e2e-crm.json || fail "create zalocrm connector"

curl -sf -H "$AUTH" -H "Content-Type: application/json" \
  -d "{\"name\":\"pos\",\"transport\":\"http\",\"endpoint\":\"http://$POS_ADDR\",\"enabled\":true}" \
  "$BASE/api/connectors" > /tmp/goso-e2e-pos.json || fail "create pos connector"

curl -sf -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"connector":"zalocrm"}' \
  "$BASE/api/agents/$AGENT_ID/connectors" >/dev/null || fail "link zalocrm"

curl -sf -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"connector":"pos"}' \
  "$BASE/api/agents/$AGENT_ID/connectors" >/dev/null || fail "link pos"

clist="$(curl -sf -H "$AUTH" "$BASE/api/connectors")"
echo "$clist" | grep -q zalocrm || fail "list missing zalocrm"
echo "$clist" | grep -q pos || fail "list missing pos"

curl -sf -H "$AUTH" -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"$AGENT_ID\",\"label\":\"e2e\"}" \
  "$BASE/api/sessions" > /tmp/goso-e2e-sess.json || fail "create session"
SID="$(json_get id < /tmp/goso-e2e-sess.json)"

echo "==> chat: tìm khách A"
curl -sf -H "$AUTH" -H "Content-Type: application/json" \
  -d "{\"session_id\":\"$SID\",\"message\":\"tìm khách A\"}" \
  "$BASE/api/chat" > /tmp/goso-e2e-chat.json || fail "chat"
CHAT="$(cat /tmp/goso-e2e-chat.json)"
echo "$CHAT" | grep -q contact_search || echo "$CHAT" | grep -q '"trace"' || fail "chat missing tool trace: $CHAT"
echo "$CHAT" | grep -qi pending_approval && fail "search should not require approval: $CHAT"

echo "==> invoke sensitive tool → pending_approval"
curl -sf -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"connector":"zalocrm","tool":"message_send","arguments":{"contact_id":"c1","text":"hello"}}' \
  "$BASE/api/tools/invoke" > /tmp/goso-e2e-inv.json || fail "invoke message_send"
INV="$(cat /tmp/goso-e2e-inv.json)"
echo "$INV" | grep -q pending_approval || fail "expected pending_approval: $INV"
APPR="$(json_path result.approval_id < /tmp/goso-e2e-inv.json)"
[ -n "$APPR" ] || fail "missing approval_id"

curl -sf -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"decision":"reject"}' \
  "$BASE/api/approvals/$APPR/decision" > /tmp/goso-e2e-dec.json || fail "decision"
echo "$(cat /tmp/goso-e2e-dec.json)" | grep -q rejected || fail "expected rejected"

curl -sf -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"connector":"pos","tool":"order_lookup","arguments":{"order_id":"9"}}' \
  "$BASE/api/tools/invoke" > /tmp/goso-e2e-posinv.json || fail "pos invoke"
echo "$(cat /tmp/goso-e2e-posinv.json)" | grep -q '"status":"ok"' || fail "pos invoke not ok"

ev="$(curl -sf -H "$AUTH" "$BASE/api/events?limit=20")"
echo "$ev" | grep -q human_feedback || fail "events missing human_feedback"
echo "$ev" | grep -qiE 'token|password|secret|bearer ' && fail "events leaked credentials"

echo "e2e-connector: OK"
