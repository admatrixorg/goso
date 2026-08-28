# SPEC 071 — Tenant-lite (SQLite) + PG16/pgvector path

> LOCKED after 070. Clean-room. **Do not wait** for DI-09 host lock. Do not kill `:8082` `:8091`.

Closes **CTO-06** as far as SQLite allows. Documents PG path; does not require a live Postgres.

## GoClaw (cite)

- Personal single-tenant vs SaaS isolation of agents/sessions/memory/providers (`docs/23-multi-tenant-architecture.md`).
- Default “master” tenant when no tenant config (same doc Mode 1).
- L2/vector is a **host** concern (`docs/07-bootstrap-skills-memory.md` hybrid FTS+vector) — goso documents pgvector, ships FTS5.

## goso today

- One SQLite file, no `tenant_id` (N1 THIẾU, N8 PARTIAL).
- AES-256-GCM secrets exist (N4 PARTIAL).
- `GOSO_VIEW_TOKEN` GET-only, not RBAC matrix (N5).

## goso plan

1. Add `tenant_id TEXT NOT NULL DEFAULT 'default'` to agents, sessions, memories, vault_docs, teams, webhooks, llm_providers (ALTER + backfill). Store/list/get filter by request tenant.
2. Request tenant: header `X-Goso-Tenant` if `GOSO_MULTI_TENANT=1`, else always `default`. Admin token required to set a non-default tenant. Invalid/empty → default. **Demo stays single-tenant.**
3. Tests: two tenants, agent in A not visible to B; webhook/job/session isolation.
4. **PG16 path (docs only):** `docs/qa/071-pgvector-path.md` — how to point `GOSO_DATABASE_URL=postgres://...` later, pgvector extension for L2, no code copy, no live host. Implementation stub: if DSN is postgres, **fail closed** with a clear log/error unless a thin driver exists; do not half-ship a broken PG open. Prefer: SQLite now + documented interface `store.StoreIface` already used.
5. RBAC: keep view-token GET-only; full matrix is 072+ if still PARTIAL. Optional: tenant header ignored for view-token.

## UI

Settings line: current tenant `default` (read-only in demo). If `GOSO_MULTI_TENANT=1`, a select/input stored in localStorage for the header. i18n. Do not invent tenant SSO.

## Tests

- Isolation test above.
- Default path unchanged for existing tests (no header).
- agpl 0.

QC: typecheck, go test, build, agpl 0, `docs/qa/071-tenant-lite.md`. Commit `admatrixmdp/spec071-tenant-lite`. Do not merge.
