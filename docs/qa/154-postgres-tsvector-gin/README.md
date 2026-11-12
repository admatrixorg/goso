# SPEC 154 — Postgres tsvector and GIN

Evidence for the additive `body_tsv` columns and GIN indexes. No control-plane change, so prototype screenshots do not apply. No customer data and no secret values. The Postgres DSN is the env var `GOSO_TEST_POSTGRES_DSN`. The value is not stored here.

## AND then OR retest

Tested commit: `8837e5ddb9f981927516c91314c3bc77380ee4eb`

`git status --porcelain` was empty before `go test ./...` and `npm run qa:tabs`. The `EXPLAIN (ANALYZE, BUFFERS)` text below was captured from the same code, in `TestPostgresRecall50kAllMatchWithinCap`, immediately before that commit. The commit adds these files and does not change the query.

[go-test-8837e5ddb9f981927516c91314c3bc77380ee4eb.txt](go-test-8837e5ddb9f981927516c91314c3bc77380ee4eb.txt)

[qa-tabs-8837e5ddb9f981927516c91314c3bc77380ee4eb.log](qa-tabs-8837e5ddb9f981927516c91314c3bc77380ee4eb.log)

[explain-analyze-8837e5ddb9f981927516c91314c3bc77380ee4eb.txt](explain-analyze-8837e5ddb9f981927516c91314c3bc77380ee4eb.txt)

`go test ./...` exited 0 on SQLite and Postgres 16.15, including `gateway/memorybench`. `npm run qa:tabs` exited 0 (`flow chat`, then `qa:tabs ok`). Chrome **156.0.8078.47**. No tab changed. Gateway was restarted from this tree on a fresh SQLite file. `GOSO_VAULT_UPLOAD` was unset.

Selective 50,000-row recall (`TestPostgresRecall50kUnder50ms`), five samples: 2.477017ms, 2.217858ms, 2.49087ms, 2.384925ms, 2.376734ms. Median **2.384925ms**.

Worst case, every filler matches one query token (`TestPostgresRecall50kAllMatchWithinCap`): fetched **2000** (the cap). Five samples: 38.407346ms, 41.798638ms, 39.174838ms, 38.70924ms, 37.832653ms. Median **38.70924ms**. Max **41.798638ms**. Under 50 ms.

`TestPostgresRecallOldestFullMatch`: 49,999 newer rows share one query token, and the oldest row contains every query token. A newest-2000 slice does not include that row. Recall still returns it.

### Step 1, AND, `LIMIT 2000`

```
Limit  (cost=2070.67..2070.67 rows=1 width=119) (actual time=8.799..8.800 rows=1 loops=1)
  Buffers: shared hit=488
  ->  Sort  (cost=2070.67..2070.67 rows=1 width=119) (actual time=8.798..8.799 rows=1 loops=1)
        Sort Key: m.created_at DESC
        Sort Method: quicksort  Memory: 25kB
        Buffers: shared hit=488
        ->  Nested Loop  (cost=2058.60..2070.66 rows=1 width=119) (actual time=8.795..8.796 rows=1 loops=1)
              Join Filter: (m.session_id = sess.id)
              Buffers: shared hit=488
              ->  Bitmap Heap Scan on memories m  (cost=2058.46..2062.48 rows=1 width=119) (actual time=8.788..8.789 rows=1 loops=1)
                    Recheck Cond: (body_tsv @@ '''mã'' & ''ma'' & ''walnut'' & ''77'' & ''làm'' & ''bằng'' & ''gỗ'' & ''gì'''::tsquery)
                    Filter: ((kind = ANY ('{episodic,durable}'::text[])) AND (session_id <> '20261112-a5cfe4f5d2b70c23'::text) AND (tenant_id = 'default'::text))
                    Heap Blocks: exact=1
                    Buffers: shared hit=486
                    ->  Bitmap Index Scan on idx_memories_body_tsv  (cost=0.00..2058.46 rows=1 width=0) (actual time=8.783..8.783 rows=1 loops=1)
                          Index Cond: (body_tsv @@ '''mã'' & ''ma'' & ''walnut'' & ''77'' & ''làm'' & ''bằng'' & ''gỗ'' & ''gì'''::tsquery)
                          Buffers: shared hit=485
              ->  Index Scan using idx_sessions_tenant on sessions sess  (cost=0.15..8.17 rows=1 width=32) (actual time=0.004..0.004 rows=1 loops=1)
                    Index Cond: (tenant_id = 'default'::text)
                    Filter: (agent_id = '20261112-9fb31361fb141aef'::text)
                    Buffers: shared hit=2
Planning:
  Buffers: shared hit=1
Planning Time: 0.117 ms
Execution Time: 8.814 ms
```

### Step 2, OR, `LIMIT 1999`

The planner seq-scans `memories` here (50,000 rows) and top-N sorts. It does not pick `idx_memories_tenant_created`. Execution time is 32.223 ms.

```
Limit  (cost=2662.65..2662.95 rows=117 width=119) (actual time=31.943..32.142 rows=1999 loops=1)
  Buffers: shared hit=1565
  ->  Sort  (cost=2662.65..2662.95 rows=117 width=119) (actual time=31.943..32.018 rows=1999 loops=1)
        Sort Key: m.created_at DESC
        Sort Method: top-N heapsort  Memory: 612kB
        Buffers: shared hit=1565
        ->  Hash Join  (cost=8.18..2658.63 rows=117 width=119) (actual time=0.014..22.148 rows=50000 loops=1)
              Hash Cond: (m.session_id = sess.id)
              Buffers: shared hit=1565
              ->  Seq Scan on memories m  (cost=0.00..2563.00 rows=32820 width=119) (actual time=0.005..15.700 rows=50000 loops=1)
                    Filter: ((body_tsv @@ '''mã'' | ''ma'' | ''walnut'' | ''77'' | ''làm'' | ''bằng'' | ''gỗ'' | ''gì'''::tsquery) AND (kind = ANY ('{episodic,durable}'::text[])) AND (session_id <> '20261112-a5cfe4f5d2b70c23'::text) AND (tenant_id = 'default'::text))
                    Buffers: shared hit=1563
              ->  Hash  (cost=8.17..8.17 rows=1 width=32) (actual time=0.007..0.008 rows=3 loops=1)
                    Buckets: 1024  Batches: 1  Memory Usage: 9kB
                    Buffers: shared hit=2
                    ->  Index Scan using idx_sessions_tenant on sessions sess  (cost=0.15..8.17 rows=1 width=32) (actual time=0.004..0.006 rows=3 loops=1)
                          Index Cond: (tenant_id = 'default'::text)
                          Filter: (agent_id = '20261112-9fb31361fb141aef'::text)
                          Buffers: shared hit=2
Planning:
  Buffers: shared hit=1
Planning Time: 0.122 ms
Execution Time: 32.223 ms
```

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
