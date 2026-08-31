# SPEC 128 — Per-menu URLs + CRM proxy 8082

Status: implemented (worker). Merge `--no-ff` and Vite `:3000` restart stay with advisor/CTO after SPEC 127 advisor live QC PASS (`8510fa3`).
Owner: Grok implementation; advisor/CTO merge and live QC.
Base: `origin/main` at `8510fa3`.

## Goal

Every left-nav menu is bookmarkable and restorable from its own hash URL. The Vite `/crm-api` proxy and CRM client default at the live goso-crm port (`8082`) so Overview / Heatmap / Marketing / Config CRM tabs stop failing as a dead-port 500.

## Operator question

“Why does each menu have no URL of its own? Refresh, copy-link, and back dump me on Overview. Remaining menus still show Error/500 after the gateway token.”

## Cause (verified)

`control-plane/src/App.tsx` kept the active menu in React `tab` state only. `go(id)` was `setTab(id)` — no pathname, no `history.pushState`, no hash except a special-case `#traces`. Refresh restored Overview (`crm`).

Repo `control-plane/vite.config.ts` and `crm.ts` `CRM_UPSTREAM_DEFAULT` still targeted `:8089` (nothing listens). Live CRM is `:8082` (pid `85417`). The running demo Vite at `/tmp/goso-cp-demo/vite.config.ts` already proxies `/crm-api` to `8082`; the **repo** config did not, so the next `:3000` restart from this tree would be wrong. CRM `/api/crm/metrics` without an org token is **401** (DI) — that is not a gateway 401.

## Behavior

Hash router (no react-router). Canonical live hashes; Vietnamese labels unchanged:

| Hash | Tab id |
| --- | --- |
| `#/` or `#/overview` | `crm` |
| `#/heatmap` | heatmap |
| `#/chat` | chat |
| `#/agents` | agents |
| `#/teams` | teams |
| `#/sessions` | sessions |
| `#/pending` | pending |
| `#/contacts` | contacts |
| `#/marketing` | marketing |
| `#/channels` | channels |
| `#/nodes` | nodes |
| `#/workstations` | workstations |
| `#/skills` | skills |
| `#/tools` | tools |
| `#/mcp` | mcp |
| `#/tts` | tts |
| `#/cron` | cron |
| `#/webhooks` | webhooks |
| `#/connectors` | connectors |
| `#/memory` | memory |
| `#/vault` | vault |
| `#/kg` | kg |
| `#/storage` | storage |
| `#/traces` | traces |
| `#/traces/<id>` | traces + selected id |
| `#/events` | events |
| `#/activity` | activity |
| `#/logs` | logs |
| `#/tenants` | tenants |
| `#/providers` | providers |
| `#/apikeys` | apikeys |
| `#/packages` | packages |
| `#/config` | settings |
| `#/config/<page>` | settings + SettingsPage `PageId` |
| `#/approvals` | approvals |
| `#/impexp` | impexp |

Rules:

1. Left-nav click (and command palette `go()`) writes the hash and renders that page.
2. Load / hashchange / back-forward restores the same page, including Config subpage and traces id.
3. Unknown hash rewrites to Overview `#/overview` and does not crash.
4. Old `#traces` / `#traces/<id>` rewrite to `#/traces` / `#/traces/<id>`.
5. Demo-only tabs (`home`,`meetings`,`tasks`,`friends`,`calendar`,`gallery`) get hashes only when `VITE_DEMO_MODE`; live mode treats them as unknown.
6. CRM default upstream `http://127.0.0.1:8082`. Keep `/crm-api` rewrite. Do not hardcode an org password/token. After the proxy is live, CRM 401 is unauthorized/offline chrome — never “0 tips”, never fake vendor success. Heatmap does not render true-empty on a CRM error.

## Constraints

- Clean-room React/TS. Inventory from live goso `:3000` + this spec. Do not copy GoClaw/Dewee layouts/wording/source.
- No react-router unless already a dependency (it is not). Tiny parse/serialize module with unit tests.
- Secrets never in chat/QA/git (no token literals, no CRM password, no `admin.token` contents). Do not invent `VITE_GOSOCRM_ORG_TOKEN`.
- Worker does not merge and does not restart Vite `:3000`. Never touch CRM `:8082` or sidecar `:8091`.
- Do not “fix” empty/permission chrome that is correct. KG `/api/kg/graph` still requires `agent_id`. MCP is ConnectorPanel (`/api/connectors`), not `/api/mcp`. Missing channel vendor tokens, S3, Grafana, SSO stay DI.

## Non-goals

- History-fallback path routing (Vite demo has none).
- A login route.
- Fake CRM success without an org token.
- Copying GoClaw layouts/source.
- Merge `--no-ff` and Vite restart (advisor/CTO).

## Acceptance criteria

1. Each live left-nav item has a unique hash; click writes it; refresh/back/forward restore it (Config subpage and traces id included).
2. Unknown hash rewrites to `#/overview` without crashing. Old `#traces` aliases rewrite.
3. Command palette uses the same `go()` helper.
4. Repo Vite `/crm-api` and `CRM_UPSTREAM_DEFAULT` target `8082`. `/crm-api` rewrite kept. No org token in source.
5. CRM 401/offline chrome is honest (no “0 tips”, no empty heatmap success).
6. `cd control-plane && npm test && npm run typecheck && npm run build` pass.
7. `GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` and `./scripts/agpl-check-docs.sh` exit 0.
8. Delivery is two commits (feat vs docs) like SPEC 125–127. Merge stays with advisor/CTO.
