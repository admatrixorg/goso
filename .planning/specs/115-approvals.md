# SPEC 115 — Approvals

> After 114. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 115. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Approvals. **THIẾU**.

## Goal

Add a **realtime execution-approval inbox** distinct from channel pairing, showing requester, agent, tool, bounded argument preview, risk, expiry, and Approve/Deny. Enforce single-resolution semantics, stale/expired handling, secret redaction, reason capture for denial, and an immutable audit record.

## AC

- [ ] Live nav tab + page. Inbox list/detail, loading/empty/error/stale.
- [ ] Approve/Deny once. Distinct from channel pairing. GET never returns secrets. Audit resolution.
- [ ] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/115-approvals.md`.

## Out of scope

Import/Export (116). Backup (117). Copying GoClaw chrome. Live vendor tokens. Packages (114) already merged.
