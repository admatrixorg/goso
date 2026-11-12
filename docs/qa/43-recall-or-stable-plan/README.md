# Issue 43 — stable plan for recall step 2

Follow-up to SPEC 154. Postgres recall step 2 is the OR of the query tokens. It still orders by `created_at DESC`, still stops at the unused part of `recallCandidateCap` (2000), and still does not call `ts_rank`. No new index. Sample rows are Minh An fixtures only. The DSN is `GOSO_TEST_POSTGRES_DSN`; the value is not stored here.

Local run: PostgreSQL 16.15. The statements below are `EXPLAIN (ANALYZE, BUFFERS)` from that run. Full text: [explain-analyze.txt](explain-analyze.txt).

## Which plan runs

Step 2 picks a plan from the scope, not from the planner's row estimate.

1. Newest `2 * limit` rows of the tenant through `idx_memories_tenant_created`, then the OR and the scope filters. `SET LOCAL enable_seqscan = off`. When that window already holds the limit, those rows are the candidates.
2. When it does not, a customer with fewer memories than the budget is read through `idx_memories_customer` (`tenant_id`, `customer_key`). An agent with fewer memories than the budget is read through `idx_memories_session`, starting from that agent's sessions (`OFFSET 0` so the tsquery is not the driver).
3. Otherwise the newest matches for that tenant come from a materialized statement whose predicates are `tenant_id = ?` and `body_tsv @@`. Sequential scans and plain index scans are off, and `cpu_tuple_cost = 1`. On a single tenant whose OR is already selective, the plan is a GIN bitmap and `tenant_id` is a filter of that small bitmap. When other tenants make the OR common, the plan is `BitmapAnd` of the tenant btree and `idx_memories_body_tsv`, and the heap fetch is the overlap.

| Shape | Candidates from | Plan | Median | Max |
|---|---|---|---:|---:|
| Selective, near 0 | GIN bitmap (probe returned 0) | `Bitmap Index Scan on idx_memories_body_tsv` | 10.301485 ms | 11.347417 ms |
| About 5% (2,501 / 50,000) | GIN bitmap (probe returned 0) | `Bitmap Index Scan on idx_memories_body_tsv` | 20.724909 ms | 20.900803 ms |
| All match | Ordered index (probe returned 1,999) | `Index Scan using idx_memories_tenant_created` | 14.619151 ms | 15.807246 ms |
| Target tenant selective, 4 other tenants × 50,000 all match the OR | BitmapAnd (probe returned 0) | `BitmapAnd` of `idx_memories_customer` (`tenant_id`) and `idx_memories_body_tsv` | 24.811498 ms | 24.947209 ms |
| Customer scope, 5 rows for the customer, 49,995 other customers all match | Customer index | `Index Scan using idx_memories_customer` | 2.32431 ms | 2.595676 ms |
| Agent scope, 16 rows for the agent, 49,980 rows for another agent all match | Agent sessions | `Nested Loop` + `Index Scan using idx_memories_session` | 10.449826 ms | 10.884709 ms |

The 50 ms cap is unchanged. Five samples of `RecallMemories` per shape. `TestPostgresRecallOldestFullMatch` passed in the same run.

## Selective (near 0)

Probe: `Index Scan using idx_memories_tenant_created`, 4,000 rows, the OR filter removes all of them. Candidates: `Bitmap Index Scan on idx_memories_body_tsv`, 1 row, execution 0.288 ms. `tenant_id` is a filter on that one row.

Samples: 10.045848 ms, 10.280274 ms, 10.301485 ms, 10.330556 ms, 11.347417 ms. Median **10.301485 ms**. Max **11.347417 ms**.

## About 5%

2,501 OR hits. Probe returns 0. Candidates: `Bitmap Index Scan on idx_memories_body_tsv`, 2,501 rows, execution 9.083 ms.

Samples: 20.900803 ms, 20.888421 ms, 19.858594 ms, 20.043022 ms, 20.724909 ms. Median **20.724909 ms**. Max **20.900803 ms**.

## All match

Probe returns 1,999, which is the limit, so the bitmap statement does not run. Plan: `Index Scan using idx_memories_tenant_created`. Execution 3.510 ms. Fetched candidates: 2,000.

Samples: 15.807246 ms, 14.619151 ms, 14.326667 ms, 15.113829 ms, 14.579347 ms. Median **14.619151 ms**. Max **15.807246 ms**.

## Other tenants

250,000 rows. The target tenant has one OR hit. Four other tenants have 50,000 rows each and every row matches the OR (200,001 global OR hits). Probe on the target tenant returns 0. The candidate statement is:

```
BitmapAnd
  Bitmap Index Scan on idx_memories_customer
    Index Cond: (tenant_id = 'tenant-a')
    (actual rows=50000)
  Bitmap Index Scan on idx_memories_body_tsv
    Index Cond: (body_tsv @@ OR)
    (actual rows=200001)
Bitmap Heap Scan
  Recheck Cond: (tenant_id = 'tenant-a' AND body_tsv @@ OR)
  (actual rows=1, heap blocks exact=1)
```

Execution 16.290 ms. The heap fetch is the one target row, not the other tenants' matches.

Samples: 24.811498 ms, 24.806941 ms, 24.154135 ms, 24.833638 ms, 24.947209 ms. Median **24.811498 ms**. Max **24.947209 ms**.

## Customer scope

50,000 rows in the tenant match the OR. Five of them belong to `zalo-personal:cust-a`, including the older needle. The other 49,995 use another customer key. Step 2 path: `customer-index`.

`Index Scan using idx_memories_customer`, index condition `tenant_id` and `customer_key`, 5 rows. Execution 0.048 ms.

Samples: 2.595676 ms, 1.718774 ms, 2.3307 ms, 2.237629 ms, 2.32431 ms. Median **2.32431 ms**. Max **2.595676 ms**.

## Other agent

The target agent has 16 rows, including the older needle. Another agent in the same tenant has 49,980 newer rows that match the OR. Step 2 path: `agent-nested-loop`.

`Nested Loop`: `Index Scan using idx_sessions_tenant` for the agent, then `Index Scan using idx_memories_session` with `Index Cond: (session_id = sessions.id)`, 16 rows. Execution 0.044 ms.

Samples: 10.551416 ms, 10.884709 ms, 10.449826 ms, 10.134619 ms, 10.4017 ms. Median **10.449826 ms**. Max **10.884709 ms**.
