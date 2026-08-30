# QA — SPEC 121 CONVERSATIONS operator UI/UX

Date: 2026-08-30. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. Worker does not merge and does not restart Vite `:3000`. Demos `:8082` `:8091` `:18791` were not bound or killed. No live paid vendors. No secrets, prompts, or private message bodies in this record.

## Live surfaces inspected (worker, before edits)

| Surface | URL | Observation (operator questions, not pixels) |
| --- | --- | --- |
| Dewee Sessions | `http://127.0.0.1:18791/sessions` | Title + “Browse conversation sessions”; search; columns Session / Agent / Context / Messages / Updated; clickable rows; `45 items`; Rows `20`; Page 1 of 3. |
| Dewee Pending Messages | `http://127.0.0.1:18791/pending-messages` | Title + buffered-channel description; Refresh; How it works (buffer while busy, auto-compaction at 50, inject summary+recent); Compact = LLM summarization now; Clear = permanent delete; truthful empty “No pending messages”. |
| Dewee Contacts | `http://127.0.0.1:18791/contacts` | Title + auto-collected identities; Refresh; channel-permission notes; search by name/username/ID; All Channels / All Types; selectable table NAME / USERNAME / SENDER ID / CHANNEL / TYPE / LAST SEEN; `24 items`; Rows 20; Page 1 of 2. |
| goso Sessions | App tab `sessions` | PageChrome already from SPEC 120: New Chat + Refresh + search/agent filter; live `non-JSON response`; meta `—` (not `0 sessions`); table SESSION / AGENT / Activity / MODE / ACTIONS; New Chat still enabled during error; no Context/Messages/Updated/pagination. |
| goso Pending | App tab `pending` | `Error: 401 {"error":"unauthorized"}` with `0 groups` and table headers. Compact/Clear weight identical. |
| goso Contacts | App tab `contacts` | `401 unauthorized` with `0 contacts`, Merge, and table headers together. |
| goso Marketing | App tab `marketing` | `Error: 500` with Create audience, File / Lead Ads sources, and `No audiences yet`. |

HTTP probes before edits: Dewee `/sessions` `/pending-messages` `/contacts` 200; goso `:3000/` and `/healthz` 200; goso `/api/sessions` `/api/pending-messages` `/api/contacts` `/api/agents` 401 `{"error":"unauthorized"}`. Gateway chrome `Gateway · unauthorized`. CRM extra on Overview was `goso-crm offline` (proxy `/crm-api` → `:8089`; live CRM process is on `:8082`).

## What changed

Shared CORE chrome (`PageChrome`, `PageStatus`) plus `classifyPageState` so loading / true-empty / filtered-empty / error / permission / stale are exclusive. Permission wins over `keepStale` so a later 401 cannot leave mutation forms enabled. In-flight refresh is loading, not stale. `listMetaCount` never reports a numeric zero during blocking states. `inventoryBlocksMutation` closes create/merge/compact/clear while inventory is error or permission. Stale keeps last-known data only for non-auth refresh failures, labeled with last-load time.

| Page | Operator contract |
| --- | --- |
| Sessions | Inventory browsing is primary. New Chat is a dismissible form, gated while blocking. Search/agent filter, prompt-mode, open-in-Chat, named delete. Context, message count, and last-update are labeled unavailable (list JSON has `id`, `agent_id`, `label`, `prompt_mode`, `created_at` only). Full-page pagination + page size; the Chat compact list is the full filtered set (not paged) so a row opened from page 2 still highlights. Agent-list failure does not block the session inventory. |
| Pending Messages | Refresh is the page primary. Counts/age/channel/agent/state only. Compact collapses to a stub count and is visually distinct from irreversible Clear. Typed confirm names the group. Compact-in-progress, 409 busy, 403/401, and mismatch stay on the action. LLM summarization of bodies is disclosed unavailable because bodies are never stored. |
| Contacts | Merge is the primary (two selected). Search, channel/type filters, pagination with offset recovery after merge/undo. Target vs source named, swappable, typed confirm, data-loss copy. Channel permission remains the supported direct/group metadata — no invented vendor-permission matrix. |
| Marketing | Title/description, CRM org (`X-Org-ID`) vs gateway token, refresh, tabs. CRM health is probed first. File / Lead Ads / scheduled / done are metadata-only; create is hidden while blocking. Each tab cites the CRM route. No claim of imported leads, scheduled delivery, or sent messages. |

