# SPEC 109 — Realtime Events

> After 108. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 109. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Realtime Events. **PARTIAL** — EventsPage is a fetched connector-event list, not a live operator stream.

## Goal

Extend EventsPage with an **optional live stream** across agents/teams while retaining bounded historical connector events. Pause/resume/clear-local-view, event-type and actor filters, reconnect/backoff state, bounded retention, schema-safe detail expansion, and redaction of message/tool payload secrets.

## AC

- [x] Live Events page: optional live stream plus historical list. Pause/resume/clear-local-view. Filters. Loading/empty/error/reconnect.
- [x] Bounded retention. Schema-safe detail. GET/stream never returns message/tool payload secrets.
- [x] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/109-realtime-events.md`.

## Out of scope

Activity (110). Logs (111). Copying GoClaw chrome. Live vendor tokens. Tenants (112).
