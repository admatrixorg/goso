# Issue 43 — stable plan for recall step 2

Follow-up to SPEC 154. Postgres recall step 2 is the OR of the query tokens. It still orders by `created_at DESC`, still stops at the unused part of `recallCandidateCap` (2000), and still does not call `ts_rank`. No new index. Sample rows are Minh An fixtures only. The DSN is `GOSO_TEST_POSTGRES_DSN`; the value is not stored here.

Local run: PostgreSQL 16.15. The statements below are `EXPLAIN (ANALYZE, BUFFERS)` from that run. Full text: [explain-analyze.txt](explain-analyze.txt).

## Which plan runs

Step 2 reads at most `2 * limit` newest rows for the tenant through `idx_memories_tenant_created`. The `ORDER BY created_at DESC LIMIT` sits in a subquery, so the tsquery is applied after that window. `SET LOCAL enable_seqscan = off` is set for the statement. When the window already contains `limit` matches, those rows are the candidates.

When the window is short of `limit`, those rows are discarded. A second statement loads the newest matches. Its only indexable predicate is `body_tsv @@ to_tsquery(...)`. `tenant_id || '' = $tenant` is a filter, so the tenant btree cannot answer it. Sequential scans are disabled, so the plan is a bitmap index scan of `idx_memories_body_tsv`.

| Shape | OR hits | Candidates from | Plan used for those rows | RecallMemories max | Median |
|---|---:|---|---|---:|---:|
| Selective, near 0 matches | 1 of 50,000 | GIN bitmap (probe returned 0) | `Bitmap Index Scan on idx_memories_body_tsv` | 3.11426 ms | 2.852304 ms |
| About 5% match (2,501 / 50,000) | 2,501 | GIN bitmap (probe returned 0) | `Bitmap Index Scan on idx_memories_body_tsv` | 20.040666 ms | 18.592981 ms |
| All match | 50,000 | Ordered index (probe returned 1,999) | `Index Scan using idx_memories_tenant_created` | 15.126032 ms | 14.440229 ms |

The 50 ms cap is unchanged. Five samples each. `TestPostgresRecallOldestFullMatch` passed in the same run. `gateway/memorybench` `TestBaseline9545d08` was 20/30 on SQLite and 20/30 on Postgres.

## Selective (near 0)

Probe budget 4,000, limit 2,000. The index scan reads 4,000 rows and the tsquery filter removes all of them. Step 2 then uses the GIN bitmap, which returns the one matching row. Execution time of that bitmap statement: 0.281 ms.

```
Index Scan using idx_memories_tenant_created on memories
  Index Cond: (tenant_id = 'default'::text)
  (actual rows=4000)
Bitmap Index Scan on idx_memories_body_tsv
  Index Cond: (body_tsv @@ '''mã'' | ''ma'' | ''walnut'' | ''77'' | ''làm'' | ''bằng'' | ''gỗ'' | ''gì'''::tsquery)
  (actual rows=1)
```

`RecallMemories` samples: 2.852304 ms, 2.535213 ms, 2.729751 ms, 3.11426 ms, 3.049055 ms. Median **2.852304 ms**. Max **3.11426 ms**.

## About 5%

2,501 rows match the OR (every 20th filler, plus the older full-match needle). The newest fillers do not match, so the probe of 3,998 rows returns 0. The GIN bitmap returns 2,501 hits and keeps the newest 1,999. Execution time of that bitmap statement: 8.752 ms. Fetched candidates: 2,000, inside the cap. The needle is recalled from the AND step.

```
Index Scan using idx_memories_tenant_created on memories
  Index Cond: (tenant_id = 'default'::text)
  (actual rows=3998, filter removed 3998)
Bitmap Index Scan on idx_memories_body_tsv
  Index Cond: (body_tsv @@ '''mã'' | ''ma'' | ''walnut'' | ''77'' | ''làm'' | ''bằng'' | ''gỗ'' | ''gì'''::tsquery)
  (actual rows=2501)
```

`RecallMemories` samples: 18.657668 ms, 18.466075 ms, 17.883106 ms, 18.592981 ms, 20.040666 ms. Median **18.592981 ms**. Max **20.040666 ms**.

## All match

The probe window is entirely matches, so step 2 stops on `Index Scan using idx_memories_tenant_created` and does not run the bitmap statement. Probe rows: 1,999, which is the limit. Fetched candidates: 2,000, inside the cap. Execution time of the probe: 3.355 ms.

```
Index Scan using idx_memories_tenant_created on memories
  Index Cond: (tenant_id = 'default'::text)
  (actual rows=3998, outer limit keeps 1999)
```

`RecallMemories` samples: 14.65876 ms, 14.440229 ms, 15.126032 ms, 14.264909 ms, 14.382327 ms. Median **14.440229 ms**. Max **15.126032 ms**.
