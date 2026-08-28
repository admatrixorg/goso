# CTO follow-up queue — SPEC 066–071

> LOCKED: 2026-08-28 by Dat (revised numbering). Sequential Grok workers. Clean-room. Do not kill `:8082` `:8091`.
> 066 merged as `98daf68`. 069–071 numbering supersedes the first 42ded0e table.

| SPEC | Title | Audit IDs | Status |
|------|-------|-----------|--------|
| **066** | Runtime provider routing + SSRF DNS + prod security + compose | CTO-01, 02, 03, 07 | **merged** `98daf68` |
| **067** | Durable webhooks (persist, freshness, replay, async status/callback) | CTO-04 | next |
| **068** | Real chat SSE (provider deltas, not post-split) | CTO-05 | |
| **069** | Health chrome + polish + CTO-09 author-id/agpl docs scan | CTO-08, 11, 09 | (was tenant-lite on 42ded0e) |
| **070** | Backup/restore drill + TLS/alert notes | CTO-10 | |
| **071** | Tenant-lite on SQLite + documented PG16/pgvector path | CTO-06 / DI-09 | Do not wait for host lock |

After 071: `.planning/specs/072-plus-queue.md`. Stop only when remaining rows are DI parked.
