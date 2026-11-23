package store

import (
	"strings"
	"time"
)

// MCPGrant is one agent's access to an MCP server.
type MCPGrant struct {
	ID              string `json:"id"`
	TenantID        string `json:"-"`
	ServerID        string `json:"server_id"`
	AgentID         string `json:"agent_id"`
	Enabled         bool   `json:"enabled"`
	ToolAllow       string `json:"tool_allow,omitempty"`
	ToolDeny        string `json:"tool_deny,omitempty"`
	ConfigOverrides string `json:"config_overrides,omitempty"`
	GrantedBy       string `json:"granted_by,omitempty"`
	CreatedAt       string `json:"created_at"`
}

// MCPGrantStore is the MCP access capability.
type MCPGrantStore interface {
	ListMCPGrants(tenant, serverID string) []*MCPGrant
	ListAgentMCPGrants(tenant, agentID string) []*MCPGrant
	PutMCPGrant(MCPGrant) (*MCPGrant, error)
	DeleteMCPGrant(tenant, serverID, agentID string) error
}

// MCPGrants returns the MCP grant capability of st, or nil.
func MCPGrants(st StoreIface) MCPGrantStore {
	if x, ok := st.(MCPGrantStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migrateMCPGrants() error {
	q := `CREATE TABLE IF NOT EXISTS mcp_agent_grants (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL DEFAULT 'default',
		server_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		tool_allow TEXT NOT NULL DEFAULT '',
		tool_deny TEXT NOT NULL DEFAULT '',
		config_overrides TEXT NOT NULL DEFAULT '',
		granted_by TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT ''
	)`
	if _, err := s.db.Exec(q); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_mcp_grant ON mcp_agent_grants(server_id, agent_id)`); err != nil {
		return err
	}
	return nil
}

const mcpGrantColumns = `id, tenant_id, server_id, agent_id, enabled, tool_allow, tool_deny, config_overrides, granted_by, created_at`

func (s *SQLiteStore) ListMCPGrants(tenant, serverID string) []*MCPGrant {
	rows, err := s.db.Query(`SELECT `+mcpGrantColumns+` FROM mcp_agent_grants WHERE tenant_id=? AND server_id=? ORDER BY created_at`,
		NormalizeTenant(tenant), strings.TrimSpace(serverID))
	if err != nil {
		return []*MCPGrant{}
	}
	defer rows.Close()
	out := []*MCPGrant{}
	for rows.Next() {
		var g MCPGrant
		var enabled int
		if rows.Scan(&g.ID, &g.TenantID, &g.ServerID, &g.AgentID, &enabled, &g.ToolAllow,
			&g.ToolDeny, &g.ConfigOverrides, &g.GrantedBy, &g.CreatedAt) != nil {
			continue
		}
		g.Enabled = enabled != 0
		out = append(out, &g)
	}
	return out
}

func (s *SQLiteStore) ListAgentMCPGrants(tenant, agentID string) []*MCPGrant {
	rows, err := s.db.Query(`SELECT `+mcpGrantColumns+` FROM mcp_agent_grants WHERE tenant_id=? AND agent_id=? ORDER BY created_at`,
		NormalizeTenant(tenant), strings.TrimSpace(agentID))
	if err != nil {
		return []*MCPGrant{}
	}
	defer rows.Close()
	out := []*MCPGrant{}
	for rows.Next() {
		var g MCPGrant
		var enabled int
		if rows.Scan(&g.ID, &g.TenantID, &g.ServerID, &g.AgentID, &enabled, &g.ToolAllow,
			&g.ToolDeny, &g.ConfigOverrides, &g.GrantedBy, &g.CreatedAt) != nil {
			continue
		}
		g.Enabled = enabled != 0
		out = append(out, &g)
	}
	return out
}

func (s *SQLiteStore) PutMCPGrant(g MCPGrant) (*MCPGrant, error) {
	g.TenantID = NormalizeTenant(g.TenantID)
	if strings.TrimSpace(g.ServerID) == "" || strings.TrimSpace(g.AgentID) == "" {
		return nil, ErrNotFound
	}
	if g.ID == "" {
		g.ID = "mcpgr_" + newID()
	}
	if g.CreatedAt == "" {
		g.CreatedAt = formatTime(time.Now().UTC())
	}
	if g.ToolAllow == "" {
		g.ToolAllow = "[]"
	}
	if g.ToolDeny == "" {
		g.ToolDeny = "[]"
	}
	_, err := s.db.Exec(`INSERT INTO mcp_agent_grants(`+mcpGrantColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(server_id, agent_id) DO UPDATE SET enabled=excluded.enabled, tool_allow=excluded.tool_allow, tool_deny=excluded.tool_deny, config_overrides=excluded.config_overrides`,
		g.ID, g.TenantID, g.ServerID, g.AgentID, boolInt(g.Enabled), g.ToolAllow, g.ToolDeny,
		g.ConfigOverrides, g.GrantedBy, g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *SQLiteStore) DeleteMCPGrant(tenant, serverID, agentID string) error {
	res, err := s.db.Exec(`DELETE FROM mcp_agent_grants WHERE tenant_id=? AND server_id=? AND agent_id=?`,
		NormalizeTenant(tenant), strings.TrimSpace(serverID), strings.TrimSpace(agentID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
