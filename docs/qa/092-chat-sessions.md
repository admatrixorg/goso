# QA — SPEC 092 Chat + Sessions operator workspace

Date: 2026-08-29. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Chat is a two-pane workspace (list + transcript/composer) with agent selection, create, open, delete, send, empty-start | `docs/qa/090-goclaw-sidebar-ux.md` Chat row |
| Sessions is a searchable list with agent/context and open/resume | `docs/qa/090-goclaw-sidebar-ux.md` Sessions row |

goso mapping (self-written): live tabs `sessions` and `chat` in [App.tsx](../../control-plane/src/App.tsx) still render [SessionsPage](../../control-plane/src/pages/SessionsPage.tsx) and [ChatPage](../../control-plane/src/pages/ChatPage.tsx). No third conversation protocol. Compact list remains the Chat left pane; full list remains the Sessions tab.

Out of scope: Pending Messages (103), Contacts (104), new SSE/chat transport.

## What changed

- Sessions list: search, agent filter/select, create, open/resume, delete. Destructive confirm names the session (`sessions.confirmDelete` with `{name}` = label or id).
- Chat: history, send, visible connecting/streaming/reconnect badges, empty and error states. Stream drop reloads history (does not re-POST). Send-fail bubble still uses redacted diagnostics.
- Selected session survives refresh via `localStorage` key `goso_selected_session` (`id` + `label` only). 404 / delete of the open session clears it.
- Transcript diagnostics/errors reuse `formatPublicError` / `redactPublicText` (tool `arguments` / `tool_input` / `tool_result` and credential-shaped keys).
- `DELETE /api/sessions/{id}` added on the existing session resource (tenant-scoped). Messages (and memories) for that id are removed. Not a new conversation protocol.
- i18n vi+en. CP typecheck. Tests for new helpers. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/store ./gateway/internal/httpapi -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` covers `filterSessions`, selected-session persist, `normalizePromptMode`, reconnect delay, `isGoneStatus`, and `redactPublicText` / `formatPublicError` (secrets + tool payloads).
- `go test` store + httpapi: `DeleteSession` memory/sqlite, `TestDeleteSession`, other-tenant DELETE 404 then owner 200.
- GET session list still has no secret fields. Delete confirm interpolates the session name. Chat stream badge `data-chat-stream`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Pending Messages. Contacts. New chat protocol. Usage charts. Binding/killing demo ports. Merge. Copying goclaw Go. Inventing live vendor tokens.
