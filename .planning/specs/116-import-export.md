# SPEC 116 — Import & Export

> After 115. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 116. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Import & Export. **THIẾU**.

## Goal

Add staged export/import for selected teams, agents, skills, and MCP metadata with manifest preview, schema/version validation, conflict strategy, dry run, progress, and rollback/reporting. Exclude secrets by default and require operators to re-enter credentials after import; never smuggle tokens through portable archives.

## AC

- [ ] Live nav tab + page. Staged export/import, loading/empty/error/progress.
- [ ] Manifest preview, schema/version validation, conflict strategy, dry run, rollback/reporting. Secrets excluded by default; re-enter credentials after import. GET/archive never returns tokens.
- [ ] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/116-import-export.md`.

## Out of scope

Backup (117). TTS (118). Copying GoClaw chrome. Live vendor tokens. Approvals (115) already merged.
