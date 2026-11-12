# SPEC 166 — Vector search

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only, the SPEC 153 `tieng-viet` cases, including the query `bao gia` against a note that says `báo giá`. No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. Weights below are starting numbers for the bench, taken from those articles, and the bench may change them before the flag is allowed on. This spec does not copy either codebase. It does not vendor another product’s index.

## Number

Numbering is the table in SPEC 153. This file is 166. A vector row for a fact stores that fact’s `scope` and `owner_id` (SPEC 161). A vector row for a memory or a vault document stores the `scope` and `owner_id` SPEC 164 added, once that spec has shipped. Until then the row stores tenant id, which is enough for the negative test below. This spec does not create the fact columns. They already exist.

Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136. Old 134 and 137 are SPEC 160, 161, and 164.

## Why

Keyword search misses a Minh An paraphrase. Postgres on `9545d08` can add `kg_entities.embedding` when the `pgvector` extension is present, and nothing fills or queries it. SQLite has no vector path. This spec adds both, and it leaves keyword search in place when no embedding provider is configured.

## Behavior

1. Flag `GOSO_MEMORY_VECTOR`. Default off. Off: keyword search only, no error.
2. No embedding provider (`GOSO_EMBEDDING_PROVIDER` unset): keyword search only, no error, whether the flag is on or off.
3. Postgres uses `pgvector` when the extension is already installed. This spec does not install it. Turning the flag on for a tenant that runs Postgres waits until Dat approves installing `pgvector` there. Until that approval, that tenant stays on keyword search.
4. SQLite computes similarity in Go over stored vectors. It does not require `sqlite-vec` or another extension.
5. The dimension is the dimension the configured model returns. It is stored next to the vector. It is not a constant in the schema.
6. `embedding_cache` maps a content hash to a vector so the same Minh An note is not embedded twice.
7. Starting blend, applied after each side is scaled to 0–1, and only while a provider is configured: memory and facts use keyword 0.3 and vector 0.7. A mixed vault result uses vault 0.4, episodic 0.3, and knowledge-graph 0.3. SPEC 153 may replace these three pairs before the flag is approved for production. Replacing them is a change to the bench record, not a second budget.
8. Index `memories`, `memory_facts`, `vault_docs` (split on paragraphs), and `kg_entities.body` on a background queue with retry. `POST /api/memory/reindex` is admin-only.
9. A query is tenant-scoped before similarity. `scope=contact` is compared only to the contact on the turn.
10. The prompt text this spec inserts is still inside the loaded-items row of the SPEC 165 table. This spec does not add a token cap.
11. Production stays off until Dat approves this flag and, for Postgres, the extension install.

## Gate

- **Invariant.** With no provider, search is keyword only.
- **Acceptance.** The SPEC 153 `tieng-viet` group scores at least 85% with the flag on and a provider configured in CI.
- **Negative test.** Another tenant returns no row.
- **Rollback.** Turn `GOSO_MEMORY_VECTOR` off.

## Non-goals

Installing `unaccent` or `pgvector` from this spec. A hard-coded dimension. Reading GoClaw’s index implementation.
