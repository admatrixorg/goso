# SPEC 183 — Per-tenant cron isolation

> Approved by CTO. One PR to `main`. Runtime only.
> GoClaw cron import stays in PR #47. Do not copy GoClaw or Dewee.
> Sample data is Minh An only: tenant `minh-an-sales`, display name `Cố vấn Minh An`.

`cron_jobs` was a single global list with one cap of 20. A request could create, list, patch, or delete a job for a session in another tenant. This spec isolates jobs by the session's tenant.

## Tenant source

The tenant is `sessions.tenant_id`. A session's tenant is fixed at creation.

`POST /api/cron` does not accept `tenant_id`. The handler struct has no such field, so a client-supplied `tenant_id` is discarded. `CreateCronJob` copies the tenant from the session row and ignores `CronJob.TenantID` on the argument.

On create, the handler calls `sessionVisible(st, sid, tid)` (`gateway/internal/httpapi/tenant.go`). A missing session and a session in another tenant both return `store.ErrNotFound`, and the handler answers **HTTP 404**. It does not answer 403.

## Schema

`cron_jobs.tenant_id TEXT NOT NULL DEFAULT 'default'` on the memory store, SQLite, and Postgres.

- **Memory.** There is no durable migration. `CreateCronJob` stamps `TenantID` from the session.
- **SQLite.** Fresh `CREATE TABLE` includes the column. An upgraded file gets `ALTER TABLE ... ADD COLUMN` at open, then a backfill from `sessions.tenant_id`, then ordinary indexes. SQLite has no `CONCURRENTLY`. Jobs whose session is already gone stay `'default'`.
- **Postgres startup is add-only.** `migratePostgres` runs only:

```sql
ALTER TABLE cron_jobs ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
```

It does not `UPDATE`, `DELETE`, or `CREATE INDEX`. Existing rows read as `'default'` until the rollout file runs. New writes stamp the session tenant immediately, so they do not wait for that file.

## Production rollout

Backfill and indexes for production Postgres are in [docs/qa/183-cron-per-tenant/migration.sql](../../docs/qa/183-cron-per-tenant/migration.sql). `CREATE INDEX CONCURRENTLY` cannot run inside the startup migration. Do not execute that file from the gateway.

Indexes:

| Name | Key |
|---|---|
| `idx_cron_jobs_tenant` | `(tenant_id)` — list and the per-tenant cap |
| `idx_cron_jobs_session` | `(session_id)` — delete jobs with the session |

SQLite creates the same keys at open.

## Cap

`CronJobCap` stays 20 and is counted **per tenant**, the same shape as `LiteMaxAgents` (count rows with that `tenant_id`, then reject). Two tenants may each hold 20 jobs. Job 21 in a tenant returns `store.ErrCronCap`. `POST /api/cron` answers 400 `cron cap: max 20 jobs per tenant`.

## Runtime

| Path | Scope |
|---|---|
| `ListCronJobs()` | Every tenant. `cron.Tick` keeps using this so a non-default tenant's jobs still fire. |
| `ListCronJobsTenant(tid)` | `GET /api/cron` and `GET /v1/cron`. Other tenants' jobs are absent. The collection itself is 200. |
| `PATCH /api/cron/{id}` | Load the job. 404 unless `job.tenant_id` matches the request tenant **and** `sessionVisible` accepts the job's session. |
| `DELETE /api/cron/{id}` | Same check, then delete. 404 on a cross-tenant id. |
| `DeleteSession` | Also deletes `cron_jobs` for that session (memory, SQLite, and Postgres, which embeds the SQLite method). |

`billing.DayLimit` inside `FireSessionChat` is unchanged.

## Tests

Memory, SQLite, and Postgres (`GOSO_TEST_POSTGRES_DSN` skips when unset):

- Cross-tenant create, patch, and delete return 404 and do not change the job. List omits the other tenant.
- A body `tenant_id` does not choose the tenant.
- Cap is 20 per tenant.
- `cron.Tick` fires due jobs from more than one tenant.
- `DeleteSession` removes that session's jobs and leaves another session's job in place.
- Postgres startup adds the column, does not backfill, and does not create the concurrent indexes. The rollout statements do.

## Out of scope

- GoClaw cron import (PR #47).
- `billing.DayLimit` and any quota change.
- Control-plane copy or layout.
- `.github/`.
