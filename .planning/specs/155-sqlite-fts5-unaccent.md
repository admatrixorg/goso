# SPEC 155 — SQLite FTS5 without diacritics

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only, the same fixtures as SPEC 153. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. This spec does not copy either codebase.

## Number

Numbering is the table in SPEC 153. This file is 155. Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136. Old 134 and 137 are SPEC 160, 161, and 164, not this file.

## Why

A query typed without diacritics does not find the same Minh An note stored with diacritics. SQLite already has FTS5 (`memory_fts`, `vault_fts`, `kg_fts`). This spec rebuilds those indexes so the existing tokenizer drops diacritics.

## Behavior

1. Rebuild the SQLite FTS5 indexes with `unicode61 remove_diacritics 2` on the same columns they index today.
2. The rebuild is idempotent. A second run leaves one index and the same rows. It does not duplicate notes.
3. Tenant predicates stay in the query. The tokenizer change does not widen which tenant is searched.
4. This spec does not install the Postgres `unaccent` extension. That install is a separate Dat approval, and only when a tenant actually runs Postgres.

The acceptance query is the Minh An note whose body contains `báo giá`. The query string is `bao gia`.

## Gate

- **Invariant.** The rebuild is idempotent.
- **Acceptance.** The query `bao gia` finds the note `báo giá`.
- **Negative test.** The same query in another tenant returns no row.
- **Rollback.** Rebuild the FTS5 indexes with the previous tokenizer.

## Non-goals

Postgres `unaccent`, embeddings, and a new memory table.
