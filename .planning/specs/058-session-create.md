# SPEC 058 — Create session from UI

> LOCKED: 2026-08-28. Clean-room React. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.
> **Do not break** router9 + deepseek chat.

`POST /api/sessions` `{agent_id, label?}` exists (`handleCreateSession`). Client `api.createSession` exists. `SessionsPage` is **list-only** (full + compact). User cannot start a new chat without curling.

## UI

- Full `SessionsPage`: agent `<select>` from `GET /api/agents`, optional label, Create button → `POST /api/sessions` → `onPick(id)` (opens Chat as today).
- Compact Chat sidebar: same Create (compact layout). After create, select that session.
- Empty agents → StatusLine / empty copy telling user to create an agent (Agents tab), do not POST.
- Empty `agent_id` → i18n error, no silent return.
- i18n vi+en. StatusLine loading/error on list+create.
- **No DELETE / rename** (no HTTP).

`docs/qa/058-session-create.md`. Commit `admatrixmdp/spec058-session-create`. Do not merge.

## QC

`cd control-plane && npm run typecheck` · `go test ./...` · agpl-check 0.

## Non-goals

Session DELETE, label PATCH, new Go routes, SSE changes (053 already live).
