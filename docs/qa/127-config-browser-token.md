# QA — SPEC 127 Config browser admin token

Date: 2026-08-31. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. Worker does not merge and does not restart Vite `:3000`. Demos `:8082` `:8091` were not bound or killed. No vendor/S3/Grafana/SSO/channel success. No token literals or token-file contents in this record.

## Live surfaces inspected (worker, before edits)

| Surface | URL | Observation |
| --- | --- | --- |
| Control Plane | `http://127.0.0.1:3000/` | 200. Vite demo. No login page. |
| Health (proxy) | `http://127.0.0.1:3000/healthz` | 200 `{"ok":true,...}`. |
| Gateway health | `http://127.0.0.1:18080/healthz` | 200 `{"ok":true,...}`. |
| Agents without bearer | `http://127.0.0.1:18080/api/agents` | 401 `{"error":"unauthorized"}`. |
| Config without bearer | `http://127.0.0.1:18080/api/config` | 401 `{"error":"unauthorized"}`. |

Cause: `authHeader()` uses `VITE_GOSO_ADMIN_TOKEN` (unset in this Vite demo) then `localStorage.goso_token`. Config → Gateway → Auth previously had only process `token_set` booleans plus a DevTools-oriented hint. Unauth left-nav pages therefore 401.

HTTP probes did not send a bearer and did not read any token file. CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`, Vite `:3000` pid `69567`, gateway `:18080` pid `68421` left running.

## What changed

Write-only **Control Plane browser token** on Config → Gateway → Auth, always rendered even when gateway inventory is permission/error. Password input, `autocomplete="off"`, empty on load, never hydrated. Save trims, rejects empty, writes `localStorage.goso_token` only, clears React state, then reloads immediately (no probe delay). Clear removes the key and reloads. Env-owned (`VITE_GOSO_ADMIN_TOKEN` non-empty) disables input, Save, and Clear. Status line is env-owned / set / not set — never the secret body. Process `token_set` / `view_token_set` / `master_key_set` stay on the same Auth card when GET `/api/config` succeeds. Copy distinguishes browser bearer from gateway process tokens. Optional probe labels are derived from the existing GET `/api/config` page state (accepted / still unauthorized / unreachable), not from a pre-reload extra request. Config leak errors use a dedicated string, not the browser-token hint. i18n vi+en.

## Checks

```
cd control-plane && npm test && npm run typecheck && npm run build
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- `npm test`: 286/286 pass (includes `browser-token.test.ts`: empty-on-load, env-owned write block, save writes localStorage and clears typed state, clear removes key, 401 inventory still allows the control, probe status-only).
- `npm run typecheck`: pass.
- `npm run build`: pass.
- i18n en/vi key sets match (1877).
- `agpl-check` and `agpl-check-docs`: exit 0.

## Out of scope

Merge `--no-ff` and Vite `:3000` restart belong to advisor/CTO live QC. CRM `:8082` and sidecar `:8091` untouched. No login route. No gateway Go auth change. Live browser confirmation of the new Auth field waits for the post-merge `:3000` restart.

No credentials or secret values are included in this record.

## Advisor live QC

Date: 2026-08-31. After `worker_done` on `task_8018ddcef219` / `ctx_121e1d303f1d`. Merged `--no-ff` as `6226aab` (`Merge SPEC 127 Config browser token`) of `eee2caf` + `1343767` + `5662d25` + `4fdd274` onto `de43d0e`. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. No token literals or token-file contents in this record.

Restart: Vite `:3000` only (new listen pid `46335`). Unchanged: CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`, gateway `:18080` pid `68421`.

Advisor re-ran `npm test` (286/286), `npm run typecheck`, `npm run build` on the worker worktree before merge. `agpl-check` and `agpl-check-docs` exit 0. `persistBrowserToken` writes `localStorage.goso_token` only and reloads; it does not call `settingsApi.putGateway`.

HTTP (status only): `GET /api/agents` without bearer 401; with the demo file token 200. Bodies not copied here.

Browser (Orca tab, start with no `goso_token`):

| Check | Live | Verdict |
| --- | --- | --- |
| 401 still shows the Auth control | Config → Gateway → Auth visible with 401 alert. Password empty. Save enabled. Clear disabled. Status: browser token not set. Process `token_set` rows hidden while inventory is blocked. Gateway process Save disabled. | PASS |
| Save writes browser token and reloads | After Save, chrome `Gateway · connected`. `localStorage.goso_token` present (boolean only). Password input empty. Overview no longer unauthorized (uptime/requests/WebSocket figures load). | PASS |
| Status + probe never show the secret | Auth card: “Browser token set”; status “Gateway accepted token”. Process booleans remain (`Admin token set` / `View token set` / `Master key set`). Snapshot of the password field has no value. | PASS |

Clear on the live demo was left unclicked so the operator session stays authenticated; unit tests cover Clear removing the key.

### Advisor verdict: PASS — SPEC 127 closed

Do not spawn a second 127 worker. CRM `:8082` and sidecar `:8091` remain untouched.
