# QA — SPEC 080 Cron heartbeat + sessions_* agent tools

Date: 2026-08-29. Clean-room Go/React. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not paste goclaw Go. Do not merge. Do not start another SPEC.

Closes matrix **K8**. Cron ticker already shipped in SPEC 054 — this spec does **not** rewrite cron.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Periodic jobs and scheduler lanes (cron already mapped in 054) | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/08-scheduling-cron.md` |
| Application-level heartbeat (not WebSocket ping/pong); ticker polls due work | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/22-heartbeat-system.md` |

goso mapping (self-written): optional in-process ticker `GOSO_HEARTBEAT=1` (default **off**) stamps Observer `last_heartbeat` RFC3339 UTC. Interval default **60s**, floor **30s** (`GOSO_HEARTBEAT_INTERVAL_SEC` below 30 clamps to 30). Admin `POST /api/system/heartbeat` does the same stamp (request body ignored). `GET /api/stats` includes `last_heartbeat` only after a POST or a tick; omitted when never fired. Agent tools `sessions_list` / `sessions_history` are advertised and dispatched like memory tools (LLM ToolCalls only, tenant jail, cap 50). Control-plane GatewayStatus shows the stamp from `GET /api/stats` when present.

Out of scope (do not implement): HEARTBEAT.md agent-loop delivery, channel delivery, `agent_heartbeats` / `heartbeat_run_logs` tables, rewriting the 054 cron ticker.

## What changed

- `observe.Observer`: `RecordHeartbeat` / `LastHeartbeat`; `Stats.LastHeartbeat` `omitempty`; `POST /api/system/heartbeat` and `/v1/system/heartbeat` (admin Bearer, same as other `/api/system/*`). View-token POST → 403.
- `gateway/internal/heartbeat`: `Enabled()`, `Interval()`, `Tick`, `Loop`. Started from `serve.Mux` only when `GOSO_HEARTBEAT=1` and not under `go test`. Cron loop unchanged.
- Pipeline tools: `sessions_list` (ListSessions jailed to calling agent tenant, cap 50, no message bodies) and `sessions_history` (`session_id`; GetSession tenant match; ListMessages last 50; fail-closed missing/other tenant). Advertised in `runtimeTools.List` next to memory tools; dispatched in `runtimeTools.Call`. No unauthenticated HTTP aliases. No chat-text keyword match.
- Control-plane `GatewayStatus` probes `GET /api/stats` alongside `/healthz` and appends last heartbeat when present. i18n vi+en.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
gofmt -l gateway desktop
go vet ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge.

## Proof

- Default `GET /api/stats` omits `last_heartbeat` (`TestHandleStatsAndMetrics`, `TestNewHealthzAndAgent`).
- `POST /api/system/heartbeat` (empty/ignored body) sets RFC3339 UTC `last_heartbeat` on the next `GET /api/stats`. `/v1` alias matches.
- `GOSO_HEARTBEAT` default off (`TestEnabled_DefaultOff`). Interval default 60s; `10` clamps to 30s (`TestInterval_Min30s`). `Tick` stamps; `Loop` stops on cancel. Mux does not start the ticker under `go test`.
- `sessions_list` / `sessions_history` advertised and dispatched via scripted provider (`TestChat_SessionsListHistoryTools`). Cap 50 (`TestDispatchSessionTool_ListJailsTenantAndCap`, `TestDispatchSessionTool_HistoryFailClosedAndCap`). Other tenant / missing session fail-closed. Chat text `sessions_list` does not dispatch (`TestChat_SessionsToolsNotKeywordMatched`).
- View-token POST `/api/system/heartbeat` → 403 (`TestRequireTokens_ViewPOSTDenyMatrix`).

## Non-goals

HEARTBEAT.md checklist / agent-loop delivery. Channel delivery (Telegram/Discord/Feishu). goclaw heartbeat tables. Rewriting cron (054). Binding/killing demo ports. Merge. Copying goclaw Go.
