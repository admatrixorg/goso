# QA — SPEC 123 CAPABILITIES operator UI/UX

Date: 2026-08-30. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. Worker does not merge and does not restart Vite `:3000`. Demos `:8082` `:8091` `:18791` were not bound or killed. No live paid vendors. No tokens, HMAC keys, MCP env values, API keys, hook payloads, or private messages in this record.

## Live surfaces inspected (worker, before edits)

| Surface | URL | Observation (operator questions, not pixels) |
| --- | --- | --- |
| Dewee Skills | `http://127.0.0.1:18791/skills` | Title Skills; Refresh; Rescan Deps; Install all dependencies; Core (9) / Custom (0); missing-deps install; counts Total/Needs attention/Missing deps/Disabled/Archived/Unmanaged; search; status filters; pagination 9 items / Rows 20 / Page 1 of 1; per-skill enable + Edit Skill; bulk checkboxes. |
| Dewee Built-in Tools | `http://127.0.0.1:18791/builtin-tools` | Title “Manage system built-in tools. Enable/disable or configure settings globally.”; Refresh; grouped tools with descriptions and Settings. |
| Dewee MCP Servers | `http://127.0.0.1:18791/mcp` | Title “Manage Model Context Protocol server connections”; Add Server + Refresh; search; truthful empty “No MCP servers” / “Add your first MCP server to get started.” |
| Dewee Add MCP Server dialog | same page, dialog only, not submitted | Name, Display Name, transport stdio/SSE/Streamable HTTP, Command, Args, env var name+value, agent hints, tool prefix, timeout 60s, Enabled, Require User Credentials, Test Connection, Cancel, Create, Close. |
| Dewee TTS | `http://127.0.0.1:18791/tts` | Title “Configure TTS providers and auto-apply settings”; Refresh + Save; Primary Provider including None (Disabled); Auto-Apply Off/Always/Inbound/Tagged; Reply Mode; Max Text Length; Timeout. |
| Dewee Cron | `http://127.0.0.1:18791/cron` | Title “Schedule recurring agent tasks”; Refresh + New Job; search; truthful empty “No cron jobs”. |
| Dewee Create Cron Job dialog | same page, dialog only, not submitted | Name, Agent ID, Schedule Type Every/Cron/Once, Interval seconds, Message, Cancel, Create. |
| Dewee Hooks | `http://127.0.0.1:18791/hooks` | Title “Agent Hooks”; lifecycle interceptors (beta); event/handler/test guidance; pii-redactor rows. Not HTTP webhooks. |
| goso Skills / Tools / MCP / Cron | App tabs `skills` `tools` `mcp` `cron` | Shared heading `Functions`; `non-JSON response` with `0 tools` / `0 connectors` / `0 skills` / `0 jobs`; enabled create/toggle forms; global pick-agent. |
| goso TTS | App tab `tts` | `401 unauthorized` together with `not configured`, `no API key`, and enabled Save/Test/Clear. |
| goso Webhooks | App tab `webhooks` | `401 unauthorized` with enabled Create form, `0 webhooks`, and last-secret empty panel. |
| goso Connectors | App tab `connectors` | `non-JSON response` with prefilled register fields, `0 connectors` / `No connectors`, and assign controls. |

HTTP probes before edits: Dewee `/skills` `/builtin-tools` `/mcp` `/tts` `/cron` `/hooks` 200 HTML; goso `:3000/` and `/healthz` 200; goso `/api/skills` `/api/agents` `/api/connectors` `/api/tts` `/api/cron` `/api/webhooks` `/api/sessions` 401 `{"error":"unauthorized"}`. CRM `:8082`, sidecar `:8091`, Dewee `:18791`, Vite `:3000` were left running. No dependency install, no MCP connect, no outbound webhook, no cron run, no TTS vendor call.

## What changed

Shared CORE chrome (`PageChrome`, `PageStatus`) plus `classifyPageState` / `classifyToolView` so loading / true-empty / filtered-empty / error / permission / dependency / stale are exclusive. Error and permission never render a zero-count empty claim. Mutations stay closed while inventory is error or permission. MCP Servers and Connectors share one `ConnectorPanel` inventory.

