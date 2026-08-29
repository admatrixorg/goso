# QA — SPEC 106 Workstations

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA. No invented live vendor tokens. Unit tests do not SSH to untrusted hosts.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Workstations: list/empty plus Add dialog; refresh, create, edit/test/remove; dialog asks display, backend, host, port, SSH user, optional identity-file **path** (not private-key material). | `docs/qa/090-goclaw-sidebar-ux.md` Workstations |

goso mapping (self-written): live tab `workstations` in [App.tsx](../../control-plane/src/App.tsx) renders [WorkstationsPage](../../control-plane/src/pages/WorkstationsPage.tsx). Operator list is `GET /api/workstations` (`/v1/workstations` alias) with `workstations` (id, display, backend, host, port, user, identity_ref path, identity_set, agent_id, health, last_tested). Detail is `GET /api/workstations/{id}`. Create is `POST /api/workstations`; edit is `PATCH /api/workstations/{id}`. Test is `POST /api/workstations/{id}/test` (local config validation, constrained output, no SSH). Disconnect/delete are `POST /api/workstations/{id}/disconnect|delete` with `confirm` matching id or display. GET never returns private keys; identity is a path/ref only. Bodies with `private_key`/`key`/`pem` are rejected.

Out of scope: Knowledge Graph (107). Storage (108). Copying GoClaw chrome. Live vendor tokens. Actual SSH to untrusted hosts in unit tests.

## What changed

- Live nav tab + page binding in `App.tsx` (system group, after Nodes). List/detail, create/edit form, loading / empty (“No workstations.” / “Không có máy chạy.”) / error.
- Create/edit/test. Identity field is a path/ref; PEM/private-key material is rejected. Client `asPublic` / `publicHasSecrets` / `looksLikeKey` as a second gate. GET returns `identity_ref` path + `identity_set`, never keys. Test output omits `identity_ref`.
- Disconnect/delete require named confirmation (id or display). Mutations 403 for view-token. Append redacted eventstore rows (`workstations` / create|update|test|disconnect|delete).
- i18n vi+en. CP typecheck. Tests. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/workstation ./gateway/internal/httpapi ./gateway/internal/auth ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` 108/108 including `asPublic` dropping a `private_key` row and PEM `identity_ref`, `publicHasSecrets` on keys, `looksLikeKey` accepting `~/.ssh/id_ed25519`, `wsConfirmMatch` for id/display, `asPublicTest` dropping leaked test payloads.
- `go test` workstation: display/host/backend/user validation; PEM identity rejected; docker default port 2375; test JSON omits `identity_ref`/keys and never dials; disconnect/delete require confirm; tenant isolation; GET JSON omits private_key; cap. httpapi: empty list + `/v1` alias; POST `private_key` 400; GET omits keys; test body has no path; confirm; 404 missing. auth/serve: view-token GET list 200, POST create/test/delete 403.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Knowledge Graph (107). Storage (108). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 107-118. SSH to untrusted hosts in unit tests.
