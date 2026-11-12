# SPEC 154 — Postgres tsvector and GIN

Evidence for the additive `body_tsv` columns and GIN indexes. No control-plane change, so prototype screenshots do not apply. No customer data and no secret values. The Postgres DSN is the env var `GOSO_TEST_POSTGRES_DSN`. The value is not stored here.

## Candidate cap retest

Tested commit: `484042b1dfe81de026dd7f9268002106059dc7d8`

`git status --porcelain` was empty before `go test ./...` and `npm run qa:tabs`. The spec sentence that quotes these samples was added with the log, after the run. It does not change the query.

[go-test-484042b1dfe81de026dd7f9268002106059dc7d8.txt](go-test-484042b1dfe81de026dd7f9268002106059dc7d8.txt)

[qa-tabs-484042b1dfe81de026dd7f9268002106059dc7d8.log](qa-tabs-484042b1dfe81de026dd7f9268002106059dc7d8.log)

`go test ./...` failed in one place: `TestPostgresRecall50kAllMatchWithinCap`. Every other package passed, including `gateway/memorybench`.

That test inserts 50,000 Minh An rows. 49,999 fillers each contain one token of the query, plus one older needle that contains the rest. Fetched rows were **2000**, which is `recallCandidateCap`. The needle was recalled. It is not in the newest 500.

Latency of `RecallMemories` on that set, five samples:

`436.450566ms, 455.045362ms, 459.285839ms, 463.110639ms, 446.988949ms`

