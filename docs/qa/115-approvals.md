# QA — SPEC 115 Approvals

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Approvals: realtime request list/empty state; refresh; approve or deny pending execution requests | `docs/qa/090-goclaw-sidebar-ux.md` Approvals |

goso mapping (self-written): live tab `approvals` in [App.tsx](../../control-plane/src/App.tsx) renders [ApprovalsPage](../../control-plane/src/pages/ApprovalsPage.tsx). Listing is `GET /api/approvals` (`/v1/approvals` alias) returning `{approvals,pending,generated_at}`. Rows are `{id,approval_id,kind,requester,agent_id,session_id,connector,tool,arg_preview,risk,status,expires_at,created_at,decided_at,decision,reason,stale}` — no `args`, `arguments`, `token`, or `secret`. `kind` is always `execution` (channel pairing stays on Channels / Nodes). `arg_preview` is a bounded redacted JSON snippet (secret keys and token shapes dropped). Approve is `POST /api/approvals/{id}/decision` `{decision:"approve"}`. Deny is the same path with `{decision:"deny"|"reject",reason}` — reason required. Second decision is 409; expired is 410. View-token GET 200 / POST 403. Issued `read` GET 200 / POST 403; `write` cannot decide; `approvals` can. Mutations append SPEC 110 audit rows (`entity=approval`, actions approve/deny) without args or secrets.

Out of scope: Import/Export (116). Backup (117). Copying GoClaw chrome. Live vendor tokens. SPECs 116–118.

## What changed

- Live nav tab + page. Inbox list/detail, loading / empty / error / stale (3s poll + stale banner).
- Approve/Deny once. Distinct from channel pairing. GET never returns secrets or raw args. Denial reason required. Immutable audit on resolution.
- i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/115-approvals.md`.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/approval ./gateway/internal/agent ./gateway/internal/auth ./gateway/internal/httpapi ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` including `asPublic` dropping `args`/`token`/`sk-` rows, `canResolve` refusing expired/stale, `approvalLabel`.
- `go test` approval gate: public JSON omits args; preview drops token/`text`/`content`; deny reason stored; second decide `ErrNotPending`; expired cannot decide; executor runs on approve only. httpapi: GET list/get omit secrets; deny without reason 400; second 409; expired GET 200 `expired` / POST 410; `/v1/approvals` alias; view-token GET 200 / POST 403; issued `read` GET 200 / POST 403, `write` cannot decide, `approvals` can. auth/serve: view GET `/api/approvals` 200, POST 403.
- Page copy: “No pending execution approvals.” / “Không có lệnh chờ duyệt.” Distinct-from-pairing note. Approve/Deny disabled on expired/stale. Deny requires a reason. Expand is text, no `dangerouslySetInnerHTML`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Import/Export (116). Backup (117). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 116-118. Channel pairing Approve/Deny (Channels / Nodes).
