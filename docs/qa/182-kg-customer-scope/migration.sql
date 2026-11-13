-- SPEC 182: Postgres indexes for customer-scoped knowledge graph.
--
-- Do NOT run this from gateway startup. migratePostgres only adds
-- customer_key (TEXT NOT NULL DEFAULT '') on kg_entities and kg_relations.
-- That ALTER is add-only: it does not UPDATE or DELETE existing rows.
-- Existing rows read as customer_key ''.
--
-- CREATE INDEX CONCURRENTLY cannot run inside a transaction block, and it
-- must not run while the gateway holds a migration transaction. Run each
-- statement on its own, with autocommit, after the column exists.
--
-- Production: this step waits for Dat's approval. Take a backup, count rows
-- per tenant, and use an off-hours window. Do not deploy this index as part
-- of the binary startup.
--
-- Order:
--   1. Backup.
--   2. Deploy the binary. Startup adds the column and does not build these
--      indexes. Reads and writes are correct without the indexes; the indexes
--      only keep a large tenant from scanning kg_entities / kg_relations.
--   3. After Dat approves, run this file.
--   4. Rollback of the index, if needed, is DROP INDEX CONCURRENTLY. Leave
--      the column in place. A binary that reads customer_key cannot start
--      against a database that does not have the column.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_kg_entities_customer
    ON kg_entities (tenant_id, agent_id, customer_key);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_kg_relations_customer
    ON kg_relations (tenant_id, customer_key);
