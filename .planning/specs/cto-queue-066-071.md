# CTO follow-up queue — SPEC 066–071

> LOCKED: 2026-08-28 after `docs/qa/audit-cto-2026-08-28.md`. Sequential. Clean-room. Do not kill `:8082` `:8091`.

| SPEC | Title | Audit IDs |
|------|-------|-----------|
| **066** | Runtime provider routing + SSRF DNS + prod security + compose | CTO-01, 02, 03, 07 |
| **067** | Durable webhooks (persist, freshness, async status) | CTO-04 |
| **068** | Real chat SSE (provider deltas, not post-split) | CTO-05 |
| **069** | Tenant-lite on SQLite + documented PG16/pgvector path | CTO-06 / DI-09 |
| **070** | Backup/restore drill + TLS/alert notes | CTO-10 |
| **071** | Strip banned author ids from `docs/qa` + repo-local agpl-check | CTO-09 |

066 first. One Grok worker at a time.
