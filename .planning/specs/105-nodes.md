# SPEC 105 — Nodes / devices

> After 104. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 105. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Nodes. **THIẾU** as a dedicated page (channel pairing stays on Channels).

## Goal

Add a **device/node page** that separates pending pairing from paired devices: approve, deny, revoke, refresh, expiry, last-seen, health. Pairing codes and credentials stay transient/non-returned. Every approval or revocation is permission-checked and audit-logged.

## AC

- [ ] Live nav tab + page. Pending vs paired lists. Loading/empty/error.
- [ ] Approve/deny pending; revoke paired. GET never returns pairing codes or tokens. 403 view-token on mutations.
- [ ] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/105-nodes.md`.

## Out of scope

Workstations (106). Channel DM pairing UI (089/094 stays on Channels). Copying GoClaw chrome. Live vendor tokens.
