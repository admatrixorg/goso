# SPEC 111 — Logs

> After 110. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 111. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Logs. **THIẾU**.

## Goal

Add a **redacted, permission-gated live log tail** with component, text, severity, pause/resume, clear-local-view, reconnect, and bounded retention. Server-side redaction and query limits are mandatory; the client must never receive credentials merely to hide them visually.

## AC

- [ ] Live nav tab + page. Component/text/severity filters. Pause/resume/clear-local-view. Loading/empty/error/reconnect.
- [ ] Bounded retention. Server-side redaction. GET/stream never returns credentials.
- [ ] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/111-logs.md`.

## Out of scope

Tenants (112). API Keys (113). Copying GoClaw chrome. Live vendor tokens. Activity (110) already merged.
