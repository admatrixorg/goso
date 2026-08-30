# QA — SPEC 125 MONITORING operator UI/UX

Date: 2026-08-30. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. Worker does not merge and does not restart Vite `:3000`. Demos `:8082` `:8091` `:18791` were not bound or killed. No Grafana Cloud, OTEL exporters, or paid log vendors. No tokens, prompts, tool argument blobs, HMAC, passwords, or vendor errors that could contain one in this record.

## Live surfaces inspected (worker, before edits)

| Surface | URL | Observation (operator questions, not pixels) |
| --- | --- | --- |
| Dewee Traces | `http://127.0.0.1:18791/traces` | Heading Traces; Refresh; search; All agents / All channels / All statuses; Advanced; table Name, Tokens, Spans, Time; 3 items; Rows 20; Page 1 of 1. Token/latency/status summaries on rows. |
| Dewee Realtime Events | `http://127.0.0.1:18791/events` (also `/realtime-events` 200 HTML) | Heading Realtime Events; Live + 0 events; Pause; Clear; type chips All / Task / Message / Agent / Team CRUD / Agent Link; All users / All chats; empty “No events yet” while waiting. |
| Dewee Activity | `http://127.0.0.1:18791/activity` | Heading Activity Log; Refresh; All Actions / All Entities; columns Action, Actor, Entity, Entity ID, IP Address, Time; 37 items; Rows 20; Page 1 of 2. No live stream, no export. |
| Dewee Logs | `http://127.0.0.1:18791/logs` | Heading Logs; Log level combobox (INFO selected); Start primary; Clear disabled until start; Filter logs; DEBUG/INFO/WARN/ERROR chips; 0/0; “Click Start to begin streaming logs.” |
| goso MONITORING tabs | App tabs `traces` `events` `activity` `logs` | Nav present under MONITORING. Pages used `SectionHeader`, not SPEC 120 PageChrome/PageStatus. Unauth APIs 401. |

HTTP probes before edits: Dewee `/traces` `/events` `/realtime-events` `/activity` `/logs` 200 HTML; goso `:3000/` and `/healthz` 200 `{"ok":true,...}`; goso `/api/traces` `/api/events` `/api/activity` `/api/logs` `/api/events/stream` `/api/logs/stream` 401 `{"error":"unauthorized"}`. CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`, Dewee `:18791` pid `11744`, Vite `:3000` pid `33438` left running. No Grafana/OTEL Cloud call, no export, no SSE success claimed, no credential lines recorded.

## What changed

Shared CORE chrome (`PageChrome`, `PageStatus`) plus `classifyPageState` / `inventoryBlocksMutation` so loading / true-empty / filtered-empty / error / permission / stream-vs-history / stale are exclusive. Error and permission never render a zero-count empty claim. Stream start, filters, and pagination stay closed while required inventory is error or permission. Refresh remains the retry. Independent failures keep provenance (list vs detail; history vs SSE).

| Page | Operator contract |
| --- | --- |
| Traces | First-class title. Refresh is the primary. Search + status/agent/channel/time-range. Hash-stable `#traces/:id`. Token/latency/status on list and detail. Error groups and truncated flags are labeled. Detail load failure is a dependency, not inventory empty. `publicHasSecrets` still drops prompt/tool payloads from the list. |
| Events | First-class title. Refresh is the primary (Resume when paused). Historical list + optional SSE. Type/actor/kind/connector filters. Stream chrome is exclusive (`off` / `connecting` / `live` / `paused` / `reconnect` / `error`). Clear local view is local only. Activity audit is labeled as a separate page. |
| Activity | First-class title. Refresh is the primary. Action/actor/entity/IP/time filters. Cursor pagination provenance (`before` / `next_before`). No live stream, no export. Public-shape leak is a warning, not permission chrome. Mutations/filters closed on 401. |
| Logs | First-class title. Start is the primary when the tail is off; Resume when paused; Refresh when live. Historical GET and SSE are separate cards with separate failure chrome. Level/component/text filters. Clear local view is local only. Credential-shaped lines stay dropped by `asPublicLog`. |

i18n vi+en for all new operator copy (1860 keys match). GET listings still drop secret-shaped rows. Clear-local-view never calls a delete API.

## Checks

```
cd control-plane && npm test && npm run typecheck && npm run build
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- `npm test`: 265/265 pass (includes traces detail-vs-inventory, filtered empty, stale/401; events history-vs-stream, pause/resume/backoff, local clear; activity permission/empty/filtered-empty/cursors; logs history-vs-SSE, local clear, credential drop).
- `npm run typecheck`: pass.
- `npm run build`: pass.
- `agpl-check` and `agpl-check-docs`: exit 0.

## DI-only gaps (honest unavailable, no fake live action)

- Dewee Traces “Advanced” query builder and Name-column prompt preview. Goso keeps redacted id/agent/status/latency/token columns; raw prompts stay out of the list.
- Dewee Events default-on WebSocket with user/chat chips. Goso uses optional SSE plus historical GET; no chat-id filter API.
- Activity export (none on goso).
- Grafana Cloud, OTEL Cloud exporters, paid log vendors, unbounded retention.
- Claiming SSE live success while `/api/events/stream` or `/api/logs/stream` returns 401.

## Out of scope

Merge and Vite `:3000` restart belong to Codex CTO / advisor live QC. CRM `:8082`, sidecar `:8091`, Dewee `:18791` untouched.

No credentials or secret values are included in this record.

## Advisor live QC (CTO credit exhausted)

Date: 2026-08-30. Codex CTO did not repeat the browser checks. Grok advisor ran them after merge `b38816f` (`Merge SPEC 125 MONITORING operator UX`, `--no-ff`) of `6be84b3` + `8ac1070` on top of SPEC 124 `c45dc8a`. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. No tokens, prompts, or vendor secrets in this record.

Restart: Vite `:3000` only (new listen pid `70496`). Unchanged: CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`, gateway `:18080` pid `68421`. Dewee `:18791` not bound or killed.

Advisor re-ran `npm test` (265/265) and `npm run typecheck` on the worker worktree before merge. Source and QA AGPL checks passed. i18n en/vi key sets match (1860).

Browser: Orca isolated profile `qc120-unauth` (no `goso_token`). Hard-reload `http://127.0.0.1:3000/`, then MONITORING Traces / Events / Activity / Logs.

| Defect | Live unauth (401) | Verdict |
| --- | --- | --- |
| Traces without PageChrome + empty on 401 | First-class `Traces`. Refresh primary. List meta `—`. 401. No inventory-empty claim. | PASS |
| Events empty-while-error + stream looking live | First-class `Nhật ký`. Refresh enabled. `Tạm dừng` and `Xóa view local` `disabled=true`. 401. | PASS |
| Activity empty + filters during 401 | First-class `Hoạt động`. Refresh primary. List meta `—`. 401. No export. | PASS |
| Logs Start enabled + empty on 401 | First-class `Log`. `Bắt đầu` `disabled=true`. Pause/clear local disabled. History 401 separate from stream chrome. | PASS |

### Advisor verdict: PASS — SPEC 125 closed

Do not spawn a second 125 worker. Sequential 126+ only when asked. CRM `:8082` and sidecar `:8091` remain untouched.
