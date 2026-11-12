# SPEC 173 — context file import

Offline importer for agent and user context files into the SPEC 150 `context_files` table. No control-plane change, so prototype screenshots do not apply and `qa:tabs` was not run. No customer data and no secret values. The Postgres DSN is the env var `GOSO_TEST_POSTGRES_DSN`. The value is not stored here.

The input JSON is an assumption. Issues #26, #28, and #29 name `agent_context_files` and `user_context_files` and do not list columns. The sample fixture is `gateway/internal/contextimport/testdata/minh-an.json`.

## Tested SHA

`a846ab1c3244b4440ce3cfb6501788bb9c23bd0c`

The transcripts below were captured from that tree. This evidence commit adds the logs and this note. It does not change the importer.

`git status --porcelain` showed only `docs/qa/173-context-import/` when these logs were copied. The Postgres open logs `CREATE EXTENSION vector skipped` because this server has no pgvector extension. That line is the existing store, not a new dependency.

Postgres for the local run was **16.15** (Ubuntu package `16.15-0ubuntu0.24.04.1`). SQLite cases run in the same command. Postgres cases run because the DSN is set.

## go test

From the repo root:

```bash
go test ./... -count=1 -timeout 25m
```

[go-test.txt](go-test.txt)

Exit 0. Every package reported `ok` or `[no test files]`, including `gateway/internal/contextimport` and `gateway/cmd/goso-context-import`. `gateway/internal/store` and `gateway/memorybench` passed on this Postgres as well.

Verbose case log, same DSN:

```bash
go test ./gateway/internal/contextimport ./gateway/cmd/goso-context-import -count=1 -timeout 180s -v
```

[go-test-cases.txt](go-test-cases.txt)

Exit 0. `TestMinhAnFixture` passed on `sqlite`, `postgres`, and `memory`, including flag-off (the imported text is not in the prompt) and flag-on (the imported `contextfile.Prompt` block is what the runner loads). Dry-run, rerun, tenant isolation, the owner cap, and invalid rows passed on all three stores.

## go test -race

Touched packages only:

```bash
go test -race -count=1 -timeout 15m ./gateway/internal/contextimport ./gateway/cmd/goso-context-import
```

[go-test-race.txt](go-test-race.txt)

Exit 0. The same DSN was set, so the race run includes the Postgres cases.

## qa:tabs

No control-plane tab, route, or copy changed. `npm run qa:tabs` was not run.
