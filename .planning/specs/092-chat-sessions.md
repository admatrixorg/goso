# SPEC 092 — Chat + Sessions operator workspace

> After 091. Clean-room React. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 092. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Chat + Sessions.

## Goal

Finish the existing **ChatPage + SessionsPage** split: searchable session list, agent selection, create/resume/delete with confirmation, message history, prompt/context mode, streaming/reconnect, empty/error. Preserve selected session across refresh. Redact tool/credential material from transcript diagnostics.

## AC

- [ ] Sessions list: search, agent, create, open/resume, delete with named confirmation.
- [ ] Chat: history, send, streaming/reconnect visible, empty and error states.
- [ ] Selected session survives refresh (localStorage or URL). Destructive target unambiguous.
- [ ] No secret/tool payloads echoed in error UI. i18n vi+en. CP typecheck. Tests for new helpers. agpl 0. `docs/qa/092-chat-sessions.md`.

## Out of scope

New conversation protocol. Pending Messages page (103). Contacts (104).
