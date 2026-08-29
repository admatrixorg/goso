# QA — SPEC 085 Local Postgres 16 + pgvector path (DI-09 N1)

Date: 2026-08-29. Clean-room Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not merge. Do not start SPEC 086.

Closes matrix **N1** (gateway can open Postgres) and documents **V2** incremental pgvector. Live channel tokens stay env (084). SQLite remains default. Default demo `:18080` is **not** switched to Postgres.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| SQL `WHERE tenant_id` isolation; fail-closed missing tenant | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/23-multi-tenant-architecture.md` |
| Hybrid FTS + vector memory; vector is a **host** concern | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/07-bootstrap-skills-memory.md` |

goso mapping (self-written): compose **profile** `postgres` (not default `up`) runs `pgvector/pgvector:pg16` on host **5433**. `store.Open` routes `postgres://` / `postgresql://` (path or `GOSO_DATABASE_URL`) to `OpenPostgres` (pgx via `database/sql`). Connect fail returns the driver error — **never** SQLite. Unset DSN keeps `GOSO_DB_PATH` SQLite / in-memory. Schema matches StoreIface including `tenant_id` where SQLite already has it. `CREATE EXTENSION vector` is optional; on success a nullable `kg_entities.embedding` is added. Search stays lexical (strpos/LIKE) until a later SPEC fills embeddings.

## What changed

- `compose.yml` service `postgres` behind profile `postgres`, image `pgvector/pgvector:pg16`, `5433:5432`, db/user/password empty-safe (`goso` default), volume `pgdata`, `pg_isready` healthcheck.
- `Open` → `OpenPostgres` for postgres DSN; `OpenSQLite` still refuses a postgres DSN (no sqlite fallback).
- Postgres DDL for agents/sessions/messages/memories/vault/teams/webhooks/kg/channel_config/… + `tenant_id`. Secrets use `BYTEA`.
- Tests: SQLite path unchanged; DSN without a server errors; `GOSO_TEST_DATABASE_URL` skip-if-unset agent+session roundtrip.
- SETUP / RUNBOOK: `docker compose --profile postgres up -d postgres` then `GOSO_DATABASE_URL=postgres://goso:goso@127.0.0.1:5433/goso?sslmode=disable`.
- Pointer: `docs/qa/071-pgvector-path.md` → this file.

## Commands

```
go test ./...
gofmt -l gateway desktop
go vet ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Optional (not default CI):

```
docker compose --profile postgres up -d postgres
GOSO_TEST_DATABASE_URL='postgres://goso:goso@127.0.0.1:5433/goso?sslmode=disable' go test ./gateway/internal/store -count=1 -run TestPostgresRoundTrip
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 086.

## Proof

- `go test ./... -count=1` OK (no Docker; `TestPostgresRoundTrip` skipped)
- `gofmt -l gateway desktop` empty; `go vet ./...` OK; `go build -o bin/goso-gateway ./gateway/cmd/goso-gateway` OK
- `agpl-check.sh` OK; `./scripts/agpl-check-docs.sh` OK
- `TestIsPostgresDSN`, `TestPgSQLRewrite`, `TestOpenSQLitePathUnchanged`
- `TestOpenPostgresDSNWithoutServerErrors` (port 1, no sqlite file)
- `TestOpenSQLiteStillRefusesPostgresDSN`
- Existing SQLite store tests still run on `?` + FTS5
- Lexical `UNION ALL` subqueries aliased `AS hits` (valid on SQLite and Postgres)
- `POST /api/system/backup` refuses when `GOSO_DATABASE_URL` is postgres (`postgres backup not supported`)
- Default `docker compose up` does not start postgres. Host 5433 was already held by OrbStack; this SPEC did not bind or kill demo ports.

## Non-goals

Switching demo `:18080` to Postgres. Filling embeddings / hybrid vector search (later SPEC). Requiring Docker in `go test`. Cloud Postgres host. Copying goclaw Go. Merge. SPEC 086.
