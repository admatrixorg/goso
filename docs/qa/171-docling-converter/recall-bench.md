# SPEC 171 — recall paths after merging main `722334d`

SPEC 171 does not add or change a recall SQL path. The merge keeps PR #44's step-2 plans, `idx_memories_customer_created`, `recallPlanCeiling`, `recallWallClockReason`, and `TestPostgresRecallORScopeIsolation`. CI still fails a shape when the path label is wrong, when `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` shows a `Seq Scan` on `memories` or misses the expected index, when rows or shared blocks exceed the ceiling, or when `fetched` exceeds `recallCandidateCap` (2000). The 50 ms check runs only when `GOSO_RECALL_BENCH=1`, and only after those plan checks. The threshold is unchanged.

This run is the agent VM, not VPS2. Machine: 4 CPUs, Intel Xeon, 16 GiB RAM. PostgreSQL **16.15** (Ubuntu `16.15-0ubuntu0.24.04.1`). The DSN is `GOSO_TEST_POSTGRES_DSN`. The value is not stored here.

`GOSO_RECALL_BENCH=1 go test ./gateway/internal/store/ ./gateway/internal/pipeline/ -run 'TestPostgresRecall50k|TestPostgresRecallBitmapAndCustomerPlan|TestRecallFor50kUnder50ms' -count=1` exited 0. Twenty samples per timed shape. Median is the value the test prints (upper middle of the sorted samples). p95 is nearest-rank. Every max is under 50 ms. Raw log: [recall-bench-20.txt](recall-bench-20.txt).

| Shape | Path | Index checked | Measured rows | Row ceiling | Measured blocks | Block ceiling | Median | p95 | Max |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|
| Selective | `bitmap-and` | `idx_memories_body_tsv` | 1 | 2 | 35 | 212 | 1.432 ms | 1.618 ms | 1.735 ms |
| About 5% | `bitmap-and` | `idx_memories_body_tsv` | 2,501 | 3,752 | 2,457 | 3,686 | 26.301 ms | 37.599 ms | 40.917 ms |
| All match | `tenant-window` | `idx_memories_tenant_created` | 3,998 | 5,997 | 132 | 198 | 15.513 ms | 16.262 ms | 16.373 ms |
| Other tenants | `bitmap-and` | `idx_memories_customer`, `idx_memories_session` | 50,000 | 75,000 | 2,159 | 3,300 | 23.094 ms | 24.087 ms | 24.923 ms |
| Customer, 5 rows | `customer-index` | `idx_memories_customer_created` | 5 | 8 | 5 | 8 | 0.409 ms | 0.811 ms | 0.839 ms |
| Agent, 16 rows | `agent-nested-loop` | `idx_memories_session` | 16 | 24 | 7 | 11 | 9.414 ms | 10.581 ms | 11.387 ms |
| Agent swamp | `agent-window` | `idx_memories_session` | 5,000 | 7,500 | 167 | 251 | 20.404 ms | 22.886 ms | 23.543 ms |
| Customer swamp | `customer-window` | `idx_memories_customer_created` | 2,000 | 3,000 | 76 | 114 | 17.187 ms | 20.201 ms | 20.424 ms |
| Customer bitmap fallback | `bitmap-and-customer` | `idx_memories_session` | 14 | 21 | 18 | 27 | — | — | — |
| `RecallFor` 50k | `bitmap-and` | plan gate in the pipeline test | — | — | — | — | 2.732 ms | 3.832 ms | 6.823 ms |

`bitmap-and-customer` is the plan-and-isolation check from #44. That test does not take 20 wall-clock samples. No plan in this run was a `Seq Scan` on `memories`. Ceilings are the #44 values (1.5 times that PR's base). They were not raised.

Without `GOSO_RECALL_BENCH`, the same store tests pass the plan checks and skip the 50 ms samples. Log: [recall-plan-gate.txt](recall-plan-gate.txt).
