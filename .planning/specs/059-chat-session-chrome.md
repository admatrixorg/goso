# SPEC 059 — Chat session chrome

> LOCKED: 2026-08-28. Clean-room React. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`. Keep SSE (`api.chatStream`).

`ChatPage` only receives `sessionId`. Empty state is “chưa chọn phiên” with no New action. Header does not show the session label. Compact session list does not highlight the current id.

## UI

- Pass `sessionLabel` (or look up from sessions list) into `ChatPage` / `SectionHeader` so the open session is named.
- Highlight the selected row in compact `SessionsPage`.
- Empty state: primary **New session** — if 058 Create is already in the sidebar, empty state should focus/scroll to it **or** accept `onNew` from App. Do not duplicate a second POST helper; reuse 058.
- Keep Enter-to-send (ignore IME 229). Keep send-fail assistant bubble (046).
- i18n vi+en.

`docs/qa/059-chat-session-chrome.md`. Commit `admatrixmdp/spec059-chat-chrome`. Do not merge.

## QC

`cd control-plane && npm run typecheck` · `go test ./...` · agpl-check 0.

## Non-goals

New chat transport, DELETE session, model picker inside Chat (agent.model is 057).
