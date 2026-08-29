# SPEC 100 — Webhooks operator surface

> After 099. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 100. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Hooks / Webhooks.

## Goal

Harden **WebhooksPage** as goso's outbound HTTP delivery surface: create/rotate/revoke, delivery status, replay/test, endpoint health, one-time secret reveal. Webhooks are not lifecycle hooks. Signing secrets stay write-only after creation; GET shows masked/status metadata only; redact payloads; audit rotate/replay.

## AC

- [x] List: status, endpoint, last delivery. Empty/loading/error. Create + one-time secret reveal (never returned later).
- [x] Rotate/revoke. GET never returns full signing secret. Replay/test without dumping secret-bearing payloads.
- [x] Clarify copy: these are HTTP webhooks, not lifecycle hooks.
- [x] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/100-webhooks.md`.

## Out of scope

Lifecycle hook interceptors (GoClaw Hooks). API Keys page (113). Copying GoClaw dialogs. Live vendor URLs with invented secrets.
