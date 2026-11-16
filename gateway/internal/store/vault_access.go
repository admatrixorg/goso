// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"fmt"
	"strings"
)

// Vault doc scopes. A tenant doc is visible to every agent of the tenant. An
// agent doc only to that agent, a team doc only to agents of that team. A
// personal doc belongs to one agent and is never served in a channel thread.
const (
	VaultScopeTenant   = "tenant"
	VaultScopeAgent    = "agent"
	VaultScopeTeam     = "team"
	VaultScopePersonal = "personal"
)

// ErrVaultScope is returned for an unknown scope or a scope without its owner.
var ErrVaultScope = errors.New("invalid vault scope")

// NormalizeVaultScope maps "" to tenant and rejects unknown values.
func NormalizeVaultScope(scope string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", VaultScopeTenant:
		return VaultScopeTenant, nil
	case VaultScopeAgent:
		return VaultScopeAgent, nil
	case VaultScopeTeam:
		return VaultScopeTeam, nil
	case VaultScopePersonal:
		return VaultScopePersonal, nil
	}
	return "", fmt.Errorf("%w: %q", ErrVaultScope, scope)
}

// NormalizeVaultAccess validates scope fields together. An empty scope with
// agent_id means agent scope, with team_id team scope. Tenant scope carries no
// owner. Agent and team scope require their owner.
func NormalizeVaultAccess(scope, agentID, teamID string) (string, string, string, error) {
	agentID = strings.TrimSpace(agentID)
	teamID = strings.TrimSpace(teamID)
	if strings.TrimSpace(scope) == "" {
		switch {
		case teamID != "" && agentID == "":
			scope = VaultScopeTeam
		case agentID != "" && teamID == "":
			scope = VaultScopeAgent
		case agentID != "" && teamID != "":
			return "", "", "", fmt.Errorf("%w: agent_id and team_id need an explicit scope", ErrVaultScope)
		}
	}
	sc, err := NormalizeVaultScope(scope)
	if err != nil {
		return "", "", "", err
	}
	switch sc {
	case VaultScopeTenant:
		return sc, "", "", nil
	case VaultScopeAgent:
		if agentID == "" {
			return "", "", "", fmt.Errorf("%w: agent scope needs agent_id", ErrVaultScope)
		}
		return sc, agentID, "", nil
	case VaultScopeTeam:
		if teamID == "" {
			return "", "", "", fmt.Errorf("%w: team scope needs team_id", ErrVaultScope)
		}
		return sc, "", teamID, nil
	case VaultScopePersonal:
		if agentID == "" {
			return "", "", "", fmt.Errorf("%w: personal scope needs agent_id", ErrVaultScope)
		}
		return sc, agentID, "", nil
	}
	return "", "", "", ErrVaultScope
}

// VaultViewer is who reads the vault. All is an operator view of the whole
// tenant. Otherwise the viewer is one agent: tenant docs, its own agent and
// personal docs, and docs of teams it belongs to. Channel adds the customer
// rule: no personal docs, and only docs with no customer_key or this thread's.
type VaultViewer struct {
	All         bool
	AgentID     string
	TeamIDs     []string
	Channel     bool
	CustomerKey string
}

// OperatorView sees every served doc of the tenant.
var OperatorView = VaultViewer{All: true}

// Allows reports whether d is visible to v. The caller still checks tenant
// and OwnerExplicit.
func (v VaultViewer) Allows(d *VaultDoc) bool {
	if d == nil {
		return false
	}
	if v.All {
		return true
	}
	scope := d.Scope
	if scope == "" {
		scope = VaultScopeTenant
	}
	if v.Channel {
		if scope == VaultScopePersonal {
			return false
		}
		if d.CustomerKey != "" && d.CustomerKey != v.CustomerKey {
			return false
		}
	}
	switch scope {
	case VaultScopeTenant:
		return true
	case VaultScopeAgent, VaultScopePersonal:
		return v.AgentID != "" && d.AgentID == v.AgentID
	case VaultScopeTeam:
		for _, id := range v.TeamIDs {
			if id != "" && id == d.TeamID {
				return true
			}
		}
		return false
	}
	return false
}

// vaultViewerSQL returns an " AND (...)" predicate on vault_docs columns,
// with col prefix p (e.g. "vault_docs." or "d."). An operator view adds none.
func vaultViewerSQL(p string, v VaultViewer) (string, []any) {
	if v.All {
		return "", nil
	}
	var args []any
	ors := []string{p + "scope IN ('tenant', '')"}
	if v.AgentID != "" {
		ors = append(ors, "("+p+"scope='agent' AND "+p+"agent_id=?)")
		args = append(args, v.AgentID)
		if !v.Channel {
			ors = append(ors, "("+p+"scope='personal' AND "+p+"agent_id=?)")
			args = append(args, v.AgentID)
		}
	}
	var teams []string
	for _, id := range v.TeamIDs {
		if id = strings.TrimSpace(id); id != "" {
			teams = append(teams, id)
		}
	}
	if len(teams) > 0 {
		ors = append(ors, "("+p+"scope='team' AND "+p+"team_id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(teams)), ",")+"))")
		for _, id := range teams {
			args = append(args, id)
		}
	}
	clause := " AND (" + strings.Join(ors, " OR ") + ")"
	if v.Channel {
		clause += " AND " + p + "scope<>'personal' AND (" + p + "customer_key='' OR " + p + "customer_key=?)"
		args = append(args, v.CustomerKey)
	}
	return clause, args
}

// migrateVaultAccess adds scope, agent_id, team_id and customer_key to
// vault_docs and vault_ingest_jobs. Existing rows become tenant scope, which
// is how they were served before. Postgres and SQLite share it.
func (s *SQLiteStore) migrateVaultAccess() error {
	cols := []struct{ name, ddl string }{
		{"scope", "TEXT NOT NULL DEFAULT 'tenant'"},
		{"agent_id", "TEXT NOT NULL DEFAULT ''"},
		{"team_id", "TEXT NOT NULL DEFAULT ''"},
		{"customer_key", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, table := range []string{"vault_docs", "vault_ingest_jobs"} {
		for _, c := range cols {
			if err := s.addColumnIfMissing(table, c.name, c.ddl); err != nil {
				return err
			}
		}
	}
	if _, err := s.db.Exec(`UPDATE vault_docs SET scope='tenant' WHERE scope IS NULL OR scope=''`); err != nil {
		return err
	}
	for _, ddl := range []string{
		`CREATE INDEX IF NOT EXISTS idx_vault_docs_scope ON vault_docs(tenant_id, scope, agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vault_docs_team ON vault_docs(tenant_id, team_id)`,
	} {
		if _, err := s.db.Exec(ddl); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) addColumnIfMissing(table, name, ddl string) error {
	if s.pg {
		if _, err := s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN IF NOT EXISTS ` + name + ` ` + ddl); err != nil {
			return fmt.Errorf("%s.%s: %w", table, name, err)
		}
		return nil
	}
	ok, err := s.hasColumn(table, name)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + name + ` ` + ddl); err != nil {
		return fmt.Errorf("%s.%s: %w", table, name, err)
	}
	return nil
}
