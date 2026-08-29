# SPEC 110 — Activity (audit trail)

> After 109. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 110. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Activity. **THIẾU**.

## Goal

Add an **immutable administrative audit trail** for configuration and privileged actions with action/actor/entity/IP/time filters, stable pagination, detail metadata, and export only for authorized roles. Keep operational events (109) separate from audit records. Redact secrets while preserving enough before/after metadata for accountability.

## AC

- [x] Live nav tab + page. Filters, pagination, loading/empty/error.
- [x] Immutable records. Operational events stay on Events. GET never returns secrets. Export only for authorized roles if implemented.
- [x] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/110-activity.md`.

## Out of scope

Logs (111). Tenants (112). Copying GoClaw chrome. Live vendor tokens. Realtime Events (109) already merged.
