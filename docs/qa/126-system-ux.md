# QA — SPEC 126 SYSTEM operator UI/UX

Date: 2026-08-30. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. Worker does not merge and does not restart Vite `:3000`. Demos `:8082` `:8091` `:18791` were not bound or killed. No paid provider, package-registry, SSO, Stripe, K8s, or import-secret success. No tokens, API keys, CLI creds, archive secrets, or vendor errors that could contain one in this record.

## Live surfaces inspected (worker, before edits)

| Surface | URL | Observation (operator questions, not pixels) |
| --- | --- | --- |
| Dewee Tenants | `http://127.0.0.1:18791/tenants` (SPA route `/admin/tenants`) | Heading Tenants; Create Tenant primary; Refresh; table Name, Slug, Status, Created; Master / master / Active; current tenant in chrome `system (Master)`. |
| Dewee Providers | `http://127.0.0.1:18791/providers` | Heading Providers; Add Provider; key-set / enabled / subscription-connected rows; 3 items; pagination. |
| Dewee API Keys | `http://127.0.0.1:18791/api-keys` | Heading API Keys; Create API Key; Refresh; Name, prefix mask, Scopes, Tenant, Status, Expiry, Last Used, Actions; 2 items. |
| Dewee Packages | `http://127.0.0.1:18791/packages` | Runtime & Packages; Refresh; runtimes python3/pip3/node/npm; tabs System / Python / Node / GitHub / CLI Credentials; Install; empty system packages. |
| Dewee Config | `http://127.0.0.1:18791/config` | Configuration; Refresh; Server / Behavior / AI Defaults / Quota / Tools / Integrations; Auth Token env-owned. Backup is a separate Dewee nav item — goso keeps it Settings-bounded. |
| Dewee Approvals | `http://127.0.0.1:18791/approvals` | Approvals; Pending execution approvals; Refresh; empty inbox “No pending approvals”; realtime copy. Distinct from pairing. |
| Dewee Import & Export | `http://127.0.0.1:18791/import-export` | Import & Export Beta; Teams / Agents / Skills & MCP / Export / Import; team export includes member agents. |
| goso SYSTEM tabs | App tabs `tenants` `providers` `apikeys` `packages` `settings` `approvals` `impexp` | Nav present under SYSTEM. Pages used `SectionHeader`, not SPEC 120 PageChrome/PageStatus. Unauth APIs 401. Backup stays inside Settings. |

HTTP probes before edits: Dewee `/tenants` `/providers` `/api-keys` `/packages` `/config` `/settings` `/approvals` `/import-export` 200 HTML; goso `:3000/` and `/healthz` 200 `{"ok":true,...}`; goso `/api/tenants` `/api/providers` `/api/api-keys` `/api/packages` `/api/config` `/api/approvals` `/api/import-export` `/api/import-export/catalog` 401 `{"error":"unauthorized"}`. CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`, Dewee `:18791` pid `11744`, Vite `:3000` pid `70496`, gateway `:18080` pid `68421` left running. No paid provider call, no package install, no secret reveal, no archive download of credentials.

## What changed

Shared CORE chrome (`PageChrome`, `PageStatus`) plus `classifyPageState` / `inventoryBlocksMutation` so loading / true-empty / filtered-empty / error / permission / dependency / not-configured / stale are exclusive. Error and permission never render a zero-count empty claim. Mutations stay closed while required inventory is error or permission. Independent CRM vs gateway failures on Config keep provenance. Stale keeps last-known data only for non-auth refresh failures, labeled with last-load time.

| Page | Operator contract |
| --- | --- |
| Tenants | First-class title. Create tenant is the primary. Refresh + search. Current/master context visible. Deactivate/remove require typed named confirmation. Detail GET failure is a dependency, not inventory empty. Isolation is an API fact, not a UI claim. |
| Providers | First-class title. Add provider is the primary. Refresh + type/source/enabled filters. Key inputs start empty and never rehydrate. GET metadata only (`key_set` / source). Test is latency/models/fail, not vendor success. Env-owned rows stay read-only. |
| API Keys | First-class title. Create is the primary. Refresh + search. One-time secret leaves UI memory after copy, Hide, or navigation. GET prefix-only. Typed named revoke. Scope toggles stay. |
| Packages | First-class title. Refresh is the primary. Allow/install/uninstall/recover require named confirmation. Install stays closed when the local runtime is missing. CLI creds are write-only and separate from provider keys. Partial/failed jobs are not success. |
| Config | First-class title uses SYSTEM nav label Config. Refresh is the primary. CRM org settings, gateway `/api/config`, and backup are labeled with provenance. Env-owned fields stay read-only. CRM 401 does not look like empty gateway config. Billing remains a developing stub. Backup stays Settings-bounded. |
| Approvals | First-class title. Refresh is the primary. Execution-approval inbox; pairing stays on Channels/Nodes. Expired/stale/already-resolved are exclusive. Args stay redacted. Named deny reason required. |
| Import & Export | First-class title. Export is the primary (closed on catalog 401). Catalog/export/preview/import/rollback. Dry-run labeled separately from apply. Archives public-shaped; secrets excluded. Skills `configured=false` is not-configured, not empty catalog. |

i18n vi+en for all new operator copy (1867 keys match). GET listings still drop secret-shaped rows. Backup remains a panel on Config.

## Checks

```
cd control-plane && npm test && npm run typecheck && npm run build
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- `npm test`: 280/280 pass (includes tenant permission-vs-empty/filtered-empty/stale; provider write-only and inventory block; API key one-time hideCopiedSecret; package CLI write-only and install block; approval inbox-vs-empty and redacted args; import catalog 401 vs empty and public-shape; settings CRM-vs-gateway provenance).
- `npm run typecheck`: pass.
- `npm run build`: pass.
- `agpl-check` and `agpl-check-docs`: exit 0.

## DI-only gaps (honest unavailable, no fake live action)

- Dewee Backup & Restore as a seventh SYSTEM nav item. Goso keeps backup inside Config (SPEC 117).
- Dewee provider “subscription connect” live vendor success. Goso tests via `/api/providers/:name/test` and reports latency/models/fail only.
- Dewee package install into a live runtime container when the local runtime is missing. Goso labels runtime-missing and keeps Install closed.
- SSO, Stripe/K8s, paid provider, package-registry, or import-secret success.
- Claiming create/revoke/install/import success while the inventory API returns 401.

## Out of scope

Merge and Vite `:3000` restart belong to Codex CTO / advisor live QC. CRM `:8082`, sidecar `:8091`, Dewee `:18791` untouched.

No credentials or secret values are included in this record.
