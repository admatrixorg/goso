# QA — SPEC 104 Contacts

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA. No invented live vendor tokens.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Contacts: search/filter toolbar; channel/type filters; selectable paginated table; refresh, filter, select, merge; inspect channel permission note and contact detail. | `docs/qa/090-goclaw-sidebar-ux.md` Contacts |

goso mapping (self-written): live tab `contacts` in [App.tsx](../../control-plane/src/App.tsx) renders [ContactsPage](../../control-plane/src/pages/ContactsPage.tsx). `FriendsPage` stays demo-only. Operator list is `GET /api/contacts` (`/v1/contacts` alias) with `contacts` (id, display, kind, channel, dest, identifiers[], count, first_seen, last_seen, permission, agent, can_undo, merged_from). Detail is `GET /api/contacts/{id}`. Merge is `POST /api/contacts/{id}/merge` with `source_id` + `confirm`; undo is `POST /api/contacts/{id}/undo` with `confirm`. GET never stores or returns tokens, pairing codes, or message text. Inbound (telegram ingest + `replyInbound`) records channel identities only.

Out of scope: Nodes (105). Workstations (106). Copying GoClaw chrome. Live vendor tokens.

## What changed

- Live nav tab + page binding in `App.tsx` (work group, after Pending). Search, channel/type filters, pagination, loading / empty (“No contacts.” / “Không có liên hệ.”) / error.
- Detail: canonical identity + channel ids + permission note + provenance (`merged_from`). Client `asPublic` / `publicHasSecrets` as a second gate.
- Merge requires a named confirmation (source id, target id, source dest, or `source>target`). Undo requires target id or last source id. Both keep identifiers and provenance. Destructive actions 403 for view-token; 403 `lite: channels off` when `GOSO_LITE` is on. Merge/undo append redacted eventstore rows (`contacts` / merge|undo).
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

- `npm run typecheck` exit 0. `npm test` 99/99 including `asPublic` dropping a `token` row, `publicHasSecrets` on token/code/content and nested identifiers, `mergeConfirmMatch` for source/target/dest/`source>target`, filter/page labels.
- `go test` channel: observe never keeps payloads; merge requires confirm and keeps both identifiers; undo restores source; tenant isolation; disabled Telegram webhook buffers dest `777` and contact listing omits `bot_token`. httpapi: empty list + `/v1` alias; GET omits token/code/secret/content/text; merge/undo confirm; lite 403; 404 missing. auth/serve: view-token GET list 200, POST merge/undo 403.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Nodes (105). Workstations (106). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge.
