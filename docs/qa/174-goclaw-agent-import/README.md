# SPEC 174 — GoClaw agent import

Evidence for the offline importer in `gateway/cmd/goso-agent-import` and `gateway/internal/agentimport`. No control-plane change, so prototype screenshots do not apply. No customer data and no secret values. Env vars are named, not filled in. The fixture is synthetic Minh An (`tu van`, `phap che`, `ke toan`, users `lan` and `hoa`).

The command reads a JSON or JSONL file. It does not open a GoClaw database. Dry-run is the default and writes nothing, including the map table. `--apply` adds agents, teams, members, and links. `--rollback --apply` removes only rows mapped for that tenant.

Postgres for the local `go test` is 16.15 when `GOSO_TEST_POSTGRES_DSN` is set. The value is not stored here. This Postgres has no pgvector extension. The store logs that `CREATE EXTENSION vector` was skipped. SQLite and the in-memory store run in the same command.

## Gateway tests

From the repo root, with `GOSO_TEST_POSTGRES_DSN` set for the Postgres cases:

```bash
go test -race -count=1 -timeout 180s ./gateway/internal/agentimport ./gateway/cmd/goso-agent-import
go test ./... -count=1 -timeout 25m
```

Logs from this branch:

- [go-test-race.txt](go-test-race.txt) — `go test -race` on the importer and the command. `TestMinhAnFixture` passed on `memory`, `sqlite`, and `postgres`.
- [go-test.txt](go-test.txt) — `go test ./... -count=1 -timeout 25m` exited 0.

`gitleaks detect` on the git history and `gitleaks detect --no-git` on the working tree both reported no leaks. The Postgres DSN is not in either log.

## qa:tabs

Code under test: `e80d90cfd3d34d052fcbc37a8f6b8b7dd935a77e`. The log is [qa-tabs-e80d90cfd3d34d052fcbc37a8f6b8b7dd935a77e.log](../174/qa-tabs-e80d90cfd3d34d052fcbc37a8f6b8b7dd935a77e.log). Earlier logs stay in `docs/qa/174/`.

Chrome **156.0.8078.47**. `CHROME_PATH=/usr/local/bin/google-chrome`. `QA_SKIP_TABS` and `QA_SHOTS` were unset.

Gateway was this commit, `go run ./gateway/cmd/goso-gateway gateway --host 127.0.0.1 --port 8080`, fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. Unset: `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_VAULT_SCOPED_SYNC`, `GOSO_LITE`, `GOSO_CONTACTS_DURABLE`. Set: `GOSO_DEV_MODE` on, `GOSO_RATE_LIMIT` 0, `GOSO_ENV` `development`. Seed before the browser run: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

Exit 0. Last lines: `flow chat` then `qa:tabs ok`.

## Dry-run shape

```bash
go run ./gateway/cmd/goso-agent-import \
  --export gateway/internal/agentimport/testdata/minh-an.json \
  --source-tenant aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1 \
  --tenant minh-an \
  --db /tmp/minh-an-agents.db
```

Stdout is counts and reason codes. It does not include prompt text or the fixture markers `SECRETMARKERdonotimport` and `EMBEDMARKERvector`. Add `--apply` only when those counts are the ones to write. Do not point `--db` at a production database.
