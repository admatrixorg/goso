# SPEC 099 — Memory operator surface

> After 098. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 099. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Memory.

## Goal

Unify **MemoryPage** around document and episodic workflows: agent/scope filters, search, detail, create/edit/delete where allowed, relation expansion, embedding/index status, and actionable “not configured” guidance. Provenance, scope, and destructive impact must be visible so operators cannot confuse session notes with durable agent memory.

## AC

- [x] Filters: agent/scope, search, loading/empty/error. Separate episodic vs durable documents if both exist.
- [x] Detail + relation expand. Embedding/index “not configured” guidance when missing (do not invent a paid embedder).
- [x] Create/edit/delete where APIs allow; destructive confirm names the target. No secret payloads in lists.
- [x] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/099-memory.md`.

## Out of scope

Knowledge Graph page (107). Vault (098). Copying GoClaw tabs. Live vendor tokens / paid embeddings.
