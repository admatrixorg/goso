# QA — SPEC 058 Create session from UI

Date: 2026-08-28. Clean-room React in `control-plane/`. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not break env `router9` + default `ocg/deepseek-v4-flash`.

Today `POST /api/sessions` `{agent_id, label?}` and client `api.createSession` already existed. `SessionsPage` (full + compact Chat sidebar) was list-only, so a user could not start a new chat without curling.

## Wired

- Full `SessionsPage`: agent `<select>` from `GET /api/agents`, optional label, Create → `POST /api/sessions` `{agent_id, label?}` → `onPick(id)` (opens Chat as today).
- Compact Chat sidebar: same Create in a stacked layout. After create, `onPick(id)` selects that session.
- Empty agents → copy `sessions.noAgents` pointing at the Agents tab. Create does not POST.
- Empty `agent_id` → `StatusLine` i18n `sessions.needAgent` (human copy, not raw `agent_id`). No silent return. No POST.
- Optional label omitted from the body when blank.
- StatusLine loading/error on list + create. i18n vi+en.
- Session list and agent select load independently (`Promise.allSettled`). A failed `GET /api/agents` does not blank existing sessions. Empty-agent copy only after a successful load with zero agents.

## Not wired (no API / out of scope)

- DELETE session — no HTTP; no delete control.
- Rename / PATCH label — no HTTP; no rename control.
- New Go routes — none. Existing `POST/GET /api/sessions` and `GET /api/agents` only.
- Chat session chrome (header title) — SPEC 059.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Non-goals

Session DELETE, label PATCH, new Go endpoints, SSE changes, SPEC 059 chat chrome, binding/killing demo ports.

## Proof (tester 2026-08-28)

- `cd control-plane && npm run typecheck` → exit 0 (re-run after review: list vs agents `allSettled`, empty-agent copy only when load succeeded with zero agents)
- `go test ./...` → exit 0 (24 ok, 3 no-test pkgs, 0 fail). Cached. No `*.go` / `gateway/*` in `git diff HEAD`.
- `GOSO_ROOT=$PWD …/agpl-check.sh` → exit 0
- Diff: `control-plane/src/pages/SessionsPage.tsx`, `control-plane/src/i18n/en.ts`, `control-plane/src/i18n/vi.ts`, `docs/qa/058-session-create.md`. No DEMO page diffs. No vite/dev server. Ports `:8082` `:8091` `:3000` `:18080` `:18088` not bound/killed.
- Source (no live UI): shared `createFields()` on full + compact — agent `<select>`, optional label, Create → `api.createSession` then `onPick(created.id)`.
- Empty `agent_id` (incl. whitespace) → `StatusLine` `sessions.needAgent`, return before POST. Empty agents → `EmptyState` `sessions.noAgents` (Agents tab copy), `create()` same copy, no POST. Blank label omitted from body.
- i18n en+vi keys present and matched: `sessions.create`, `sessions.needAgent`, `sessions.noAgents`, `sessions.label`, `sessions.pickAgent`, `sessions.add`, `sessions.placeholder.label`.
- No DELETE/rename UI in `SessionsPage`. No new Go routes.
- No control-plane unit tests (no CP test runner). Backend `POST /api/sessions` already covered in `gateway/internal/httpapi/handlers_test.go`.
