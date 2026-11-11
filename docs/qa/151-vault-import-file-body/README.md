# SPEC 151 — vault import file bodies

Evidence for reading a document body from `--src-dir` when the export has no `vault_versions` row. No control-plane change, so prototype screenshots do not apply. No customer data and no secret values. Env vars are named, not filled in. Fixtures are synthetic Minh An rows.

## Tested SHA

`e82c22019f07926d85bfff1c860c236972e2173f`

`git status --porcelain` was empty before `go test` and before `qa:tabs`. This notes commit does not change gateway or control-plane code.

Postgres for the local `go test` was **16.15** (Ubuntu package `16.15-0ubuntu0.24.04.1`). The DSN is the env var `GOSO_TEST_POSTGRES_DSN`. The value is not stored here. This Postgres has no pgvector extension; the store logs that `CREATE EXTENSION vector` was skipped and continues. SQLite cases run in the same command. Postgres cases run because the DSN is set.

## Gateway tests

[go-test-e82c22019f07926d85bfff1c860c236972e2173f.txt](go-test-e82c22019f07926d85bfff1c860c236972e2173f.txt)

From the repo root:

```bash
go test ./... -count=1 -timeout 25m
```

Exit 0. Every package reported `ok` or `[no test files]`, including `gateway/internal/vaultimport` and `gateway/cmd/goso-vault-import`.

Verbose importer cases, with memory, SQLite, and Postgres subtests:

[go-test-vaultimport-e82c22019f07926d85bfff1c860c236972e2173f.txt](go-test-vaultimport-e82c22019f07926d85bfff1c860c236972e2173f.txt)

`TestFileBodyMinhAn` passed on `memory`, `sqlite`, and `postgres`. It covers a version row (body stays the version text), a file body and sha256 of the raw file bytes, a missing file, a file over 1 MiB (`too-large`), invalid UTF-8 and a NUL byte (`binary-skipped`), an empty file (`empty-body`), a missing `--src-dir` that writes nothing, per-tenant doc and link totals, a second run that does not write, a hand edit that stays `path-taken`, and an unchanged memory row.

## qa:tabs

[qa-tabs-e82c22019f07926d85bfff1c860c236972e2173f.log](qa-tabs-e82c22019f07926d85bfff1c860c236972e2173f.log)

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

## Flags and reasons

| Flag | Meaning |
|---|---|
| `--src-dir` | Source workspace. Required when a kept document has no version row. The export path is resolved the same way as `path-missing` |
| `--include-personal` | Default off. Personal documents are `scope-personal-held` once, including when a team document shares the path |
| `--keep-prefix` | Unchanged. Store the path with the root still attached |
| `--dry-run` | Unchanged. No write |

A version row still supplies the body, and its trailer sum is sha256 of the title and body prefix. A document with no version row takes the file bytes. The trailer sum is sha256 of those bytes. Over 1 MiB (`security.MaxAPIBody`, the same cap as `storage.MaxFileBytes`) is `too-large`. Non-UTF-8 or NUL is `binary-skipped`. An empty file is `empty-body`. None of those write a row.

The JSON report has `docs` and `links`. Each is a list per source tenant. `input = inserted + updated + skipped + rejected`, and the reason counts sum to `skipped + rejected`.
