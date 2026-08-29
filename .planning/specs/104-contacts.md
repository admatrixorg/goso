# SPEC 104 — Contacts directory

> After 103. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 104. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Contacts. **THIẾU** — FriendsPage is demo-only, not live.

## Goal

Add a **live contact directory** sourced from channel interactions: search, channel/type filters, canonical identity detail, permission visibility, selection, guarded merge. Deterministic merge provenance and undo/audit so operators can consolidate duplicates without losing channel identifiers or consent context.

## AC

- [x] Live nav tab + page (not demo-only Friends). Search/filter, loading/empty/error.
- [x] Detail: canonical identity + channel ids. GET never returns tokens. Merge requires confirmation and keeps identifiers.
- [x] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/104-contacts.md`.

## Out of scope

Nodes (105). Workstations (106). Copying GoClaw chrome. Live vendor tokens.
