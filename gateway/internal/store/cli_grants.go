// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"strings"
	"time"
)

// CLIGrant is one agent's access to a CLI binary.
type CLIGrant struct {
	ID         string `json:"id"`
	TenantID   string `json:"-"`
	BinaryID   string `json:"binary_id"`
	AgentID    string `json:"agent_id"`
	DenyArgs   string `json:"deny_args,omitempty"`
	Enabled    bool   `json:"enabled"`
	TimeoutSec int    `json:"timeout_seconds"`
	Tips       string `json:"tips,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// TeamUserGrant is one user's role on a team.
type TeamUserGrant struct {
	ID        string `json:"id"`
	TenantID  string `json:"-"`
	TeamID    string `json:"team_id"`
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	GrantedBy string `json:"granted_by,omitempty"`
	CreatedAt string `json:"created_at"`
}

// CLIGrantStore is the CLI access capability, and TeamUserGrantStore the team
// membership capability.
type CLIGrantStore interface {
	ListCLIBinaries(tenant string) []*CLIBinary
	ListCLIGrants(tenant, binaryID string) []*CLIGrant
	ListAgentCLIGrants(tenant, agentID string) []*CLIGrant
	PutCLIGrant(CLIGrant) (*CLIGrant, error)
	DeleteCLIGrant(tenant, id string) error
}

type TeamUserGrantStore interface {
	ListTeamUserGrants(tenant, teamID string) []*TeamUserGrant
	PutTeamUserGrant(TeamUserGrant) (*TeamUserGrant, error)
	DeleteTeamUserGrant(tenant, id string) error
}

func CLIGrants(st StoreIface) CLIGrantStore {
	if x, ok := st.(CLIGrantStore); ok {
		return x
	}
	return nil
}

func TeamUserGrants(st StoreIface) TeamUserGrantStore {
	if x, ok := st.(TeamUserGrantStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migrateCLIGrants() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS secure_cli_agent_grants (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			binary_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			deny_args TEXT NOT NULL DEFAULT '[]',
			enabled INTEGER NOT NULL DEFAULT 1,
			timeout_seconds INTEGER NOT NULL DEFAULT 0,
			tips TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_cli_grant ON secure_cli_agent_grants(tenant_id, binary_id, agent_id)`,
		`CREATE TABLE IF NOT EXISTS team_user_grants (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			team_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer',
			granted_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_team_user_grant ON team_user_grants(tenant_id, team_id, user_id)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

const cliGrantColumns = "id, tenant_id, binary_id, agent_id, deny_args, enabled, timeout_seconds, tips, created_at"

func scanCLIGrant(rows interface {
	Scan(dest ...any) error
}) (*CLIGrant, error) {
	var g CLIGrant
	var enabled int
	if err := rows.Scan(&g.ID, &g.TenantID, &g.BinaryID, &g.AgentID, &g.DenyArgs,
		&enabled, &g.TimeoutSec, &g.Tips, &g.CreatedAt); err != nil {
		return nil, err
	}
	g.Enabled = enabled != 0
	return &g, nil
}

func (s *SQLiteStore) ListCLIGrants(tenant, binaryID string) []*CLIGrant {
	q := "SELECT " + cliGrantColumns + " FROM secure_cli_agent_grants WHERE tenant_id=?"
	args := []any{NormalizeTenant(tenant)}
	if strings.TrimSpace(binaryID) != "" {
		q += " AND binary_id=?"
		args = append(args, strings.TrimSpace(binaryID))
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return []*CLIGrant{}
	}
	defer rows.Close()
	out := []*CLIGrant{}
	for rows.Next() {
		if g, err := scanCLIGrant(rows); err == nil {
			out = append(out, g)
		}
	}
	return out
}

func (s *SQLiteStore) ListAgentCLIGrants(tenant, agentID string) []*CLIGrant {
	rows, err := s.db.Query("SELECT "+cliGrantColumns+
		" FROM secure_cli_agent_grants WHERE tenant_id=? AND agent_id=?", NormalizeTenant(tenant), strings.TrimSpace(agentID))
	if err != nil {
		return []*CLIGrant{}
	}
	defer rows.Close()
	out := []*CLIGrant{}
	for rows.Next() {
		if g, err := scanCLIGrant(rows); err == nil {
			out = append(out, g)
		}
	}
	return out
}

func (s *SQLiteStore) PutCLIGrant(g CLIGrant) (*CLIGrant, error) {
	g.TenantID = NormalizeTenant(g.TenantID)
	g.BinaryID = strings.TrimSpace(g.BinaryID)
	g.AgentID = strings.TrimSpace(g.AgentID)
	if g.BinaryID == "" || g.AgentID == "" {
		return nil, ErrNotFound
	}
	if g.ID == "" {
		g.ID = "clig_" + newID()
	}
	if g.CreatedAt == "" {
		g.CreatedAt = formatTime(time.Now().UTC())
	}
	if strings.TrimSpace(g.DenyArgs) == "" {
		g.DenyArgs = "[]"
	}
	_, err := s.db.Exec(`INSERT INTO secure_cli_agent_grants(`+cliGrantColumns+`) VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(tenant_id, binary_id, agent_id) DO UPDATE SET deny_args=excluded.deny_args,
		enabled=excluded.enabled, timeout_seconds=excluded.timeout_seconds, tips=excluded.tips`,
		g.ID, g.TenantID, g.BinaryID, g.AgentID, g.DenyArgs, boolInt(g.Enabled),
		g.TimeoutSec, g.Tips, g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *SQLiteStore) DeleteCLIGrant(tenant, id string) error {
	res, err := s.db.Exec("DELETE FROM secure_cli_agent_grants WHERE id=? AND tenant_id=?",
		strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const teamUserGrantColumns = "id, tenant_id, team_id, user_id, role, granted_by, created_at"

func (s *SQLiteStore) ListTeamUserGrants(tenant, teamID string) []*TeamUserGrant {
	q := "SELECT " + teamUserGrantColumns + " FROM team_user_grants WHERE tenant_id=?"
	args := []any{NormalizeTenant(tenant)}
	if strings.TrimSpace(teamID) != "" {
		q += " AND team_id=?"
		args = append(args, strings.TrimSpace(teamID))
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return []*TeamUserGrant{}
	}
	defer rows.Close()
	out := []*TeamUserGrant{}
	for rows.Next() {
		var g TeamUserGrant
		if rows.Scan(&g.ID, &g.TenantID, &g.TeamID, &g.UserID, &g.Role, &g.GrantedBy, &g.CreatedAt) != nil {
			continue
		}
		out = append(out, &g)
	}
	return out
}

func (s *SQLiteStore) PutTeamUserGrant(g TeamUserGrant) (*TeamUserGrant, error) {
	g.TenantID = NormalizeTenant(g.TenantID)
	g.TeamID = strings.TrimSpace(g.TeamID)
	g.UserID = strings.TrimSpace(g.UserID)
	if g.TeamID == "" || g.UserID == "" {
		return nil, ErrNotFound
	}
	if strings.TrimSpace(g.Role) == "" {
		g.Role = "viewer"
	}
	if g.ID == "" {
		g.ID = "tug_" + newID()
	}
	if g.CreatedAt == "" {
		g.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.Exec(`INSERT INTO team_user_grants(`+teamUserGrantColumns+`) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(tenant_id, team_id, user_id) DO UPDATE SET role=excluded.role, granted_by=excluded.granted_by`,
		g.ID, g.TenantID, g.TeamID, g.UserID, g.Role, g.GrantedBy, g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *SQLiteStore) DeleteTeamUserGrant(tenant, id string) error {
	res, err := s.db.Exec("DELETE FROM team_user_grants WHERE id=? AND tenant_id=?",
		strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
