# QA — SPEC 130 CRM 401 chrome is not gateway token failure

Date: 2026-08-31. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. Worker does not merge and does not restart Vite `:3000`. Demos `:8082` `:8091` were not bound or killed. No vendor/S3/Grafana/SSO/channel success. No token literals or token-file contents in this record. `/tmp/goso-cp-demo/crm-login.txt` and process `GOSOCRM_ORG_TOKEN` were not copied.

## Live surfaces inspected (worker, before repo edits)

Verified by the dispatch (after SPEC 127 token save; chrome Gateway · connected; `localStorage.goso_token` set; length not copied here):

| Surface | Observation |
| --- | --- |
| `#/agents` | Gateway · connected. `GET /api/agents` 200 JSON. Create enabled. No 401. |
| `#/sessions` `#/channels` `#/config/gateway` | Connected, no 401. |
| `#/config` (default subpage was Account) | `Not authorized for this API. · Error: 401 {"error":"unauthorized"}` because Config opened CRM Account (`/crm-api/api/settings/account`) which needs X-Org-Token. |
| `#/heatmap` | Raw `401 {"error":"unauthorized"}`. |
| `#/marketing` | CRM permission + “CRM refused authorization — separate from the gateway.” |
| Overview KPI | Already said CRM refused — keep that split. |
| `/crm-api/healthz` | 200. Proxy to live CRM `:8082` is fine. CRM inventory 401 without org token. |

Cause: `#/config` defaulted to CRM Account. CRM 401 chrome concatenated `common.permission` with a JSON body and looked like a gateway token failure. `crm.ts` only sent `X-Org-Token` from unset `VITE_GOSOCRM_ORG_TOKEN`.

HTTP probes in this worker run did not print token values or token-file contents. CRM `:8082` and sidecar `:8091` left running. Vite `:3000` not restarted.

## What changed

Default Config hash is Gateway. `#/config` and empty settings hash parse to `gateway`; `#/config/account` still opens Account. CRM surfaces (Overview CRM block, Heatmap, Marketing, Config CRM tabs) use dedicated `crm.permission` copy: org unauthorized / org token not set; not the gateway admin token. `formatCrmPublicError` and PageStatus `permissionText` do not dump `401 {"error":"unauthorized"}`. Write-only CRM org token on Config → Account: password empty on load, never hydrated; Save writes `localStorage.goso_crm_org_token` then reloads; Clear removes the key; `VITE_GOSOCRM_ORG_TOKEN` wins and disables write; control stays visible on CRM 401. `crmOrgHeaders` uses env or localStorage. Distinct from `goso_token`. No fake CRM KPI 200.

## Checks

```
cd control-plane && npm test && npm run typecheck && npm run build
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- `npm test`: 314/314 pass (includes `hash-route.test.ts`: `#/config` → gateway, `#/config/account` stays account; `crm.test.ts`: CRM 401 copy has no raw JSON, headers include localStorage token when env empty, env wins; `crm-org-token.test.ts`: empty-on-load, env-owned disables write, save writes `goso_crm_org_token` and clears typed state, clear removes key, 401 inventory still allows the control; `public-error.test.ts`: `formatCrmPublicError` does not dump 401 JSON).
- `npm run typecheck`: pass.
- `npm run build`: pass.
- `agpl-check` and `agpl-check-docs`: exit 0.

## Out of scope

Merge `--no-ff` and Vite `:3000` restart belong to advisor/CTO live QC. CRM `:8082` and sidecar `:8091` untouched. Gateway auth, channel vendor tokens, S3/Grafana/SSO stay DI. CRM Go server unchanged. Live browser confirmation that `#/config` lands on Gateway and Account shows the org-token field waits for the post-merge `:3000` restart.

No credentials or secret values are included in this record.

## Advisor live QC

Date: 2026-08-31. After `worker_done` on `task_eb5e7a2c404a` / `ctx_adbbe63302b3`. Merged `--no-ff` as `0a7d236` (`Merge SPEC 130 CRM 401 chrome vs gateway`) of `b348a5d` + `b1c65cf` onto `c28911f`. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. No token literals or `crm-login.txt` contents in this record.

Restart: Vite `:3000` only (new listen pid `53549`). Unchanged: CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`, gateway `:18080` pid `68421`.

Advisor re-ran `npm test` (314/314), `npm run typecheck`, `npm run build` on the worker worktree before merge. `agpl-check` and `agpl-check-docs` exit 0.

Browser (Orca tab; gateway `goso_token` already set — value not recorded; CRM org token **not** filled):

| Check | Live | Verdict |
| --- | --- | --- |
| `#/config` default | Hash stays `#/config`. Chrome `Gateway · connected`. Auth / Control Plane browser token visible. Not Account display-name. No raw `401 {"error":"unauthorized"}`. | PASS |
| `#/config/account` | Org-token field present. One empty password input. Permission copy without JSON dump. Gateway chrome still connected (split from CRM 401). | PASS |
| `#/heatmap` | CRM copy present. No raw 401 JSON. Gateway · connected. | PASS |
| `#/marketing` | CRM copy present. No raw 401 JSON. Gateway · connected. | PASS |

Did not paste `crm-login.txt` or invent an org token. Save/Clear of `goso_crm_org_token` covered by unit tests.

### Advisor verdict: PASS — SPEC 130 closed

Do not spawn a second 130 worker. CRM `:8082` and sidecar `:8091` remain untouched.
