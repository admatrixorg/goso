# SPEC 182 — customer-scoped knowledge graph

Add-only `customer_key TEXT NOT NULL DEFAULT ''` on `kg_entities` and `kg_relations` for the in-memory store, SQLite, and Postgres. No existing row is updated or deleted. Rows that already exist stay shared (`''`).

A write from a channel session stamps `customer_key` with `CustomerKeyForLabel` of that session (the same rule as `memories`). The caller cannot set the key. A web session, or a write with no session, stores `''`.

## Reads

Web and API calls with no channel session see the whole tenant, the same as memory recall with `CustomerScoped` false.

A channel caller sees a row when `customer_key = ''` OR `customer_key` equals that customer. Another customer's entities and relations are hidden. A relation is also hidden when either endpoint belongs to another customer, so a shared edge cannot name that entity.

`RecallFor` / `RecallMemories` do not read the knowledge graph. Their SQL, plan labels, and the #44 row/buffer ceilings are unchanged.

Paths that apply the filter:

| Path | Where |
|---|---|
| Search | `SearchProgressive` L2 on the memory store, SQLite `instr` and FTS5, and Postgres `body_tsv` |
| Neighbours | `ListKGRelations`, `ExpandKG`, `ListKGGraph` / `BuildKGGraph` |
| Tools | `memory_search`, `memory_expand` (`DispatchMemoryTool`, `hitVisible`) |
| API | `GET /api/kg/search`, `GET /api/kg/entities/{id}`, `GET /api/kg/graph` when `session_id` is a channel session |

`GOSO_KG_EXTRACT` stays off. When it is on, a channel reply stamps the extracted entity with that session's customer. Web extraction stays shared.

## Deploy order

Postgres startup runs only:

```sql
ALTER TABLE kg_entities ADD COLUMN IF NOT EXISTS customer_key TEXT NOT NULL DEFAULT '';
ALTER TABLE kg_relations ADD COLUMN IF NOT EXISTS customer_key TEXT NOT NULL DEFAULT '';
```

It does **not** create `idx_kg_entities_customer` or `idx_kg_relations_customer`. Those are `CREATE INDEX CONCURRENTLY` in [migration.sql](migration.sql). `CONCURRENTLY` cannot run inside the startup migration. SQLite creates the same keys at open; SQLite has no `CONCURRENTLY` and the file is local.

Production order:

1. Backup the database.
2. Deploy this binary. Startup adds the column. That is enough for correct reads and writes.
3. Stop here unless Dat has approved the index build. Do not run [migration.sql](migration.sql) in this deploy.
4. After approval, off-hours, with autocommit (not inside a transaction), run [migration.sql](migration.sql). The column must already exist.
5. Index rollback is `DROP INDEX CONCURRENTLY`. Leave the column. A binary that selects `customer_key` will not start if the column is missing.

Sample rows in tests are Minh An fixtures only. No pgvector change. No workflow change.