| Page | Operator contract |
| --- | --- |
| Skills | First-class title. Create is the primary (form closed until opened). Search. Archive confirms the named skill. Dependency rescan/install, enable/edit, bulk, and richer status are unavailable copy. |
| Built-in Tools | First-class title. Requires a loaded agent inventory and a selected agent. Distinguishes no-agent, no-selection, 404/unsupported, true empty, filtered empty, permission, stale. Toggle is per-agent; global settings stay unavailable. |
| MCP Servers | First-class title. Add connector is the primary. Search. Name/transport/endpoint-or-command, write-only token, env-var name, enabled, test. Display name, args, env values, hints, prefix, timeout, per-user credentials, and stored-token clear have no API. Same inventory as Connectors; assignment is on Connectors. |
| Connectors | goso-only extra on the same inventory. Registration + agent assignment. Agent-list failure is a labeled dependency, not a false empty catalog. |
| TTS | First-class Save + Refresh. GET shape-filter. API key starts empty. Save/Test/Clear disabled in permission. Not-configured / disabled / ready are distinct from 401. Test never assumes vendor success. Typed clear still names the provider. |
| Cron | First-class title. Create is the primary. Session-backed `every:Nm\|Nh` or five-field UTC. Once/agent-target stay unavailable. Job errors and session-list errors are independent. Delete confirms the named spec. |
| HTTP Webhooks | First-class title. Explicit HTTP-versus-lifecycle copy. Create form closed while unauthorized. Prefix-only GET. One-time token/HMAC disposed after copy, Hide, or leaving the page. Test/replay require an outbound http(s) URL. Rotate/revoke name the target. |

i18n vi+en for all new operator copy. GET listings still drop secret-shaped rows.

## Checks

```
cd control-plane && npm test && npm run typecheck && npm run build
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- `npm test`: 233/233 pass (includes tool selection kinds, cron spec/once rejection, connector transport validation, write-only connector body, one-time webhook dispose, named skill/cron confirms, TTS permission-over-empty).
- `npm run typecheck`: pass.
- `npm run build`: pass.
- `agpl-check` and `agpl-check-docs`: exit 0.

## DI-only gaps (honest unavailable, no fake live action)

- Skill dependency rescan, install, missing-deps diagnosis, enable/edit, bulk select, Core/Custom/archived status.
- Built-in Tools global enable and per-tool Settings.
- MCP display name, structured args, env values, agent/per-tool hints, tool prefix, create-form timeout, per-user credentials, stored-token clear (empty PATCH keeps the token).
- Cron once/at schedules and agent-target jobs (goso jobs target a session).
- Lifecycle Agent Hooks (goso page is signed HTTP webhooks).
- Live vendor TTS success, MCP connect success, webhook delivery, cron execution.

## Out of scope

Merge and Vite `:3000` restart belong to Codex CTO. CRM `:8082`, sidecar `:8091`, Dewee `:18791` untouched.

No credentials or secret values are included in this record.

## Advisor live QC (CTO credit exhausted)

Date: 2026-08-30. Codex CTO did not repeat the browser checks. Grok advisor ran them after merge `0820bac` (`Merge SPEC 123 CAPABILITIES operator UX`, `--no-ff`) of `9a5571d` + `4b3143d` on top of SPEC 122 `d19c979`. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. No tokens, HMAC keys, MCP env values, or private messages in this record.

Restart: Vite `:3000` only (new listen pid `1885`). Unchanged: CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`, gateway `:18080` pid `68421`. Dewee `:18791` not bound or killed.

Advisor re-ran `npm test` (233/233) and `npm run typecheck` on the worker worktree before merge. Source and QA AGPL checks passed. i18n en/vi key sets match (1813).

Browser: Orca isolated profile `qc120-unauth` (no `goso_token`). Hard-reload `http://127.0.0.1:3000/`, then CAPABILITIES Skills / Tools / MCP / TTS / Cron / Webhooks / Connectors.

| Defect | Live unauth (401) | Verdict |
| --- | --- | --- |
| Shared Functions heading + 0-count empty + enabled create | Skills first-class title. `Tạo skill` `disabled=true`. No `0 skills` / empty-on-error. | PASS |
| Tools `0 tools` + Pick an agent during agent-inventory 401 | Tools list meta `—`. 401. No `0 tools` / pick-agent empty. | PASS |
| MCP `0 connectors` + Add form | First-class `MCP Servers`. `Thêm connector` `disabled=true`. | PASS |
| TTS 401 labeled not-configured + enabled Save/Test | First-class `TTS`. `Lưu` `disabled=true`. `not configured` not used as the 401 state. | PASS |
| Cron `0 jobs` + create during error | First-class `Cron`. `Tạo job` `disabled=true`. Job 401 and session 401 shown separately. List meta `—`. | PASS |
| Webhooks 401 + Create + `0 webhooks` | First-class `Webhook HTTP`. `Tạo webhook` `disabled=true`. HTTP-versus-lifecycle copy present. | PASS |
| Connectors 401 + prefilled register + `0 connectors` | `Đăng ký` `disabled=true`. Same inventory note as MCP. No false-empty count. | PASS |

### Advisor verdict: PASS — SPEC 123 closed

Do not spawn a second 123 worker. Sequential 124+ only when asked. CRM `:8082` and sidecar `:8091` remain untouched.
