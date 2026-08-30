# SPEC 117 — Backup & Restore

> After 116. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 117. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Backup & Restore. **PARTIAL**.

## Goal

Extend the existing Settings backup flow with system preflight, database/tool compatibility checks, system/tenant scope, archive validation, restore planning, progress, and failure recovery. Add optional S3-compatible storage using write-only access/secret inputs and `configured` metadata, require destructive restore confirmation, and never include credentials in backup archives.

## AC

- [ ] Live Settings backup (or dedicated tab). Preflight, progress, loading/empty/error.
- [ ] System/tenant scope, archive validation, restore planning, failure recovery. Optional S3 write-only + `configured` on GET. Destructive restore confirm. Archives never include credentials.
- [ ] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/117-backup-restore.md`.

## Out of scope

TTS (118). Copying GoClaw chrome. Live vendor tokens. Import/Export (116) already merged.
