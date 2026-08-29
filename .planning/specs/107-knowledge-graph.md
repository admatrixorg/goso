# SPEC 107 — Knowledge Graph explorer

> After 106. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 107. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Knowledge Graph. **THIẾU** as a dedicated page (Vault links + Memory expand stay).

## Goal

Add an **agent-scoped entity/relationship explorer** after Memory/Vault: agent/scope selection, graph **or** list (usable without canvas), search, provenance, bounded node counts, embedding/index health. Empty/not-configured states. Never imply inferred relationships are authoritative facts.

## AC

- [ ] Live nav tab + page. Agent/scope required empty state. List usable without canvas. Loading/empty/error/not-configured.
- [ ] Bounded node/edge counts. Provenance visible. Reuse existing KG APIs if present (`/api/kg/...`). GET never returns secrets.
- [ ] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/107-knowledge-graph.md`.

## Out of scope

Storage (108). Copying GoClaw canvas. Live vendor tokens. Paid embeddings.
