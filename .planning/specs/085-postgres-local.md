# SPEC 085 — Local Postgres 16 + pgvector path (DI-09)

> After 084. Clean-room. Do not kill `:8082` `:8091` `:3000` `:18080`.
> **No cloud host.** Docker Compose local only. SQLite remains default.

Closes matrix **N1** path (gateway can open Postgres) and documents **V2** incremental pgvector. Live channel tokens stay env (084).

## GoClaw cite (docs only)

`/Users/mqglobal/Documents/goclaw/goclaw-source/docs/23-multi-tenant-architecture.md` SQL `tenant_id`.
`/Users/mqglobal/Documents/goclaw/goclaw-source/docs/07-bootstrap-skills-memory.md` hybrid FTS + vector as **host** concern.

## goso plan

1. Compose **profile** `postgres` (not default `up`): service `postgres` image `pgvector/pgvector:pg16`, host port **5433** (not 8082/8091/18080/3000), db `goso`, user/password from env placeholders empty-safe. Volume `pgdata`. Healthcheck `pg_isready`.
2. Gateway: `GOSO_DATABASE_URL=postgres://…` **opens a Postgres driver** (pgx via `database/sql`). **No silent SQLite fallback** if DSN is postgres. Unset DSN → SQLite `GOSO_DB_PATH` as today.
3. `RefusePostgres` only when DSN is postgres **and** driver Open fails? No: remove refuse-always. `Open` routes postgres DSN to `OpenPostgres`. Connect fail → return error (fail-closed).
4. Schema: Postgres tables matching current SQLite StoreIface (agents/sessions/messages/memories/vault/teams/webhooks/kg/channel_config/… + `tenant_id`). Use `$1` placeholders. `CREATE EXTENSION IF NOT EXISTS vector` **optional** — if it fails, log and continue (lexical still works). Incremental: add nullable `embedding vector` on `kg_entities` **only if** extension exists; search still FTS/LIKE until a later SPEC fills embeddings.
5. Tests: existing SQLite tests unchanged. New tests: `IsPostgresDSN`; `Open` with sqlite path still works; postgres DSN without server **errors** (not sqlite). If `GOSO_TEST_DATABASE_URL` set, round-trip one agent+session. **Do not** require docker in `go test` default CI.
6. Docs: SETUP/RUNBOOK `docker compose --profile postgres up -d postgres` then `GOSO_DATABASE_URL=postgres://goso:goso@127.0.0.1:5433/goso?sslmode=disable`. Update `docs/qa/071-pgvector-path.md` pointer. QA `docs/qa/085-postgres-local.md`.
7. Do **not** change default demo `:18080` to Postgres.

QC: typecheck if CP, `go test ./...`, build, gofmt, vet, agpl, agpl-docs.
Commit `admatrixmdp/spec085-postgres-local`. Do not merge. Do not start 086.
