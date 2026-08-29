# QA — SPEC 103 Pending Messages

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA. No invented live vendor tokens.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Pending Messages: explanatory panel plus buffered-group list/empty state; refresh; compact or clear a buffer; live empty “No pending messages.” | `docs/qa/090-goclaw-sidebar-ux.md` Pending Messages |

goso mapping (self-written): live tab `pending` in [App.tsx](../../control-plane/src/App.tsx) renders [PendingPage](../../control-plane/src/pages/PendingPage.tsx). Operator list is `GET /api/pending-messages` (`/v1/pending-messages` alias) with `groups` (id, channel, dest, agent_id, agent, count, oldest_at, newest_at, age_ms, compacted, compacting). Compact is `POST /api/pending-messages/{id}/compact` and clear is `POST /api/pending-messages/{id}/clear`; both require a `confirm` string matching id, dest, or `channel:dest`. GET never stores or returns message text, tokens, pairing codes, or secrets. Inbound from a disabled agent (or a dest that already has a buffer) enqueues counts only.

Out of scope: Contacts (104). Nodes/Workstations (105/106). Copying GoClaw chrome. Live vendor tokens.

## What changed

- Live nav tab + page binding in `App.tsx` (work group, after Chat). Loading / empty (“No pending messages.” / “Không có tin chờ.”) / error / compact-in-progress.
- List groups: count, age, channel, agent (or n/a). Refresh. Client `asPublic` / `publicHasSecrets` as a second gate.
- Compact and clear use a named/preview confirmation panel. Server rejects missing or mismatched `confirm` (400). Destructive actions 403 for view-token; 403 `lite: channels off` when `GOSO_LITE` is on. Compact/clear append redacted eventstore rows (`pending-messages` / compact|clear).
- i18n vi+en. CP typecheck. Tests. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/channel ./gateway/internal/httpapi ./gateway/internal/auth ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` 95/95 including `asPublic` dropping a `token` row, `publicHasSecrets` on token/code/content, `pendingConfirmMatch` for id/dest/`channel:dest`, age/preview labels.
- `go test` channel: enqueue never keeps payloads; compact requires confirm; clear/busy; tenant isolation; disabled Telegram webhook buffers dest `777` and GET listing omits `bot_token`. httpapi: empty list + `/v1` alias; GET omits token/code/secret/content/text; compact/clear confirm; lite 403; 404 missing and 409 compact in progress. auth/serve: view-token GET list 200, POST compact/clear 403.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Contacts (104). Nodes (105). Workstations (106). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge.
