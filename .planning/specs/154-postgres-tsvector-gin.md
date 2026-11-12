# SPEC 154 — Postgres tsvector and GIN

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only, the same fixtures as SPEC 153. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 154. Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136. Old 134 and 137 are SPEC 160, 161, and 164, not this file.

## Why

On Postgres, recall and memory search stop at the newest rows (`MemoryQueryCap` and `recallScanCap` are 500 in `gateway/internal/store`). A matching Minh An note older than that window is invisible. SQLite already uses FTS5. This spec gives Postgres an index without rewriting existing columns.

## Behavior

1. Add a `tsvector` column, maintained from the existing body, on `memories.body`, `vault_docs.body`, and `kg_entities.body`. Add a GIN index on each new column. The text search configuration is `simple`.
2. The migration only adds columns and indexes. It does not change the type, default, or contents of a column that already exists on `9545d08`.
3. Recall and search on Postgres use the index. They do not use “newest 500 rows” as the candidate set. SQLite’s FTS5 path is unchanged in this spec (diacritics are SPEC 155).
4. The 50,000-row check inserts synthetic rows that repeat Minh An demo sentences. It does not import an external corpus.
5. Run SPEC 153. The Postgres score equals the SQLite score.

## Gate

- **Invariant.** Only add columns. Do not modify old columns.
- **Acceptance.** Postgres matches SQLite on the SPEC 153 bench. A 50,000-row search finishes in under 50 ms.
- **Negative test.** A matching note older than 500 rows is still recalled.
- **Rollback.** Drop the new GIN indexes and stop querying the new columns. Do not drop or alter a column that existed before this spec.

## Non-goals

`unaccent`, pgvector, a new tokenizer on SQLite, and any change to the prompt. Installing Postgres extensions waits for Dat (SPEC 166).