i18n vi+en for all new operator copy. No credential hydration. GET listings still drop secret-shaped rows in `asPublic`.

## Checks

```
cd control-plane && npm test && npm run typecheck && npm run build
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- `npm test`: 208/208 pass (includes `page-state` filtered-empty / listMetaCount / clampPageOffset / permission-over-stale / in-flight loading, session pagination recovery, pending write kinds, contact mergePair/swap, marketing source/campaign non-execution).
- `npm run typecheck`: pass.
- `npm run build`: pass.

## DI-only gaps (honest unavailable, no fake live action)

- Session context usage and message counts (list JSON has `created_at` only).
- Session last-update distinct from `created_at`.
- Pending LLM summarization of buffered bodies (bodies are never stored; compact is a stub-count collapse).
- Dewee-style auto-compaction threshold / inject-on-next-turn (not a goso listing field).
- Contacts NAME/USERNAME columns as separate Dewee fields (goso has canonical display + channel ids).
- Dewee channel-permission matrix (Telegram/Discord/Zalo/Slack/Feishu/Bitrix) — goso only has direct/group metadata.
- Marketing file import, Lead Ads import, campaign schedule, and channel send (CRM stores name/source/size and draft/scheduled/done records).
- CRM HTTP is organization-scoped via `X-Org-ID`; live demo CRM on `:8082` is not the Vite `/crm-api` `:8089` target.

## Out of scope

Merge and Vite `:3000` restart belong to Codex CTO. CRM `:8082`, sidecar `:8091`, Dewee `:18791` untouched.

No credentials or secret values are included in this record.

## Advisor live QC (CTO credit exhausted)

Date: 2026-08-30. Codex CTO did not repeat the browser checks. Grok advisor ran them after merge `08a76ae` (`Merge SPEC 121 CONVERSATIONS operator UX`, `--no-ff`) of `1b591a4` + `83b7e7d`. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. No secrets, tokens, prompts, or private message bodies in this record.

Restart: Vite `:3000` only (new listen pid `10278`). Unchanged: CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`, gateway `:18080` pid `68421`. Dewee `:18791` not bound or killed.

Browser: Orca isolated profiles `qc120-unauth` (no `goso_token`) and `qc120-auth` (admin token in localStorage only; value not recorded). Hard-reload `http://127.0.0.1:3000/`, then CONVERSATIONS Sessions / Pending / Contacts / Marketing.

Advisor re-ran `npm test` (208/208) and `npm run typecheck` on the worker worktree before merge. Source and QA AGPL checks passed. i18n en/vi key sets match (1714).

### Live defects from the SPEC

| Defect | Live unauth / blocking | Live auth | Verdict |
| --- | --- | --- | --- |
| Sessions error + `0 sessions` / empty + enabled New Chat | `Chat mới` `disabled=true`. Copy `0 phiên` / `Chưa có phiên` absent. `PageStatus` 401. List meta `—`. | Inventory rows present. `Chat mới` enabled. Context / message count / last-update labeled unavailable. | PASS |
| Pending 401 + `0 groups` + Compact/Clear | No Compact/Clear buttons. `0 nhóm` / `Không có tin chờ` absent. 401 + list meta `—`. | True-empty after successful load: `0 nhóm` + `Không có tin chờ.` | PASS |
| Contacts 401 + `0 contacts` + Merge | `Gộp` `disabled=true`. `0 danh bạ` / empty-on-error absent. 401 + list meta `—`. | True-empty `0 liên hệ` + `Không có liên hệ.` Merge stays disabled until two rows are selected. | PASS |
| Marketing error + Create + `No audiences yet` | CRM extra offline (`/crm-api` → `:8089`). `Tạo tệp` `disabled=true`. `No audiences yet` / `Chưa có audience` absent. | Gateway connected; CRM still offline. `Tạo tệp` remains disabled. No false-empty audience claim. | PASS |

### Advisor verdict: PASS — SPEC 121 closed

Do not spawn a second 121 worker. Sequential 122+ stays with Codex CTO / advisor only when asked. CRM `:8082` and sidecar `:8091` remain untouched.
