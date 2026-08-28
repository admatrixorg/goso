# QA — SPEC 071 Tenant-lite (SQLite)

Date: 2026-08-28. Clean-room Go/React. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not wait for a pgvector/Postgres host (DI-09 parked). Do not merge. Do not start SPEC 072.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Personal/single-tenant default vs SaaS isolation of agents/sessions/memory/providers | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/23-multi-tenant-architecture.md` (Mode 1 default “master” tenant when no tenant config; Mode 2 isolation) |
| L2/vector is a **host** concern (hybrid FTS + vector) | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/07-bootstrap-skills-memory.md` |

goso mapping (self-written): SQLite `tenant_id TEXT NOT NULL DEFAULT 'default'` on agents, sessions, memories, vault_docs, teams, webhooks, llm_providers (ALTER + backfill). List/get filter by request tenant. Header `X-Goso-Tenant` only when `GOSO_MULTI_TENANT=1`; else always `default`. Admin token required for a non-default tenant. Empty/invalid → `default`. Demo (unset / control-plane demo) stays single-tenant. View-token remains GET-only and cannot switch tenants. `GOSO_VIEW_TOKEN` is not a RBAC matrix (077). Postgres DSN fail-closed — see `docs/qa/071-pgvector-path.md`.

## What changed

- Store: `tenant_id` column + default stamp on create; existing rows backfilled to `default`.
- HTTP: request tenant from `X-Goso-Tenant` when `GOSO_MULTI_TENANT=1`; list/get 404 across tenants.
- `GET /api/tenant` `{tenant, multi_tenant}` (`/v1` alias).
- Control-plane Settings: current tenant (read-only in demo). If gateway reports multi-tenant, localStorage `goso_tenant` input for the header. i18n vi+en. No SSO.
- `GOSO_DATABASE_URL` / `postgres://` DSN: refuse open; StoreIface stays SQLite.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 072.

## Proof

- Two tenants: agent in A not visible to B (list + get).
- Session isolation (list + chat 404).
- Webhook + job isolation.
- Default path unchanged for existing tests (no header).
- Empty/invalid header → default.
- Header ignored when `GOSO_MULTI_TENANT` unset.
- View-token cannot switch tenant; POST still 403.
- Postgres DSN fail-closed (`ErrPostgresUnsupported`).

SQLite-lite limits (accepted): `agent_key`, `llm_providers.name`, and `vault_docs.path` stay process-global unique (not composite with `tenant_id`). Cron/connectors/secrets are not tenant-columned in this SPEC.

## Non-goals

SPEC 072+. Full RBAC (077). Live Postgres/pgvector host (DI-09). Tenant SSO. Per-tenant unique `agent_key` / provider name / vault path. Cron `tenant_id`. Binding/killing demo ports. Merge. Copying goclaw Go. Secrets in docs.
