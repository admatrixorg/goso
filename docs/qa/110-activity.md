# QA — SPEC 110 Activity

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Activity: filtered paginated audit table (action, actor, entity, entity ID, IP, time); refresh, filter, page, inspect | `docs/qa/090-goclaw-sidebar-ux.md` Activity |

goso mapping (self-written): live tab `activity` in [App.tsx](../../control-plane/src/App.tsx) renders [ActivityPage](../../control-plane/src/pages/ActivityPage.tsx). Listing is `GET /api/activity` (`/v1/activity` alias) with `action`/`actor`/`entity`/`ip`/`since`/`until` plus `limit`/`before` cursor (default 50, max 200, UI 25). Records are append-only and separate from Events (109). GET copies drop secret keys (`api_key`, `token`, `body`, …) and `sk-`/`Bearer` shapes while keeping before/after metadata such as `enabled`, `status`, `key_set`. View-token GET 200; POST is not a route (403 via view matrix). Export is not implemented.

Out of scope: Logs (111). Tenants (112). Copying GoClaw chrome. Live vendor tokens. SPECs 111–118.

## What changed

- Live nav tab + page. Filters action/actor/entity/IP/time, stable `before` pagination, row detail, loading / empty / error.
- Immutable audit records for configuration and privileged actions (agent CRUD, config, providers, nodes approve/deny/revoke, workstations, storage upload/delete, webhooks create/rotate/revoke, pending compact/clear, contacts merge/undo). Operational events stay on Events. GET never returns secrets.
- i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/110-activity.md`.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/auditlog ./gateway/internal/httpapi ./gateway/internal/auth ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` including `asPublicRecord` dropping `api_key`/`token`/`body` and Bearer shapes, `publicHasSecrets`, `parseDetail` skipping payload keys, `activityQs` + `uniqueField`.
- `go test` auditlog: append-only, secret keys dropped, action/actor/entity/IP/time filters, `before` cursor stable after a later append, cap drops oldest only. httpapi: GET omits `sk-`/`token`/`body`; agent create writes an audit row with IP; Events GET does not mix audit rows; `/v1/activity` alias; view-token GET 200 / POST 403. auth/serve: view GET `/api/activity` 200, POST 403.
- Page copy: “No audit records yet.” / “Chưa có bản ghi kiểm toán.” Expand is `<dl>` text, no `dangerouslySetInnerHTML`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Logs (111). Tenants (112). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 111-118. Export download (authorized-only if added later).
