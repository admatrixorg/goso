# SPEC 095 — Providers operator surface

> After 094. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 095. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Providers.

## Goal

Complete **ProvidersPage** for key-based and subscription providers: add/edit/enable/test/connect, model discovery, latency/error feedback, operator-visible source/state. API keys stay write-only (`key_set` / source only on GET). Never hydrate secret inputs. Test errors must not echo credentials or `Authorization` headers.

## AC

- [ ] List: search/filter, type, source (`env`/`sqlite`), `key_set` badge, optional enabled state. Empty/loading/error. Env-owned rows stay read-only (existing `env overlay`).
- [ ] Add/edit: name (create only), type, base_url, model, write-only `api_key` (blank on load, empty PATCH does not clear). Test models/chat with `latency_ms` and redacted error. Do not dump raw JSON that could contain keys.
- [ ] Explicit rotate/clear for sqlite-boxed keys (empty PATCH already keeps the key — add DELETE `/api/providers/{name}/key` or equivalent). GET `/api/providers` never includes `api_key`. Env-wins after clear.
- [ ] i18n vi+en. CP typecheck. Tests for helpers + httpapi never-leak. agpl 0. `docs/qa/095-providers.md`.

## Out of scope

Live paid vendor keys. Packages/API Keys pages (113/114). Copying GoClaw dialogs. Subscription marketplace UI.
