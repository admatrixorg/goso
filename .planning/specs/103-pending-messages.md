# SPEC 103 — Pending Messages (channel buffer)

> After 102. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 103. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Pending Messages. **THIẾU** — no live tab.

## Goal

Add a **channel-buffer operations page**: explain buffering/compaction, list groups with counts/age/channel/agent, safe manual compact and clear. Preview/confirmation for data loss. Live refresh. Permission-checked. Compact-in-progress/error. No raw secret-bearing payloads in listings.

## AC

- [ ] Live nav tab + page binding in `App.tsx` (not demo-only). Loading/empty (“no pending”) / error / in-progress.
- [ ] List groups: count, age, channel, agent (or explicit n/a). Refresh. No token/code fields on GET.
- [ ] Compact and clear require named/preview confirmation. Destructive actions audit-friendly. 403 if view-token / lite if applicable.
- [ ] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/103-pending-messages.md`.

## Out of scope

Contacts (104). Nodes/Workstations (105/106). Copying GoClaw chrome. Live vendor tokens.
