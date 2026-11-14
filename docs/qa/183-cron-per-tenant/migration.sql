-- SPEC 183: backfill cron_jobs.tenant_id and build its indexes.
--
-- Do NOT run this from gateway startup. migratePostgres only adds
-- tenant_id (TEXT NOT NULL DEFAULT 'default') on cron_jobs.
-- That ALTER is add-only: it does not UPDATE or DELETE existing rows.
-- Existing rows read as tenant_id 'default' until this file runs.
-- New jobs written by this binary already store the session tenant.
--
-- CREATE INDEX CONCURRENTLY cannot run inside a transaction block, and it
-- must not run while the gateway holds a migration transaction. Run each
-- statement on its own, with autocommit, after the column exists.
--
-- The backfill copies sessions.tenant_id. A job whose session is already
-- gone stays 'default'. This file does not delete those rows.
--
-- Production: this step waits for Dat's approval. Take a backup, count
-- cron_jobs per tenant, and use an off-hours window. Do not deploy this
-- file as part of the binary startup. Until it runs, a job that predates
-- the column is visible to the default tenant and is hidden from the
-- session's real tenant. The ticker still runs it (ListCronJobs is global).
--
-- Order:
--   1. Backup.
--   2. Deploy the binary. Startup adds the column and does not backfill
--      or build these indexes.
--   3. After Dat approves, run each statement below on its own.
--   4. Rollback of an index, if needed, is DROP INDEX CONCURRENTLY. Leave
--      the column in place. A binary that reads tenant_id cannot start
--      against a database that does not have the column.

UPDATE cron_jobs AS c
SET tenant_id = s.tenant_id
FROM sessions AS s
WHERE c.session_id = s.id
  AND c.tenant_id IS DISTINCT FROM s.tenant_id;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_cron_jobs_tenant
    ON cron_jobs (tenant_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_cron_jobs_session
    ON cron_jobs (session_id);
