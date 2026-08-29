# SPEC 106 — Workstations

> After 105. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 106. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Workstations. **THIẾU**.

## Goal

Add **remote execution target** administration for SSH/Docker: list/detail, create/edit/test, agent visibility, health, removal. Store only references to approved identity material, never display private keys. Validate host/port/user/backend. Constrain test output. Explicit confirmation for disconnect/delete.

## AC

- [x] Live nav tab + page. List/detail, loading/empty/error.
- [x] Create/edit/test. Identity is a path/ref, never a private key. GET never returns keys. Delete/disconnect requires confirmation.
- [x] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/106-workstations.md`.

## Out of scope

Knowledge Graph (107). Storage (108). Copying GoClaw chrome. Live vendor tokens. Actual SSH to untrusted hosts in unit tests.
