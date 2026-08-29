# QA — SPEC 105 Nodes / devices

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA. No invented live vendor tokens.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Nodes: combined pending/paired device list; refresh; approve/deny pending; remove paired; empty “No devices.” Pairing codes stay transient. | `docs/qa/090-goclaw-sidebar-ux.md` Nodes |

goso mapping (self-written): live tab `nodes` in [App.tsx](../../control-plane/src/App.tsx) renders [NodesPage](../../control-plane/src/pages/NodesPage.tsx). Channel DM pairing stays on Channels (089/094). Operator list is `GET /api/nodes` (`/v1/nodes` alias) with `pending` and `paired` (id, display, kind, status, health, requested_at, expires_at, approved_at, last_seen). Request is `POST /api/nodes/request` `{display}` (exact POST path; no Bearer). Approve/deny/revoke are `POST /api/nodes/{id}/approve|deny|revoke` with `confirm` matching id or display. GET never returns pairing codes, tokens, or secrets. Pending TTL 10 minutes; paired last-seen older than 5 minutes is `stale`.

Out of scope: Workstations (106). Channel DM pairing UI. Copying GoClaw chrome. Live vendor tokens.

## What changed

- Live nav tab + page binding in `App.tsx` (system group, after Channels). Pending vs paired lists. Loading / empty (“No pending requests.” / “No devices.”) / error.
- Approve/deny pending; revoke paired. Named confirmation (id or display). Client `asPublic` / `publicHasSecrets` as a second gate.
- GET omits token/code/secret/content. Mutations 403 for view-token. `POST /api/nodes/request` is an exact-path auth bypass. Approve/deny/revoke append redacted eventstore rows (`nodes` / approve|deny|revoke|request).
- i18n vi+en. CP typecheck. Tests. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/node ./gateway/internal/httpapi ./gateway/internal/auth ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` 103/103 including `asPublic` dropping a `token` row, `publicHasSecrets` on token/code/content, `nodeConfirmMatch` for id/display, formatWhen fallback.
- `go test` node: request requires display; approve/deny/revoke require confirm; expired cannot approve (deny still works); tenant isolation; GET JSON omits token/code/secret; pending cap. httpapi: empty list + `/v1` alias; GET omits token/code/secret; confirm; 404 missing. auth/serve: view-token GET list 200, POST approve/deny/revoke 403; request exact POST path bypass.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Workstations (106). Channel DM pairing UI (089/094 stays on Channels). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 106-118.
