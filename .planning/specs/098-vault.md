# SPEC 098 — Vault operator surface

> After 097. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 098. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Vault.

## Goal

Extend **VaultPage** from document/link lists to an operator knowledge workspace: type/agent/team filters, search, document detail, inbound/outbound links, sync health, optional bounded graph. Large-vault limits, keyboard/a11y, stale-index warnings, safe rendering of untrusted document content.

## AC

- [x] List: search + type/agent/team filters, loading/empty/error, sync health/stale warning if APIs exist.
- [x] Detail: document metadata, inbound/outbound links. Untrusted body is escaped/plain (no raw HTML inject).
- [x] Optional bounded graph (or explicit “no canvas” usable list). Cap node counts.
- [x] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/098-vault.md`.

## Out of scope

Knowledge Graph page (107). Storage page (108). Copying GoClaw canvas. Live vendor tokens.
