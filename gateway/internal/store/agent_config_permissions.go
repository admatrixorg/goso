// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"strings"
	"time"
)

// AgentConfigPermission is one allow or deny for an agent writing a config
// surface, optionally narrowed to one user and one chat scope.
type AgentConfigPermission struct {
	ID         string `json:"id"`
	TenantID   string `json:"-"`
	AgentID    string `json:"agent_id"`
	Scope      string `json:"scope"`
	ConfigType string `json:"config_type"`
	UserID     string `json:"user_id,omitempty"`
	Permission string `json:"permission"`
	GrantedBy  string `json:"granted_by,omitempty"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// AgentConfigPermissionStore is the config write permission capability.
type AgentConfigPermissionStore interface {
	ListAgentConfigPermissions(tenant, agentID string) []*AgentConfigPermission
	PutAgentConfigPermission(AgentConfigPermission) (*AgentConfigPermission, error)
	DeleteAgentConfigPermission(tenant, id string) error
}

// AgentConfigPermissions returns the capability of st, or nil.
func AgentConfigPermissions(st StoreIface) AgentConfigPermissionStore {
	if x, ok := st.(AgentConfigPermissionStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migrateAgentConfigPermissions() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS agent_config_permissions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			agent_id TEXT NOT NULL,
			scope TEXT NOT NULL DEFAULT '*',
			config_type TEXT NOT NULL DEFAULT '*',
			user_id TEXT NOT NULL DEFAULT '',
			permission TEXT NOT NULL DEFAULT 'allow',
			granted_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_agent_config_perm ON agent_config_permissions(tenant_id, agent_id, scope, config_type, user_id)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

const agentConfigPermColumns = "id, tenant_id, agent_id, scope, config_type, user_id, permission, granted_by, created_at, updated_at"

func (s *SQLiteStore) ListAgentConfigPermissions(tenant, agentID string) []*AgentConfigPermission {
	q := "SELECT " + agentConfigPermColumns + " FROM agent_config_permissions WHERE tenant_id=?"
	args := []any{NormalizeTenant(tenant)}
	if strings.TrimSpace(agentID) != "" {
		q += " AND agent_id=?"
		args = append(args, strings.TrimSpace(agentID))
	}
	q += " ORDER BY config_type, scope"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return []*AgentConfigPermission{}
	}
	defer rows.Close()
	out := []*AgentConfigPermission{}
	for rows.Next() {
		var p AgentConfigPermission
		if rows.Scan(&p.ID, &p.TenantID, &p.AgentID, &p.Scope, &p.ConfigType, &p.UserID,
			&p.Permission, &p.GrantedBy, &p.CreatedAt, &p.UpdatedAt) != nil {
			continue
		}
		out = append(out, &p)
	}
	return out
}

func (s *SQLiteStore) PutAgentConfigPermission(p AgentConfigPermission) (*AgentConfigPermission, error) {
	p.TenantID = NormalizeTenant(p.TenantID)
	p.AgentID = strings.TrimSpace(p.AgentID)
	if p.AgentID == "" {
		return nil, ErrNotFound
	}
	if strings.TrimSpace(p.Scope) == "" {
		p.Scope = "*"
	}
	if strings.TrimSpace(p.ConfigType) == "" {
		p.ConfigType = "*"
	}
	if strings.TrimSpace(p.Permission) == "" {
		p.Permission = "allow"
	}
	now := formatTime(time.Now().UTC())
	if p.ID == "" {
		p.ID = "acp_" + newID()
		p.CreatedAt = now
		p.UpdatedAt = now
	}
	_, err := s.db.Exec(`INSERT INTO agent_config_permissions(`+agentConfigPermColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(tenant_id, agent_id, scope, config_type, user_id)
		DO UPDATE SET permission=excluded.permission, granted_by=excluded.granted_by, updated_at=excluded.updated_at`,
		p.ID, p.TenantID, p.AgentID, p.Scope, p.ConfigType, p.UserID, p.Permission,
		p.GrantedBy, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *SQLiteStore) DeleteAgentConfigPermission(tenant, id string) error {
	res, err := s.db.Exec("DELETE FROM agent_config_permissions WHERE id=? AND tenant_id=?",
		strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
