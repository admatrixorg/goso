# SPEC 114 — Packages

> After 113. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 114. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Packages. **THIẾU**.

## Goal

Add **runtime/package administration** only behind elevated permissions, with system/Python/Node/GitHub inventory, compatibility warnings, install/uninstall progress, and failure logs. Separate package management from CLI credential management, require allowlists/pinning and explicit confirmation, and model partial installation/recovery states.

## AC

- [ ] Live nav tab + page. Inventory, loading/empty/error/progress.
- [ ] Install/uninstall with confirm. Allowlists/pinning. Failure logs. GET never returns credentials. Elevated permission.
- [ ] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/114-packages.md`.

## Out of scope

Approvals (115). Import/Export (116). Copying GoClaw chrome. Live vendor tokens. API Keys (113) already merged.
