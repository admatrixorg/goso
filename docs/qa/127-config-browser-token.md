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
