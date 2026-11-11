# Durable tenant registry

Evidence for SPEC 151. No customer data and no secret values. Env vars are named, not filled in. Fixtures in the tests are synthetic Minh An rows (`minh-an`, `phap-che`, `kinh-doanh`).

## Tested SHA

`6ef1a0b15c9b44e5e90460b2f66af6a9a27e8e71`

`git status --porcelain` was empty before the `go test` commands and before `npm run qa:tabs`. The transcripts below do not print a later SHA.

Postgres for the local `go test` was **16.15** (Ubuntu package). The DSN is the env var `GOSO_TEST_POSTGRES_DSN`. The value is not stored here. This server does not have the `vector` extension. `CREATE EXTENSION vector` logs a skip and the suite still passes, which is the same path the plain `postgres:16` CI service takes.

## No flag

An empty `tenant_registry` table loads as the master tenant only. That is the previous startup. Persistence is the default whenever the gateway opens a SQLite file or Postgres. There is no on/off flag. `GOSO_DB_PATH` unset or `:memory:` still keeps the registry in the process, because that mode has no database to reopen.

## Gateway tests

SQLite (DSN unset):

[go-test-6ef1a0b15c9b44e5e90460b2f66af6a9a27e8e71-sqlite.txt](go-test-6ef1a0b15c9b44e5e90460b2f66af6a9a27e8e71-sqlite.txt)

Postgres 16.15:

[go-test-6ef1a0b15c9b44e5e90460b2f66af6a9a27e8e71-postgres.txt](go-test-6ef1a0b15c9b44e5e90460b2f66af6a9a27e8e71-postgres.txt)

From the repo root:

```bash
go test ./... -count=1 -timeout 300s
```

Exit 0 both times. Every package reported `ok` or `[no test files]`.

Named cases, verbose, with the Postgres DSN set:

[go-test-6ef1a0b15c9b44e5e90460b2f66af6a9a27e8e71-cases.txt](go-test-6ef1a0b15c9b44e5e90460b2f66af6a9a27e8e71-cases.txt)

```bash
go test ./gateway/internal/store/ ./gateway/internal/tenant/ ./gateway/internal/httpapi/ \
  -count=1 -timeout 180s -v \
  -run 'TenantRegistry|RegistryPersist|RegistryEmpty|Tenants_Persist|LoadMemory'
```

Exit 0. SQLite and Postgres both pass restart, write-failure, migration rollback, and the memory/vault/contacts survival checks. The HTTP failure line is `goso tenant registry: create failed class=closed`.

## qa:tabs

[qa-tabs-6ef1a0b15c9b44e5e90460b2f66af6a9a27e8e71.txt](qa-tabs-6ef1a0b15c9b44e5e90460b2f66af6a9a27e8e71.txt)

Chrome: **Google Chrome 156.0.8078.47**.

From `control-plane/`, `QA_SHOTS` unset:

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=$(command -v google-chrome) npm run qa:tabs
```

Gateway was `go run ./gateway/cmd/goso-gateway gateway` on `127.0.0.1:8080` with a fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. Env names set: `GOSO_DEV_MODE`, `GOSO_RATE_LIMIT` (0), `GOSO_ENV` (`development`). Unset: `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_CONTACTS_DURABLE`, `GOSO_VAULT_SCOPED_SYNC`, `GOSO_LITE`, `GOSO_MULTI_TENANT`.

Seed before the browser run: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

Exit 0. Last lines: `flow chat` then `qa:tabs ok`. Nothing was skipped. `#/tenants` passed. Honest empty states, not failures: `#/decision`, `#/decision-flow`, `#/hooks`, `#/meetings`, and the matching aliases.

This change does not alter the control plane. Prototype screenshots are not part of this run.
