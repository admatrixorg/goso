# QA — SPEC 071 PG16 / pgvector path (docs only)

Date: 2026-08-28. Clean-room. **No live Postgres host.** DI-09 stays parked. This file is the documented path for a later driver — not an implementation.

## GoClaw behavior (READ-ONLY cite — paths only)

| Behavior | Cite |
|----------|------|
| Hybrid FTS + vector memory; vector is a host concern | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/07-bootstrap-skills-memory.md` |
| SaaS isolation of agents/sessions/memory/providers on SQL `WHERE tenant_id` | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/23-multi-tenant-architecture.md` |

goso today ships **FTS5** on SQLite (`memory_fts`, `vault_fts`). Semantic / L2 vector search is not in this build.

## DSN later

When a thin Postgres driver exists (not now):

```
GOSO_DATABASE_URL=postgres://USER:PASS@HOST:5432/goso?sslmode=require
```

Until then:

- `store.Open` / `OpenSQLite` **fail closed** if `GOSO_DATABASE_URL` or `GOSO_DB_PATH` looks like `postgres://` / `postgresql://`.
- Error: `postgres is not supported in this build: SQLite only (see docs/qa/071-pgvector-path.md)`.
- Do **not** half-open a broken PG driver or silently fall back to SQLite when a postgres DSN is set.
- `store.StoreIface` remains the SQLite (and in-memory) implementation.

## pgvector for L2

Later, on PG16:

1. `CREATE EXTENSION IF NOT EXISTS vector;`
2. Store embeddings beside FTS (goso FTS5 stays the lexical path).
3. Scope every query with `tenant_id` (same column added on SQLite in SPEC 071).
4. Fail closed if the DSN is postgres but the driver/extension is missing — never return unfiltered or half-indexed hits.

No code is copied from goclaw. No live host is required for SPEC 071.

## Non-goals

Implementing the PG driver. Binding a Postgres port. Waiting on DI-09. Copying goclaw Go.
