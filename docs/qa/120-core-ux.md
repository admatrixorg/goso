# QA — SPEC 120 CORE operator UI/UX

Date: 2026-08-30. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. Worker does not merge and does not restart Vite `:3000`. Demos `:8082` `:8091` `:18791` were not bound or killed. No live paid vendors. No secrets, prompts, or private message bodies in this record.

## Live surfaces inspected (worker, before edits)

| Surface | URL | Observation (operator questions, not pixels) |
| --- | --- | --- |
| Dewee Overview | `http://127.0.0.1:18791/overview` | Overview/Usage tabs; connection `Connected · dev`; requests/tokens/cost; agents `0 / 2 running`; channels `1 / 2 online`; system health (uptime, database, providers, tools, sessions, clients); runtimes; connected clients table; cron empty; recent requests with status. |
| Dewee Chat | `http://127.0.0.1:18791/chat` | Agent selector, New Chat, list rows with message count + timestamp + delete, no-selection “Start a conversation”. |
| Dewee Agents | `http://127.0.0.1:18791/agents` | Agent Transfer + Create Agent; search; type/creator filters; list/card toggles; status/provider/model; prompt/evolution; context limit; pagination; named delete. |
| Dewee Teams | `http://127.0.0.1:18791/teams` | Peer tabs Agent Teams / Agent Links. Teams: search, Create Team, truthful empty. Links: reachable with zero teams; source/target/direction/status/description/actions; Create Link; empty “No agent links configured.” |
| goso Overview | `http://127.0.0.1:3000` tab `crm` | Chrome `Gateway · connected` while page `Gateway · unauthorized`; KPI/cards hidden; `agents/sessions/channels: non-JSON response`; CRM extra `goso-crm online` plus `401 unauthorized` and “0 tips / No advice yet”. |
| goso Chat / Agents / Teams | App tabs `chat` `agents` `teams` | Defects 3–8 as locked: empty+error together, always-open create forms, links buried in selected-team detail. |

HTTP probes before edits: Dewee `/overview` `/chat` `/agents` `/teams` 200; goso `:3000/` and `/healthz` 200.

## What changed

Shared CORE chrome (`PageChrome`, `PageStatus`) plus `classifyPageState` so loading / true-empty / error / permission / stale are exclusive. Error and permission never render a zero-count empty claim. Stale keeps last-known data only when labeled with last-load time.

| Page | Operator contract |
| --- | --- |
| Overview | Chrome and Overview both use `combineGatewayKind(healthz, /api/stats)`. Unauthorized/offline still show the supported questions with `—`, not a wipe. Usage/tokens/cost, clients, runtimes, recent-request table, database/provider/tool counts are honest unavailable. CRM is a labeled goso extra; CRM 401 is not “0 tips”. |
| Chat | One New Chat primary on the session list. List shows agent + `created_at` activity. Message count, attachments, voice, and context usage are marked unavailable (no fake-live controls). Send stays disabled until there is text. SSE connecting/streaming/reconnect/error remain. Session delete confirms the named target. |
| Agents | Create is the primary action; the form opens for create/edit only. Transfer is a non-action unavailable badge (no 404 button). Provider/model and orchestration vs prompt-mode copy at the decision point. 409 stale conflict offers reload. Delete confirms the named agent. |
| Teams | Peer views Agent Teams and Agent Links. Links stay reachable with zero teams, with source/target/direction plus unavailable status/description. Team delete is typed-name confirm; member remove and unlink are named/directional confirms. Existing member/task/mailbox/evolution depth remains on a selected team. |

i18n vi+en for all new operator copy. No credential hydration.

## Checks

```
cd control-plane && npm test && npm run typecheck && npm run build
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- `npm test`: 186/186 pass (includes `page-state`, `confirm`, `combineGatewayKind`, stale overview, link merge).
- `npm run typecheck`: pass.
- `npm run build`: pass.

## DI-only gaps (honest unavailable, no fake live action)

- Overview Usage charts, token in/out, cost.
- Connected-client table, runtime inventory, recent-request table.
- Database/providers/tools counts on Overview (operator is pointed at Providers / Functions).
- Agent Transfer (no goso API).
- Chat attachment, voice, context-usage meter, session message counts (list JSON has `created_at` only).
- Agent-link status and description columns (link JSON is source/target/direction only).

## Out of scope

Heatmap except shared chrome compatibility (unchanged tab). Merge and Vite `:3000` restart belong to Codex CTO. CRM `:8082`, sidecar `:8091`, Dewee `:18791` untouched.

No credentials or secret values are included in this record.
