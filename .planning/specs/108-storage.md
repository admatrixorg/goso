# SPEC 108 — Storage

> After 107. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 108. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Storage. **THIẾU**.

## Goal

Add a **permission-scoped workspace file browser** with path breadcrumbs, size/type/mtime metadata, upload/download, preview, and guarded delete. Enforce server-side path confinement, file-size/type limits, content handling, quota feedback, and prevent credential files or internal runtime paths from being exposed by default.

## AC

- [x] Live nav tab + page. Breadcrumbs, list metadata, loading/empty/error.
- [x] Upload/download/preview. Guarded delete with confirm. Path jail. Listing is metadata; preview is bounded. Never list internal runtime/secret paths by default. GET never returns credential values.
- [x] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/108-storage.md`.

## Out of scope

Realtime Events (109). Activity/Logs (110/111). Copying GoClaw chrome. Live vendor tokens. Packages (114).
