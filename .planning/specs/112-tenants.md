# SPEC 112 — Tenants

> After 111. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 112. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Tenants. **THIẾU**.

## Goal

Add **master-admin tenant lifecycle and user-access management** with searchable list/detail, create, status changes, membership/role visibility, and guarded deactivation. Enforce tenant isolation at the API layer, show current/master context clearly, and audit every access or status mutation.

## AC

- [x] Live nav tab + page. Searchable list/detail, loading/empty/error.
- [x] Create/status/membership visibility. Guarded deactivation with confirm. API tenant isolation. GET never returns secrets. Audit mutations.
- [x] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/112-tenants.md`.

## Out of scope

API Keys (113). Packages (114). Copying GoClaw chrome. Live vendor tokens. Logs (111) already merged.
