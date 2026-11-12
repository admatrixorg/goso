# SPEC 153 — Vietnamese memory recall bench

Evidence for the 30-case Minh An bench and the floor recorded from main `9545d08`. No control-plane change, so prototype screenshots do not apply. No customer data and no secret values. The Postgres DSN is the env var `GOSO_TEST_POSTGRES_DSN`. The value is not stored here.

## Tested SHA

`5fad3a8916a6498a7066c78fcbbb63a404ef1a55`

`git status --porcelain` was empty before `go test ./...` and before `npm run qa:tabs`. This evidence commit only adds these transcripts.

[go-test-5fad3a8916a6498a7066c78fcbbb63a404ef1a55.txt](go-test-5fad3a8916a6498a7066c78fcbbb63a404ef1a55.txt)

[memory-vi-5fad3a8916a6498a7066c78fcbbb63a404ef1a55.txt](memory-vi-5fad3a8916a6498a7066c78fcbbb63a404ef1a55.txt)

[qa-tabs-5fad3a8916a6498a7066c78fcbbb63a404ef1a55.log](qa-tabs-5fad3a8916a6498a7066c78fcbbb63a404ef1a55.log)

## Floor

Both engines pass 20 of 30. The numbers are also in `bench/memory-vi/baseline-9545d08.json` and `.planning/specs/153-memory-vi-bench.md`.

| Group | SQLite | Postgres |
|---|---:|---:|
| Trích xuất thông tin | 6/6 | 6/6 |
| Suy luận nhiều phiên | 3/4 | 4/4 |
| Cập nhật tri thức | 0/4 | 0/4 |
| Thẩm quyền | 0/4 | 0/4 |
| Từ chối | 4/4 | 4/4 |
| Cô lập | 4/4 | 4/4 |
| Tiếng Việt không dấu, đồng nghĩa | 3/4 | 2/4 |
| Total | 20/30 | 20/30 |

Từ chối is 4/4 on both. Asking about something the notes never said (tc-01, tc-03, tc-04) admits nothing. `ok cảm ơn` (tc-02) admits nothing. Customer, tenant, and agent isolation (cl-01, cl-02, cl-03) admit nothing.

Postgres for this run was **16.15** (Ubuntu package `16.15-0ubuntu0.24.04.1`). This Postgres has no pgvector extension. The store logs that `CREATE EXTENSION vector` was skipped and continues. SQLite runs in the same command.

## Gateway tests

From the repo root, with `GOSO_TEST_POSTGRES_DSN` set:

```bash
go test ./... -count=1 -timeout 25m
```

Exit 0. Every package reported `ok`, including `gateway/memorybench`.

Verbose bench, same DSN:

```bash
go test ./gateway/memorybench -count=1 -timeout 15m -v -run TestBaseline
```

Exit 0. The transcript prints `memory-vi sqlite 20/30` and `memory-vi postgres 20/30`.

## qa:tabs

Chrome: **Google Chrome 156.0.8078.47**. Binary: `/usr/local/bin/google-chrome`.

`QA_SKIP_TABS` was not set. `QA_SHOTS` was not set.

From `control-plane/`:

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=/usr/local/bin/google-chrome npm run qa:tabs
```

Gateway was `go run ./gateway/cmd/goso-gateway gateway --host 127.0.0.1 --port 8080` from this tree. Fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. Unset: `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_VAULT_SCOPED_SYNC`, `GOSO_LITE`, `GOSO_CONTACTS_DURABLE`. Set: `GOSO_DEV_MODE` on, `GOSO_RATE_LIMIT` 0, `GOSO_ENV` `development`.

Seed before the browser run: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

Exit 0. Last lines: `flow chat` then `qa:tabs ok`. Nothing was skipped. Honest empty states, not failures: `#/decision`, `#/decision-flow`, `#/hooks`, `#/meetings`, and the matching aliases.
