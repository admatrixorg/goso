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

## Codex CTO post-merge live QC

Merge: `33e7c78` (`Merge SPEC 120 CORE operator UX`, `--no-ff`)

The CTO independently reran `npm test` (186/186), `npm run typecheck`, `npm run build`, the source AGPL check, and the QA-doc AGPL check; all passed. Only Vite `:3000` was restarted, using the run-specific proxy to gateway `:18080`; CRM `:8082`, sidecar `:8091`, and Dewee `:18791` were not restarted or killed. Live QC then hard-refreshed goso and opened Overview, Chat, Agents, Teams, and the Agent Links peer view, followed by a fresh behavior comparison with live Dewee Teams and Agent Links.

### Acceptance verdict

| Acceptance area | Verdict | Live evidence |
| --- | --- | --- |
| Stable first-class CORE chrome | PASS | Overview, Agents, Teams, and the Teams/Links peer views have clear titles, refresh, filters, and primary-action locations; Chat preserves the session-list/chat split. |
| Gateway/auth consistency | PASS with minor defect | Chrome and Overview both reported `unauthorized`; all unsupported overview figures were `No figure`/unavailable. Overview rendered the same authorization message twice. |
| Loading/empty/error/permission/stale exclusivity | FAIL | Agents and Teams correctly avoided a zero-count/empty claim during the live `non-JSON response`, but Agent Links converted the failed agent inventory into a successful-looking `No agent links yet` state. |
| Mutation gating during blocking failures | FAIL | `Create agent`, `Create team`, and `Create Link` remained enabled during blocking live errors. Chat's `New Chat` was correctly disabled. |
| Honest unsupported behavior | PASS | Agent Transfer, Overview usage/cost/clients/runtimes/recent requests, Chat attachment/voice, and Agent Link status/description remain explicitly unavailable rather than fake-live. |
| CRM extra state provenance | FAIL | With goso-crm offline, the block correctly said metrics were unavailable but still displayed `0 tips` and `No advice yet`, a false-empty claim. |
| Destructive/write-only contracts | PASS by code/test; live blocked | Named/typed confirmations and credential non-hydration tests pass. No destructive action or secret flow was exercised against unavailable live APIs. |
| Vietnamese/English and gates | PASS | i18n typecheck/build and both AGPL checks pass. |

### CTO verdict: FAIL — follow-up required

The follow-up must preserve the upstream agent/team dependency error in Agent Links, prevent `Promise.all([])` from becoming a true-empty result after the agent inventory failed, and disable or hide all create/mutation entry points while their required inventory is in blocking error or permission state. It must also make the offline/permission/error CRM advisor meta `—` (or hide the empty table) instead of `0 tips`/`No advice yet`, and remove the duplicate Overview authorization alert. After the fix merges, the CTO must repeat the same hard-refresh/browser checks before SPEC 120 can close.