Median **455.045362ms**. Max **463.110639ms**. That max is over 50 ms. The selective 50,000-row case, where fillers do not share a query token, is still under 50 ms (median 1.123026ms on the earlier run in this same file's history, and the selective test still passes inside this log). This paragraph is the worst-case number. It is not replaced with the selective number.

`npm run qa:tabs` exited 0 (`flow chat`, then `qa:tabs ok`). Chrome **156.0.8078.47**. No tab changed. Gateway was restarted from this tree on a fresh SQLite file. `GOSO_VAULT_UPLOAD` was unset.

Search queries that this spec added already had a limit before this retest: memory search 50, vault `VaultSearchLimit` 50, progressive L1 and L2 `progressivePerTier` 25. No extra limit was missing there.

## Merge retest

`origin/main` at `d409b99` (SPEC 170) was merged in. `CHANGELOG.md` keeps the SPEC 154 entry and both SPEC 170 entries. `TestIngestJobTableAdditive` on Postgres now expects the extra generated `body_tsv` column; the SPEC 170 columns are still required, and SQLite's `vault_docs` column set is unchanged.

Tested commit: `5464e95ea50aef189b92c82f3c4c58ae22cb3a3c`

`git status --porcelain` was empty before `go test ./...` and `npm run qa:tabs`.

[go-test-5464e95ea50aef189b92c82f3c4c58ae22cb3a3c.txt](go-test-5464e95ea50aef189b92c82f3c4c58ae22cb3a3c.txt)

[qa-tabs-5464e95ea50aef189b92c82f3c4c58ae22cb3a3c.log](qa-tabs-5464e95ea50aef189b92c82f3c4c58ae22cb3a3c.log)

`go test ./...` exited 0 on SQLite and Postgres 16.15, including `gateway/internal/store` and `gateway/memorybench`. `npm run qa:tabs` exited 0 (`flow chat`, then `qa:tabs ok`). Chrome **156.0.8078.47**. No tab changed, so there are no prototype screenshots. Gateway was restarted from this tree on a fresh SQLite file before the browser run. `GOSO_VAULT_UPLOAD` was unset.

## Tested SHA

`a48fd9d93ec7039246d5b14013c5fd0dc1f681ba`

`git status --porcelain` was empty before `go test ./...`, the memory-vi bench, the SPEC 154 tests, and `npm run qa:tabs`.

[go-test-a48fd9d93ec7039246d5b14013c5fd0dc1f681ba.txt](go-test-a48fd9d93ec7039246d5b14013c5fd0dc1f681ba.txt)

[memory-vi-a48fd9d93ec7039246d5b14013c5fd0dc1f681ba.txt](memory-vi-a48fd9d93ec7039246d5b14013c5fd0dc1f681ba.txt)

[spec154-a48fd9d93ec7039246d5b14013c5fd0dc1f681ba.txt](spec154-a48fd9d93ec7039246d5b14013c5fd0dc1f681ba.txt)

[qa-tabs-a48fd9d93ec7039246d5b14013c5fd0dc1f681ba.log](qa-tabs-a48fd9d93ec7039246d5b14013c5fd0dc1f681ba.log)

One earlier `go test ./...` on this same commit failed only in cleanup of `TestTenants_PersistRestartAndSaveFailure` (`TempDir RemoveAll: directory not empty`). The test body had not failed an assertion. The rerun above exited 0. That test does not use the new columns.

## Bench vs floor `9545d08`

Both engines stay at 20/30. No group score changed, so `bench/memory-vi/baseline-9545d08.json` was not edited.

| Group | SQLite before | SQLite now | Postgres before | Postgres now |
|---|---:|---:|---:|---:|
| Trích xuất thông tin | 6/6 | 6/6 | 6/6 | 6/6 |
| Suy luận nhiều phiên | 3/4 | 3/4 | 4/4 | 4/4 |
| Cập nhật tri thức | 0/4 | 0/4 | 0/4 | 0/4 |
| Thẩm quyền | 0/4 | 0/4 | 0/4 | 0/4 |
| Từ chối | 4/4 | 4/4 | 4/4 | 4/4 |
| Cô lập | 4/4 | 4/4 | 4/4 | 4/4 |
| Tiếng Việt không dấu, đồng nghĩa | 3/4 | 3/4 | 2/4 | 2/4 |
| Total | 20/30 | 20/30 | 20/30 | 20/30 |

Postgres total equals SQLite (20/30). `tv-03` (`gui bao gia` / `báo giá`) is still a Postgres miss. Diacritics are SPEC 155. This spec does not install `unaccent`.

## 50,000 rows

Postgres **16.15** (Ubuntu `16.15-0ubuntu0.24.04.1`). No pgvector extension. The store logs that `CREATE EXTENSION vector` was skipped and continues.

Synthetic rows: 49,999 copies of Minh An demo sentences plus one older note, `Ghế mẫu Minh An mã MA-WALNUT-77 làm bằng gỗ sồi.` The query is `mã MA-WALNUT-77 làm bằng gỗ gì`. The note is not among the newest 500 rows.

`TestPostgresRecall50kUnder50ms` (`RecallMemories`), five samples after the plan check:

`845.791µs, 986.683µs, 820.991µs, 968.715µs, 409.402µs`. Median **845.791µs**.

`EXPLAIN` uses `Bitmap Index Scan on idx_memories_body_tsv`.

`TestRecallFor50kUnder50ms` (`pipeline.RecallFor`): **3.473678ms**, one item injected, body contains `MA-WALNUT-77` and `gỗ sồi`.

Both are under 50 ms.

## What each gate is

| Gate | Test |
|---|---|
| Only add columns. Do not modify old columns. | `TestPostgresBodyTSVAdditive`, `TestSQLiteHasNoBodyTSV` |
| Existing body is searchable after the column is added | `TestPostgresBodyTSVBackfillAndSearch` |
| Postgres bench score is not below SQLite, and no passing case regresses | `TestBaseline9545d08` |
| Recall of 50,000 rows finishes in under 50 ms | `TestPostgresRecall50kUnder50ms`, `TestRecallFor50kUnder50ms` |
| A note older than `recallScanCap` (500) is still recalled | `TestRecallOlderThanScanCap`, `TestRecallForOlderThanScanCap` |
| Rollback drops the index and leaves old columns | `TestPostgresTSVectorRollbackDropsIndexOnly` |

## Rollback

```sql
DROP INDEX IF EXISTS idx_memories_body_tsv;
DROP INDEX IF EXISTS idx_vault_docs_body_tsv;
DROP INDEX IF EXISTS idx_kg_entities_body_tsv;
```

Do not drop or alter a column that existed before this spec. After the indexes are gone, recall still reads `body_tsv`, so a note older than 500 rows is still returned; Postgres scans the column. Restoring the newest-500 scan means reverting the query code. The generated columns may be dropped only after that revert:

```sql
ALTER TABLE memories DROP COLUMN IF EXISTS body_tsv;
ALTER TABLE vault_docs DROP COLUMN IF EXISTS body_tsv;
ALTER TABLE kg_entities DROP COLUMN IF EXISTS body_tsv;
```

## qa:tabs

Chrome: **Google Chrome 156.0.8078.47**. Binary: `/usr/local/bin/google-chrome`.

`QA_SKIP_TABS` was not set. `QA_SHOTS` was not set. No tab copy changed, so there are no prototype screenshots.

From `control-plane/`:

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=/usr/local/bin/google-chrome npm run qa:tabs
```

Gateway was `go run ./gateway/cmd/goso-gateway gateway --host 127.0.0.1 --port 8080` from this tree, on SQLite (`GOSO_DATABASE_URL` unset). Fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. Set: `GOSO_DEV_MODE` on, `GOSO_RATE_LIMIT` 0, `GOSO_ENV` `development`.

Seed before the browser run: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

Exit 0. Last lines: `flow chat` then `qa:tabs ok`. Nothing was skipped. Honest empty states, not failures: `#/decision`, `#/decision-flow`, `#/hooks`, `#/meetings`, and the matching aliases.

## Queue line

In `.planning/specs/goso-2026-10-31-queue.md`, under "What to start next", `Then **SPEC 134**` is now `Then **SPEC 153**`. The same sentence still says not to start 135's fact updates before 134. That clause is the old dependency note, not the "what to start next" pointer, so it was left as written.
