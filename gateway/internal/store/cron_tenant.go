// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import "fmt"

// cronTenantRolloutStmts is the production Postgres step for SPEC 183.
// migratePostgres does not run it. CREATE INDEX CONCURRENTLY cannot run in
// the startup path, and the backfill waits for Dat's approval.
// Operators run docs/qa/183-cron-per-tenant/migration.sql after the column
// exists. SQLite backfills and creates the same keys at open; it has no
// CONCURRENTLY.
var cronTenantRolloutStmts = []string{
	`UPDATE cron_jobs AS c
SET tenant_id = s.tenant_id
FROM sessions AS s
WHERE c.session_id = s.id
  AND c.tenant_id IS DISTINCT FROM s.tenant_id`,
	`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_cron_jobs_tenant
    ON cron_jobs (tenant_id)`,
	`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_cron_jobs_session
    ON cron_jobs (session_id)`,
}

// migrateCronTenant adds tenant_id on an upgraded SQLite file, copies
// sessions.tenant_id onto existing jobs, and creates the tenant and session
// indexes. Fresh files already have the column from CREATE TABLE.
// Postgres must not call this: its backfill and indexes are cronTenantRolloutStmts.
func (s *SQLiteStore) migrateCronTenant() error {
	if s.pg {
		return fmt.Errorf("postgres cron tenant backfill is not run at startup")
	}
	has, err := s.hasColumn("cron_jobs", "tenant_id")
	if err != nil {
		return fmt.Errorf("cron_jobs.tenant_id check: %w", err)
	}
	if !has {
		if _, err := s.db.Exec(`ALTER TABLE cron_jobs ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`); err != nil {
			return fmt.Errorf("cron_jobs.tenant_id add: %w", err)
		}
	}
	// Only rows whose session tenant differs are rewritten. A job with no
	// session keeps the column default.
	if _, err := s.db.Exec(`
		UPDATE cron_jobs
		SET tenant_id = (SELECT sessions.tenant_id FROM sessions WHERE sessions.id = cron_jobs.session_id)
		WHERE EXISTS (
			SELECT 1 FROM sessions
			WHERE sessions.id = cron_jobs.session_id
			  AND sessions.tenant_id != cron_jobs.tenant_id
		)`); err != nil {
		return fmt.Errorf("cron_jobs.tenant_id backfill: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_cron_jobs_tenant ON cron_jobs(tenant_id)`); err != nil {
		return fmt.Errorf("idx_cron_jobs_tenant: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_cron_jobs_session ON cron_jobs(session_id)`); err != nil {
		return fmt.Errorf("idx_cron_jobs_session: %w", err)
	}
	return nil
}
