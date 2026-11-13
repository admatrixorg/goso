# SPEC 173 — context file import

Offline importer for GoClaw `agent_context_files` and `user_context_files` into the SPEC 150 `context_files` table. The input is a JSON or JSONL export. This tool does not connect to GoClaw. No control-plane tab changed, so prototype screenshots do not apply. No customer data and no secret values. The Postgres DSN is the env var `GOSO_TEST_POSTGRES_DSN`. The value is not stored here.

## Column mapping

| GoClaw | `context_files` |
|---|---|
| `tenant_id` | Kept for `--source-tenant` only. Written as `--tenant`. |
| `agent_id` + `file_name` | Agent file. `agent_id` selects an existing agent (`--agent-map`, or an agent whose id or `agent_key` equals it). Stored owner is that agent's id. Natural key `agent_id` + `file_name`. |
| `agent_id` + `user_id` + `file_name` | User file. The agent must be in `--tenant`. The stored owner is `<channel>:<sender_id>` from `--customer-map`, or `goclaw:<user_id>` when the id is absent. Two agents that disagree on the same customer and name are `scope-conflict`. Natural key `agent_id` + `user_id` + `file_name`. |
| `content` | `body` |
| `created_at`, `updated_at` | Stored on the row. A rerun with the same body and clocks does not write. |

The GoClaw uuid primary key is not stored. The sample is `gateway/internal/contextimport/testdata/minh-an.json` (fake Minh An agents and users, including per-user files).

## Tested SHA

`5cefc1bd00a28a88909129be9bee8c09cc5d0397`

The transcripts below were captured from that tree. This evidence commit adds the logs and this note. It does not change the importer.

Postgres for the local run was **16.15** (Ubuntu package `16.15-0ubuntu0.24.04.1`). SQLite cases run in the same command. Postgres cases run because the DSN is set. The Postgres open logs `CREATE EXTENSION vector skipped` because this server has no pgvector extension. That line is the existing store, not a new dependency.

## go test

From the repo root:

```bash
go test ./... -count=1 -timeout 25m
```

[go-test.txt](go-test.txt)

Exit 0. Every package reported `ok` or `[no test files]`, including `gateway/internal/contextimport` and `gateway/cmd/goso-context-import`.

Verbose case log, same DSN:

```bash
go test ./gateway/internal/contextimport ./gateway/cmd/goso-context-import -count=1 -timeout 180s -v
```

[go-test-cases.txt](go-test-cases.txt)

Exit 0. `TestMinhAnFixture` passed on `sqlite`, `postgres`, and `memory`, including flag-off (the imported text is not in the prompt) and flag-on (the imported `contextfile.Prompt` block is what the runner loads). Dry-run, rerun, per-user files, tenant isolation, the owner cap, JSONL, and a user-file conflict passed on the stores those tests open.

## go test -race

Touched packages:

```bash
go test -race -count=1 -timeout 15m ./gateway/internal/contextimport ./gateway/cmd/goso-context-import ./gateway/internal/store
```

[go-test-race.txt](go-test-race.txt)

Exit 0. The same DSN was set, so the race run includes the Postgres cases. `gateway/internal/store` is included because the importer now asks `PutContextFile` to keep a source clock.

## qa:tabs

Code under test: `db01bf264563ec5f336a4b8c232145823b830329`. The log is [qa-tabs-db01bf264563ec5f336a4b8c232145823b830329.log](../173/qa-tabs-db01bf264563ec5f336a4b8c232145823b830329.log).

Chrome binary: `/usr/local/bin/google-chrome`. `QA_SKIP_TABS` was not set. `QA_SHOTS` was not set.

From `control-plane/`:

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=/usr/local/bin/google-chrome npm run qa:tabs
```

Gateway was `go run ./gateway/cmd/goso-gateway gateway --host 127.0.0.1 --port 8080` from that commit. Fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. Unset: `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_VAULT_SCOPED_SYNC`, `GOSO_LITE`, `GOSO_CONTACTS_DURABLE`, `GOSO_CONTEXT_FILES`. Set: `GOSO_DEV_MODE` on, `GOSO_RATE_LIMIT` 0, `GOSO_ENV` development.

Seed before the browser run: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

Exit 0. Last lines: `flow chat` then `qa:tabs ok`.
