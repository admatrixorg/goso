# SPEC 154 — Postgres tsvector and GIN

Status: implemented on this branch (MQG-231). Sample data is Minh An only, the same fixtures as SPEC 153. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 154. Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136. Old 134 and 137 are SPEC 160, 161, and 164, not this file.

## Why

On Postgres, recall and memory search stop at the newest rows (`MemoryQueryCap` and `recallScanCap` are 500 in `gateway/internal/store`). A matching Minh An note older than that window is invisible. SQLite already uses FTS5. This spec gives Postgres an index without rewriting existing columns.

## Behavior

1. Add a `tsvector` column, maintained from the existing body, on `memories.body`, `vault_docs.body`, and `kg_entities.body`. Add a GIN index on each new column. The text search configuration is `simple`.
2. The migration only adds columns and indexes. It does not change the type, default, or contents of a column that already exists on `9545d08`.
3. Recall and search on Postgres use the index. They do not use “newest 500 rows” as the candidate set. Postgres recall loads at most `recallCandidateCap` rows (`2000` in `gateway/internal/store`): an AND of the tokens first, then an OR of the same tokens only when the AND returned fewer rows than the caller asked for. Both steps order by `created_at DESC`. There is no `ts_rank` in SQL. Token-overlap scoring still runs in Go. SQLite’s FTS5 path is unchanged in this spec (diacritics are SPEC 155).
4. The 50,000-row check inserts synthetic rows that repeat Minh An demo sentences. It does not import an external corpus.
5. Run SPEC 153. The Postgres score equals the SQLite score. On this branch both engines stay at the `9545d08` floor, 20/30, with the same per-group scores. `bench/memory-vi/baseline-9545d08.json` is not edited. `tv-03` stays a Postgres miss because `unaccent` is SPEC 155.

## Gate

- **Invariant.** Only add columns. Do not modify old columns.
- **Acceptance.** Postgres matches SQLite on the SPEC 153 bench. A 50,000-row search finishes in under 50 ms.
- **Negative test.** A matching note older than 500 rows is still recalled.
- **Rollback.** Drop the new GIN indexes and stop querying the new columns. Do not drop or alter a column that existed before this spec.

## Implementation

The migration in `gateway/internal/store/postgres_tsvector.go` adds, and does not rewrite, these columns:

| Table | Column | Expression | Index |
|---|---|---|---|
| `memories` | `body_tsv` | `to_tsvector('simple', coalesce(body, ''))` STORED | `idx_memories_body_tsv` GIN |
| `vault_docs` | `body_tsv` | same | `idx_vault_docs_body_tsv` GIN |
| `kg_entities` | `body_tsv` | same | `idx_kg_entities_body_tsv` GIN |
| `memories` | (no new column) | existing `tenant_id`, `created_at` | `idx_memories_tenant_created` btree `(tenant_id, created_at DESC)` |

Recall does not use `ts_rank` or `ts_rank_cd`. Scoring stays in Go (`lexicalScore`). Postgres loads candidates in two steps, each `ORDER BY created_at DESC`, and never more than `recallCandidateCap` (2000) rows together:

1. `body_tsv @@ to_tsquery('simple', 't1 & t2 & …')` with tokens kept only by `safeTSToken`. Same tenant, agent, and customer scope as before. `LIMIT recallCandidateCap`.
2. Run only when step 1 returned fewer than the caller's `limit`. Same OR tokens, same scope, same order, `LIMIT` the unused part of the cap. Rows already returned by step 1 are dropped. Step 2 does not leave the plan to the row estimate. A customer with fewer memories than the probe budget is read through `idx_memories_customer`. An agent with fewer memories than that budget is read through `idx_memories_session` from that agent's sessions. Otherwise it reads at most twice the limit of the newest rows through `idx_memories_tenant_created` (`SET LOCAL enable_seqscan = off`). If that window already holds enough matches, those are the candidates. If it does not, the window is discarded and a second statement loads the newest matches for that tenant: `tenant_id = ?` and `body_tsv @@`, with sequential scans and plain index scans disabled and `cpu_tuple_cost = 1`, so a polluted GIN bitmap is intersected with the tenant btree instead of heap-fetching every tenant's matches.

`idx_memories_tenant` is only `(tenant_id)`. `idx_memories_session` is `(session_id, created_at)`. `idx_memories_customer` is `(tenant_id, customer_key)`. None of those is `(tenant_id, created_at)`, which is the pair this query always filters and sorts. Agent and customer filters stay in the WHERE because they are optional. SQLite still uses FTS5. `unaccent` is not installed.

