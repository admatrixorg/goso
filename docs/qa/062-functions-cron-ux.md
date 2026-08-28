# QA — SPEC 062 Functions / cron form UX

Date: 2026-08-28. Clean-room React in `control-plane/`. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not break env `router9` + default `ocg/deepseek-v4-flash`.

Today Functions already had tools + connector patch + cron card (`cronApi`, SPEC 054). `createCron()` returned silently when spec/session/message were empty. Delete had no confirm. Empty session list did not point at the Sessions tab (058). Connector save with empty name was a silent return.

## Wired

- Create job: trim spec / session / message. Empty spec → `StatusLine` `functions.cron.needSpec`. Empty session → `functions.cron.needSession`. Empty message → `functions.cron.needMessage`. No silent return. No POST.
- If `sessions.length === 0` after a successful sessions load: empty copy `functions.cron.noSessions` (Sessions tab). Create uses the same copy and does not POST.
- Delete: `window.confirm` with `functions.cron.confirmDelete`, then `DELETE /api/cron/{id}`. Cancel does not DELETE.
- Job row shows `enabled` as a read-only badge when the JSON includes a boolean (`common.enabled` / `common.disabled`). No PATCH enable, no toggle control.
- Connector Save is always on the card. Empty connector name → `StatusLine` `functions.needConnector`. No PATCH. Endpoint/token fields stay gated on a picked connector.
- Cron list and sessions load independently (`Promise.allSettled`). A failed `GET /api/sessions` does not blank existing jobs. Empty-session copy only after a successful load with zero sessions.
- i18n vi+en.

## Not wired (no API / out of scope)

- Cron PATCH / enable / run-now — 054 non-goal; no enable toggle that would POST.
- MCP cron rewrite, OS crontab.
- New Go routes — none. Existing `GET/POST /api/cron` and `DELETE /api/cron/{id}` only. Connector still `PATCH` via `toolsApi.patchConnector`.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Non-goals

Cron PATCH/run-now, MCP cron rewrite, OS crontab, SPEC 063, binding/killing demo ports.

## Proof (2026-08-28)

- `cd control-plane && npm run typecheck` → exit 0 (`tsc --noEmit`, no errors)
- `go test ./...` → exit 0 (24 ok, 3 no-test pkgs, 0 fail). Cached. No `*.go` / `gateway/*` in `git diff HEAD`.
- `GOSO_ROOT=$PWD …/agpl-check.sh` → exit 0 (`OK`; no banned author ids outside `.planning`)
- Diff: `control-plane/src/pages/FunctionsPage.tsx`, `control-plane/src/api/cron.ts` (`enabled?` display-only), `control-plane/src/i18n/en.ts`, `control-plane/src/i18n/vi.ts`, this QA file. No DEMO page diffs. No vite/dev server. Ports `:8082` `:8091` `:3000` `:18080` `:18088` not bound/killed.
- Source (no live UI): `createCron()` empty spec/session/message → `StatusLine` `functions.cron.need*` then return before `cronApi.create`. Empty sessions after successful load → `EmptyState` + create `functions.cron.noSessions`, no POST. `deleteCron` `window.confirm(functions.cron.confirmDelete)` then `cronApi.remove`; cancel returns. Connector Save empty name → `functions.needConnector`, no `patchConnector`. Job `enabled` is a Badge, not a control; `cronApi` is still list/create/remove only.
- i18n en+vi keys present and matched (426/426): `functions.needConnector`, `functions.cron.needSpec`, `functions.cron.needSession`, `functions.cron.needMessage`, `functions.cron.noSessions`, `functions.cron.confirmDelete`, `functions.cron.col.enabled`.
- Env router9 + default `ocg/deepseek-v4-flash` untouched (no gateway/llm files in diff).
- No control-plane unit tests (no CP test runner). Backend cron already covered in `gateway/internal/httpapi/handlers_cron_test.go`.
