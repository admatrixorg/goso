// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import "errors"

// ErrCrossTenantLink is returned when a link would join agents of two
// different tenants. Links never cross a tenant.
var ErrCrossTenantLink = errors.New("agents belong to different tenants")

// migrateAgentLinkTenant adds agent_links.tenant_id (the from agent's
// tenant) and fills it for rows written before the column existed. The
// table holds a handful of rows per agent, so the backfill is cheap on both
// SQLite and Postgres. Rows whose from agent is gone stay empty.
func (s *SQLiteStore) migrateAgentLinkTenant() error {
	if s.pg {
		if _, err := s.db.Exec(`ALTER TABLE agent_links ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	} else {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('agent_links') WHERE name='tenant_id'`).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := s.db.Exec(`ALTER TABLE agent_links ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
		}
	}
	if _, err := s.db.Exec(`UPDATE agent_links SET tenant_id = COALESCE((SELECT a.tenant_id FROM agents a WHERE a.id = agent_links.from_agent_id), '') WHERE tenant_id = ''`); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_links_tenant ON agent_links(tenant_id)`)
	return err
}
