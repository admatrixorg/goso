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
3. Recall and search on Postgres use the index. They do not use “newest 500 rows” as the candidate set. Postgres recall loads at most `recallCandidateCap` rows (`2000` in `gateway/internal/store`), ordered by `ts_rank_cd(body_tsv, query) DESC, created_at DESC`. Token-overlap scoring still runs in Go on that set. SQLite’s FTS5 path is unchanged in this spec (diacritics are SPEC 155).
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

Recall on Postgres matches `body_tsv @@ to_tsquery('simple', …)`. The SQL is:

```sql
ORDER BY ts_rank_cd(m.body_tsv, q.query) DESC, m.created_at DESC
LIMIT 2000
```

`2000` is the named constant `recallCandidateCap`. It is not `recallScanCap` (500). A note older than 500 rows is still in the candidate set when its `ts_rank_cd` keeps it inside the 2000. Go then scores those rows by token overlap, same as before, so admission is unchanged on the bench. SQLite still uses FTS5. `unaccent` is not installed.

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
| 50,000 rows that all match one query token stay inside `recallCandidateCap`, and the needle is still recalled | `TestPostgresRecall50kAllMatchWithinCap` |

`TestPostgresRecall50kAllMatchWithinCap` also times that query. On Postgres 16.15, commit `484042b`, the five samples were 436.450566ms, 455.045362ms, 459.285839ms, 463.110639ms, and 446.988949ms. Median 455.045362ms. Max 463.110639ms. Fetched rows were 2000, the cap, and the older needle was in the result. That max is over 50 ms. The selective 50,000-row case (fillers that do not share a query token) stays under 50 ms. The all-match number is the measurement; it is not replaced with the selective number. The same lines are in `docs/qa/154-postgres-tsvector-gin/`.
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
```

After the indexes are gone, this code still reads `body_tsv`, so a note older than 500 rows is still returned; Postgres scans the column instead of using the index. Restoring the newest-500 scan means reverting the query code in this spec. The generated columns may be dropped only after that revert:

```sql
ALTER TABLE memories DROP COLUMN IF EXISTS body_tsv;
ALTER TABLE vault_docs DROP COLUMN IF EXISTS body_tsv;
ALTER TABLE kg_entities DROP COLUMN IF EXISTS body_tsv;
```