Search of those three bodies uses `phraseto_tsquery('simple', …)`, which is a whole word or phrase. It is not the old Postgres substring (`instr` / `strpos`) on the body. Typing `Min` does not find `Minh`. Typing `Minh` does. `TestPostgresSearchMinDoesNotMatchMinh` locks that. Message text, vault titles, and entity names stay on `strpos`, because this spec does not add a column for them. A vault title can still match a substring.

These search queries already have a limit: memory search 50, vault search `VaultSearchLimit` (50), progressive L1 and L2 `progressivePerTier` (25). None of them loads an unbounded body match into Go.

## Follow-up

`SearchMemory` still has no tenant filter. That was true before this spec, on substring search and on SQLite FTS5. This spec does not add one. A later change should scope memory search by tenant.

## Tests

| Gate | Test |
|---|---|
| Invariant | `TestPostgresBodyTSVAdditive`, `TestSQLiteHasNoBodyTSV` |
| Backfill of an existing body | `TestPostgresBodyTSVBackfillAndSearch` |
| Negative: note older than `recallScanCap` | `TestRecallOlderThanScanCap` (store) and `TestRecallForOlderThanScanCap` (pipeline) |
| 50,000 rows under 50 ms | `TestPostgresRecall50kUnder50ms` (store `RecallMemories`) and `TestRecallFor50kUnder50ms` (pipeline `RecallFor`) |
| 50,000 rows that all match one query token stay inside `recallCandidateCap`, and the needle is still recalled, in under 50 ms | `TestPostgresRecall50kAllMatchWithinCap` |
| 50,000 rows, about 5% matching the OR, stay under 50 ms | `TestPostgresRecall50kFivePercentWithinCap` |
| Oldest note that contains every query token is recalled when 50,000 newer rows share only one token | `TestPostgresRecallOldestFullMatch` |
| `Min` does not match `Minh` | `TestPostgresSearchMinDoesNotMatchMinh` |
| Bench, Postgres not below SQLite | `gateway/memorybench` `TestBaseline9545d08` |
| Rollback drops the index only | `TestPostgresTSVectorRollbackDropsIndexOnly` |

## Non-goals

`unaccent`, pgvector, a new tokenizer on SQLite, and any change to the prompt. Installing Postgres extensions waits for Dat (SPEC 166). The control plane is unchanged, so prototype screenshots do not apply.

## Rollout

The migration runs by itself when the gateway opens Postgres. There is no feature flag and no way to turn the new columns off after deploy without a code change.

`ALTER TABLE … ADD COLUMN body_tsv tsvector GENERATED ALWAYS AS (…) STORED` rewrites each table and holds a lock that blocks writes for the rewrite. It runs once for `memories`, once for `vault_docs`, and once for `kg_entities`.

`CREATE INDEX` is not `CREATE INDEX CONCURRENTLY`. The index build also blocks writes on that table until it finishes.

Before any deploy, the Freelance manager counts rows in `memories`, `vault_docs`, and `kg_entities` on VPS2. That count is read-only. The manager takes a backup and picks a low-traffic window. Deploy only with Dat's approval.

Rollback after deploy is a code revert, so queries stop reading `body_tsv`, plus the `DROP INDEX` statements below. Dropping the indexes alone leaves the code reading `body_tsv` as a sequential scan. That still finds a note older than 500 rows. It does not restore the old 500-row scan, and it does not turn the feature off.

## Rollback

Drop the new indexes. Do not drop or alter a column that existed before this spec (`memories.body` and the other pre-existing columns stay):

```sql
DROP INDEX IF EXISTS idx_memories_body_tsv;
DROP INDEX IF EXISTS idx_vault_docs_body_tsv;
DROP INDEX IF EXISTS idx_kg_entities_body_tsv;
DROP INDEX IF EXISTS idx_memories_tenant_created;
```

After the indexes are gone, this code still reads `body_tsv`, so a note older than 500 rows is still returned; Postgres scans the column instead of using the index. Restoring the newest-500 scan means reverting the query code in this spec. The generated columns may be dropped only after that revert:

```sql
ALTER TABLE memories DROP COLUMN IF EXISTS body_tsv;
ALTER TABLE vault_docs DROP COLUMN IF EXISTS body_tsv;
ALTER TABLE kg_entities DROP COLUMN IF EXISTS body_tsv;
```
