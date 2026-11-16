// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"fmt"
	"strings"
)

// agentKeyScopeIndex is the uniqueness key for agents: one agent_key per
// tenant. Two tenants may each hold the same key (audit N9, 16/11).
const agentKeyScopeIndex = `CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_tenant_key ON agents(tenant_id, agent_key)`

// agentKeyMigrateFault, when set, aborts migrateAgentKeyScope at a named
// step. Tests use it to prove the migration is transactional.
var agentKeyMigrateFault func(step string) error

// FindAgentByKey returns the agent with this key in this tenant, or nil.
// Every lookup by agent_key goes through the tenant: the same key in
// another tenant is a different agent and is never returned.
func FindAgentByKey(st StoreIface, tenant, key string) *Agent {
	key = strings.TrimSpace(key)
	if st == nil || key == "" {
		return nil
	}
	for _, a := range st.ListAgents() {
		if a != nil && a.AgentKey == key && SameTenant(a.TenantID, tenant) {
			return a
		}
	}
	return nil
}

// migrateAgentKeyScope replaces a machine-wide UNIQUE(agent_key) with
// UNIQUE(tenant_id, agent_key). Rows are unchanged: keys were globally
// unique before, so they are unique per tenant too. Postgres uses the same
// method from migratePostgres.
func (s *SQLiteStore) migrateAgentKeyScope() error {
	if s.pg {
		return s.migrateAgentKeyScopePostgres()
	}
	type uidx struct {
		name   string
		origin string
	}
	rows, err := s.db.Query(`SELECT name, origin FROM pragma_index_list('agents') WHERE "unique"=1`)
	if err != nil {
		return err
	}
	var found []uidx
	for rows.Next() {
		var u uidx
		if err := rows.Scan(&u.name, &u.origin); err != nil {
			rows.Close()
			return err
		}
		found = append(found, u)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rebuild := false
	var dropIdx []string
	for _, u := range found {
		cols, err := s.sqliteIndexColumns(u.name)
		if err != nil {
			return err
		}
		if len(cols) != 1 || cols[0] != "agent_key" {
			continue
		}
		if u.origin == "c" {
			dropIdx = append(dropIdx, u.name)
		} else {
			// A column or table UNIQUE constraint lives in the table
			// definition (sqlite_autoindex_*) and cannot be dropped alone.
			rebuild = true
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, name := range dropIdx {
		if _, err := tx.Exec(`DROP INDEX IF EXISTS ` + quoteIdent(name)); err != nil {
			return err
		}
	}
	if rebuild {
		if _, err := tx.Exec(`CREATE TABLE agents_scoped (
			id TEXT PRIMARY KEY,
			agent_key TEXT NOT NULL,
			display_name TEXT,
			model TEXT,
			llm_provider TEXT,
			created_at TEXT NOT NULL,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			enabled INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT '',
			instructions TEXT,
			orchestration_mode TEXT
		)`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO agents_scoped(id, agent_key, display_name, model, llm_provider, created_at, tenant_id, enabled, updated_at, instructions, orchestration_mode)
			SELECT id, agent_key, display_name, model, llm_provider, created_at, COALESCE(NULLIF(tenant_id, ''), 'default'), enabled, updated_at, instructions, orchestration_mode FROM agents`); err != nil {
			return err
		}
		if agentKeyMigrateFault != nil {
			if err := agentKeyMigrateFault("copied"); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`DROP TABLE agents`); err != nil {
			return err
		}
		if _, err := tx.Exec(`ALTER TABLE agents_scoped RENAME TO agents`); err != nil {
			return err
		}
		if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_tenant ON agents(tenant_id)`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(agentKeyScopeIndex); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) sqliteIndexColumns(index string) ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM pragma_index_info(?) ORDER BY seqno`, index)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c *string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		if c == nil {
			cols = append(cols, "")
			continue
		}
		cols = append(cols, *c)
	}
	return cols, rows.Err()
}

// migrateAgentKeyScopePostgres looks up the real names of any UNIQUE
// constraint or standalone unique index on agents(agent_key) alone, drops
// them, and adds UNIQUE(tenant_id, agent_key). A database created by an
// older build has the constraint agents_agent_key_key; nothing assumes that.
func (s *SQLiteStore) migrateAgentKeyScopePostgres() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	cons, err := pgNames(tx, `SELECT con.conname
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid = con.conrelid
		JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
		WHERE nsp.nspname = current_schema()
		  AND rel.relname = 'agents'
		  AND con.contype = 'u'
		  AND array_length(con.conkey, 1) = 1
		  AND con.conkey[1] = (SELECT a.attnum FROM pg_attribute a WHERE a.attrelid = rel.oid AND a.attname = 'agent_key')`)
	if err != nil {
		return err
	}
	for _, name := range cons {
		if _, err := tx.Exec(`ALTER TABLE agents DROP CONSTRAINT IF EXISTS ` + quoteIdent(name)); err != nil {
			return fmt.Errorf("drop constraint %s: %w", name, err)
		}
	}
	if agentKeyMigrateFault != nil {
		if err := agentKeyMigrateFault("postgres-dropped"); err != nil {
			return err
		}
	}
	idx, err := pgNames(tx, `SELECT ic.relname
		FROM pg_index i
		JOIN pg_class ic ON ic.oid = i.indexrelid
		JOIN pg_class tc ON tc.oid = i.indrelid
		JOIN pg_namespace nsp ON nsp.oid = tc.relnamespace
		WHERE nsp.nspname = current_schema()
		  AND tc.relname = 'agents'
		  AND i.indisunique AND NOT i.indisprimary
		  AND i.indnatts = 1
		  AND i.indkey[0] = (SELECT a.attnum FROM pg_attribute a WHERE a.attrelid = tc.oid AND a.attname = 'agent_key')
		  AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conindid = i.indexrelid)`)
	if err != nil {
		return err
	}
	for _, name := range idx {
		if _, err := tx.Exec(`DROP INDEX IF EXISTS ` + quoteIdent(name)); err != nil {
			return fmt.Errorf("drop index %s: %w", name, err)
		}
	}
	if _, err := tx.Exec(agentKeyScopeIndex); err != nil {
		return err
	}
	return tx.Commit()
}

func pgNames(tx txHandle, q string) ([]string, error) {
	rows, err := tx.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// quoteIdent double-quotes a catalog name for DDL.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
