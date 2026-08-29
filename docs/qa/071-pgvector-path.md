# QA — SPEC 071 PG16 / pgvector path (docs only)

Date: 2026-08-28. Clean-room. **No live Postgres host in 071.** DI-09 N1 is implemented in **SPEC 085** — see `docs/qa/085-postgres-local.md`. This file remains the 071 historical path; the driver lives there.

## GoClaw behavior (READ-ONLY cite — paths only)

| Behavior | Cite |
|----------|------|
| Hybrid FTS + vector memory; vector is a host concern | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/07-bootstrap-skills-memory.md` |
| SaaS isolation of agents/sessions/memory/providers on SQL `WHERE tenant_id` | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/23-multi-tenant-architecture.md` |

goso today ships **FTS5** on SQLite (`memory_fts`, `vault_fts`). Semantic / L2 vector search is not in this build.

## DSN (implemented in SPEC 085)

Local compose profile (host port **5433**, not 5432 / demo ports):

```
docker compose --profile postgres up -d postgres
GOSO_DATABASE_URL=postgres://goso:goso@127.0.0.1:5433/goso?sslmode=disable
```

- `store.Open` routes a postgres DSN to `OpenPostgres` (pgx via `database/sql`).
- Connect fail → error; **never** SQLite fallback.
- `OpenSQLite` still refuses a postgres DSN.
- Unset `GOSO_DATABASE_URL` keeps SQLite `GOSO_DB_PATH`.
- Default demo `:18080` stays SQLite. Details: `docs/qa/085-postgres-local.md`.

## pgvector for L2

Later, on PG16:

1. `CREATE EXTENSION IF NOT EXISTS vector;`
2. Store embeddings beside FTS (goso FTS5 stays the lexical path).
3. Scope every query with `tenant_id` (same column added on SQLite in SPEC 071).
4. Driver connect fail is fail-closed (no sqlite fallback). Missing `vector` extension is optional in 085 (log and continue lexical). Do not return unfiltered hits.

No code is copied from goclaw. No live host is required for SPEC 071.

## Non-goals

SPEC 071 did not implement the driver (085 does). Binding demo ports. Copying goclaw Go.
