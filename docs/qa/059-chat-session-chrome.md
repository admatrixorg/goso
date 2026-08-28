# QA — SPEC 059 Chat session chrome

Date: 2026-08-28. Clean-room React in `control-plane/`. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not break env `router9` + default `ocg/deepseek-v4-flash`.

Today `ChatPage` only received `sessionId`. Empty state was “chưa chọn phiên” with no New action. Header did not show the session label. Compact session list did not highlight the current id. SPEC 058 already wired Create on full + compact `SessionsPage` via `api.createSession`.

## Wired

- `App` keeps `sessionLabel` with `sessionId`. Pick/create passes `label || id` into `ChatPage`.
- Open-session `SectionHeader` title is the session label (fallback: id). Description still uses `chat.descSession` with the session id.
- Compact `SessionsPage` receives `selectedId`; the matching row uses accent-soft background, accent border, and `aria-current="true"`.
- Chat empty state: primary **New session** (`chat.newSession`). `onNew` focuses/scrolls to the existing 058 create form (`SessionsPage.focusCreate` → agent `<select>`). No second `POST /api/sessions` helper; Create still goes through `SessionsPage.create` → `api.createSession`.
- Enter-to-send still ignores IME `keyCode` 229. Send still uses `api.chatStream`. Send-fail still replaces the in-flight assistant bubble with a `local-err-` assistant bubble (046).
- i18n vi+en: `chat.newSession`.

## Not wired (no API / out of scope)

- DELETE session / rename — no HTTP; no new controls.
- New Go routes / SSE transport changes — none. Existing `POST/GET /api/sessions` and `api.chatStream` only.
- Model picker inside Chat — agent.model is 057.
- SPEC 060 command palette.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Non-goals

New chat transport, session DELETE, model picker in Chat, SPEC 060, binding/killing demo ports.

## Proof (tester 2026-08-28)

- `cd control-plane && npm run typecheck` → exit 0 (`tsc --noEmit`, no errors)
- `go test ./...` → exit 0 (24 ok, 3 no-test pkgs, 0 fail). Cached. No `*.go` / `gateway/*` in `git diff HEAD`.
- `GOSO_ROOT=$PWD …/agpl-check.sh` → exit 0 (`OK`; no banned author ids outside `.planning`)
- Diff: `control-plane/src/App.tsx`, `control-plane/src/pages/ChatPage.tsx`, `control-plane/src/pages/SessionsPage.tsx`, `control-plane/src/i18n/en.ts`, `control-plane/src/i18n/vi.ts`. Untracked: this QA file. No DEMO page diffs. No vite/dev server started. Ports `:8082` `:8091` `:3000` `:18080` `:18088` not bound/killed (pre-existing listeners left alone).
- Source (no live UI): open-session `SectionHeader` title = `sessionLabel?.trim() || sessionId`; desc still `chat.descSession` + id. Compact `SessionsPage` `selectedId={sessionId}` → accent-soft + accent border + `aria-current="true"`. Empty Chat primary **New session** `onNew` → `sessionsRef.current?.focusCreate()` only (scroll create box, focus agent `<select>`). `createSession` only in `SessionsPage.create` + `api/client.ts`. ChatPage has no `createSession` / no POST helper.
- i18n en+vi: `chat.newSession` present. Key sets equal (415/415). `en: Record<MsgKey, string>` typecheck covers match.
- Keep: `api.chatStream` in `ChatPage.send`. Enter ignores IME `isComposing` / `keyCode === 229`. Send-fail replaces in-flight asst with `localId("local-err")` bubble (`id.startsWith("local-err-")`).
- Env router9 + default `ocg/deepseek-v4-flash` untouched (`gateway/internal/llm/compat.go` not in diff).
- No control-plane unit tests (no CP test runner). Backend sessions/chat already in `gateway/internal/httpapi` (cached ok).
