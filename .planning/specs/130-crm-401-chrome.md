# SPEC 130 — CRM 401 must not look like gateway token failure

Status: implemented (worker). Merge `--no-ff` and Vite `:3000` restart stay with advisor/CTO after SPEC 129 advisor live QC PASS (`c28911f`).
Owner: Grok implementation; advisor/CTO merge and live QC.
Base: `origin/main` at `c28911f`.

## Goal

Pasting the gateway admin token must not leave Config looking like a gateway 401. `#/config` opens Gateway. CRM 401 chrome says org token / X-Org-Token, never dumps `{"error":"unauthorized"}`, and is distinct from the gateway bearer.

## Operator question

“I saved the gateway token (SPEC 127). Agents/Sessions/Channels are connected. Why does Config still show `Not authorized for this API. · Error: 401 {"error":"unauthorized"}`?”

## Cause (verified)

After SPEC 127 save of gateway `goso_token` (length 32): `#/agents` `Gateway · connected`, `GET /api/agents` 200 JSON, Create enabled, no 401. `#/sessions` `#/channels` `#/config/gateway` connected, no 401.

`#/config` opened **CRM Account** (`/crm-api/api/settings/account`) which requires **X-Org-Token**, not the gateway bearer. Vite demo does not set `VITE_GOSOCRM_ORG_TOKEN`. Live CRM `:8082` returns 401 without org token. Demo `/crm-api/healthz` is 200 (proxy is fine). Heatmap dumped raw `401 {"error":"unauthorized"}`. PageStatus concatenated `common.permission` with that JSON body.

This is a Control Plane UX bug, not a gateway token failure.

## Behavior

1. Default Config URL is Gateway. `#/config` and empty settings hash open `gateway`, not `account`. `#/config/account` and the Account/Users/… rail still open those CRM pages. Invalid config segments fall back to gateway.
2. CRM permission chrome is not gateway chrome. Overview CRM block, Heatmap, Marketing, and Config CRM tabs use dedicated i18n (`crm.permission`): CRM org unauthorized / org token not set; not the gateway admin token. They do not concatenate `common.permission` with `Error: 401 {"error":"unauthorized"}`. `formatCrmPublicError` / PageStatus for CRM 401 do not dump JSON body.
3. Write-only CRM org token on Config → Account, modeled on SPEC 127:
   - Password input empty on load. Never hydrate.
   - Save → `localStorage.goso_crm_org_token` (name exact), clear input, reload.
   - Clear removes the key.
   - Env `VITE_GOSOCRM_ORG_TOKEN` wins and disables the field.
   - Visible even when CRM inventory is 401.
   - `crm.ts` `orgHeaders`: env token OR localStorage `goso_crm_org_token`. Never log the value.
   - Distinct from `goso_token`. Label says CRM org token / X-Org-Token.
4. No fake CRM KPI success without a live 200. Do not copy token files or process env into source, QA, or chat.

## Constraints

- Clean-room React/TS. Do not copy GoClaw/Dewee source.
- Secrets never in chat/QA/git (no token literals, no `crm-login.txt` / `GOSOCRM_ORG_TOKEN` contents).
- Worker does not merge and does not restart Vite `:3000`. Never touch CRM `:8082`, sidecar `:8091`, or Dewee `:18791`.

## Non-goals

- Gateway auth, channel vendor tokens, S3/Grafana/SSO.
- Changing the CRM Go server.
- Auto-fill from a local token file.
- Copying GoClaw layouts/source.
- Merge `--no-ff` and Vite restart (advisor/CTO).

## Acceptance criteria

1. `#/config` and `hashForTab("settings")` open Gateway. `#/config/account` still opens Account.
2. CRM 401 chrome uses `crm.permission` (vi+en) and contains no raw JSON body.
3. Config → Account always shows the write-only CRM org token control, including when CRM inventory is 401. Env-owned disables write. Save writes `goso_crm_org_token` then reloads. Clear removes the key.
4. `crmOrgHeaders` sends `X-Org-Token` from env, else localStorage, never logs the value.
5. `cd control-plane && npm test && npm run typecheck && npm run build` pass.
6. `GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` and `./scripts/agpl-check-docs.sh` exit 0.
7. Delivery is two commits (feat vs docs) like SPEC 127/128/129. Merge stays with advisor/CTO.
