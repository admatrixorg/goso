# SPEC 183 — per-tenant cron

`cron_jobs.tenant_id` is `TEXT NOT NULL DEFAULT 'default'` on the memory store, SQLite, and Postgres. The value is copied from `sessions.tenant_id` when the job is created. The request body cannot set it.

Postgres startup runs only:

```sql
ALTER TABLE cron_jobs ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
```

It does not backfill and it does not create `idx_cron_jobs_tenant` or `idx_cron_jobs_session`. Those statements are in [migration.sql](migration.sql). `CREATE INDEX CONCURRENTLY` cannot run inside the startup migration. SQLite backfills from `sessions` and creates the same keys at open. SQLite has no `CONCURRENTLY` and the file is local.

Production order:

1. Backup the database.
2. Deploy this binary. Startup adds the column. New jobs store the session tenant. Rows that already existed stay `'default'` until the next step.
3. Stop here unless Dat has approved the backfill and the index build. Do not run [migration.sql](migration.sql) in this deploy.
4. After approval, off-hours, with autocommit (not inside a transaction), run each statement in [migration.sql](migration.sql). The column must already exist.
5. Index rollback is `DROP INDEX CONCURRENTLY`. Leave the column. A binary that selects `tenant_id` will not start if the column is missing.

Until step 4, a pre-existing job is listed for the default tenant and is hidden from the session's real tenant. `cron.Tick` still runs it, because the ticker lists every tenant.

Sample rows in tests are Minh An fixtures only (`minh-an-sales`, `Cố vấn Minh An`). `billing.DayLimit` is unchanged. GoClaw cron import stays in PR #47.
