# SPEC 093 — Agents operator lifecycle

> After 092. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 093. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Agents.

## Goal

Evolve **AgentsPage** from basic list/form into operator lifecycle: create, edit, enable/disable, provider/model assignment, context/instruction modes, transfer, validation, safe deletion. List filters and status health. Do **not** dump full system prompts in the list. Specify concurrency/conflict for edits.

## AC

- [ ] List: search/filter, status (active/inactive), provider/model, no full prompt in list.
- [ ] Create/edit: identity, provider, model, instructions/context mode; validation errors; enable/disable.
- [ ] Delete/transfer: named confirmation; 409 on conflict if concurrent edit detected (or last-write with explicit error).
- [ ] i18n vi+en. Loading/empty/error. CP typecheck. Tests for helpers. agpl 0. `docs/qa/093-agents-lifecycle.md`.

## Out of scope

Agent Link & Team (097). Full prompt dump in list. Copying GoClaw dialogs.
