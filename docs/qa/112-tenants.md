# QA — SPEC 112 Tenants

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Tenants: table (name, slug, status, created) plus create/detail; create, open/edit status/access, refresh | `docs/qa/090-goclaw-sidebar-ux.md` Tenants |

goso mapping (self-written): live tab `tenants` in [App.tsx](../../control-plane/src/App.tsx) renders [TenantsPage](../../control-plane/src/pages/TenantsPage.tsx). Listing is `GET /api/tenants` (`/v1/tenants` alias) with `q` search. Detail is `GET /api/tenants/{id}` with members/roles. Create is `POST /api/tenants` `{slug,name}`. Status is `POST /api/tenants/{id}/status` `{status,confirm}` — deactivation requires slug or name confirm and cannot target master `default`. Path `{id}` must be a valid slug; invalid ids 404 and never alias to master. Membership is `POST /api/tenants/{id}/members`, `PATCH .../members/{mid}`, `DELETE .../members/{mid}` with confirm. Current/master context is `GET /api/tenant` plus the list envelope (`current`, `master`, `multi_tenant`). GET copies have no token/secret fields; secret-shaped slug/name/subject are rejected. View-token GET list/context 200; POST 403. Writes against a registered deactivated tenant return 409 (`tenant deactivated`), including WS `op=chat`; the write guard sits inside auth so unauthenticated callers still get 401. Cross-tenant list/get stays 404 (071). Mutations append SPEC 110 audit rows (`entity=tenant`, actions create/status/access). Registry is in-process (same class as nodes/activity); restart drops created tenants.

Out of scope: API Keys (113). Packages (114). Copying GoClaw chrome. Live vendor tokens. SPECs 113–118.

## What changed

- Live nav tab + page. Searchable list/detail, create, status, membership/role visibility, guarded deactivation with confirm. Loading / empty / error.
- API tenant isolation. Current/master context on the page and Settings. Audit every access or status mutation. GET never returns secrets.
- i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/112-tenants.md`.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/tenant ./gateway/internal/httpapi ./gateway/internal/auth ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` including `asPublic` dropping token/`sk-` rows, `publicHasSecrets`, `filterTenants`, `tenantConfirmMatch` / `memberConfirmMatch`.
- `go test` tenant registry: master always present, create/search, deactivate confirm, master cannot deactivate, unregistered stays writable, members/roles reject secret-shaped subject. httpapi: GET omits token/api_key/`sk-`; create/status/access write audit rows; deactivated tenant POST /api/agents 409 while GET still isolates; `/v1/tenants` alias; view-token GET 200 / POST 403. auth/serve: view GET `/api/tenants` 200, POST 403.
- Page copy: “No tenants yet.” / “Chưa có tenant.” Context shows current vs master. Deactivate confirm types slug or name. Expand is text, no `dangerouslySetInnerHTML`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

API Keys (113). Packages (114). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 113-118.
