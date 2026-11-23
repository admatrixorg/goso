package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Workstation is one remote backend an agent can run commands on.
type Workstation struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"-"`
	Key         string          `json:"workstation_key"`
	Name        string          `json:"name"`
	BackendType string          `json:"backend_type"`
	DefaultCWD  string          `json:"default_cwd,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	Active      bool            `json:"active"`
	CreatedBy   string          `json:"created_by,omitempty"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	AgentIDs    []string        `json:"agent_ids,omitempty"`
	Permissions int             `json:"permission_count"`
}

// WorkstationPermission is one allow-pattern for a workstation.
type WorkstationPermission struct {
	ID            string `json:"id"`
	TenantID      string `json:"-"`
	WorkstationID string `json:"workstation_id"`
	Pattern       string `json:"pattern"`
	Enabled       bool   `json:"enabled"`
	CreatedBy     string `json:"created_by,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// WorkstationActivity is one recorded command event.
type WorkstationActivity struct {
	ID            string `json:"id"`
	TenantID      string `json:"-"`
	WorkstationID string `json:"workstation_id"`
	AgentID       string `json:"agent_id,omitempty"`
	Action        string `json:"action"`
	Detail        string `json:"detail,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// WorkstationStore is the workstation capability.
type WorkstationStore interface {
	ListWorkstations(tenant string) []*Workstation
	GetWorkstation(tenant, idOrKey string) (*Workstation, error)
	PutWorkstation(Workstation) (*Workstation, error)
	DeleteWorkstation(tenant, id string) error
	ListWorkstationPermissions(tenant, workstationID string) []*WorkstationPermission
	PutWorkstationPermission(WorkstationPermission) (*WorkstationPermission, error)
	DeleteWorkstationPermission(tenant, id string) error
	SetWorkstationPermissionEnabled(tenant, id string, enabled bool) error
	ListWorkstationActivity(tenant, workstationID string, limit int) []*WorkstationActivity
	LinkWorkstationAgent(tenant, workstationID, agentID string) error
	UnlinkWorkstationAgent(tenant, workstationID, agentID string) error
}

// Workstations returns the workstation capability of st, or nil.
func Workstations(st StoreIface) WorkstationStore {
	if x, ok := st.(WorkstationStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migrateWorkstations() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS workstations (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			workstation_key TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			backend_type TEXT NOT NULL DEFAULT 'ssh',
			default_cwd TEXT NOT NULL DEFAULT '',
			metadata TEXT NOT NULL DEFAULT '{}',
			active INTEGER NOT NULL DEFAULT 1,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_workstations_key ON workstations(tenant_id, workstation_key)`,
		`CREATE TABLE IF NOT EXISTS workstation_permissions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			workstation_id TEXT NOT NULL,
			pattern TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS workstation_agents (
			tenant_id TEXT NOT NULL DEFAULT 'default',
			workstation_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY (workstation_id, agent_id)
		)`,
		`CREATE TABLE IF NOT EXISTS workstation_activity (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			workstation_id TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) GetWorkstation(tenant, idOrKey string) (*Workstation, error) {
	idOrKey = strings.TrimSpace(idOrKey)
	var (
		w      Workstation
		active int
		meta   string
	)
	err := s.db.QueryRow(`SELECT id, tenant_id, workstation_key, name, backend_type, default_cwd, metadata, active, created_by, created_at, updated_at
		FROM workstations WHERE tenant_id=? AND (id=? OR workstation_key=?)`, NormalizeTenant(tenant), idOrKey, idOrKey).
		Scan(&w.ID, &w.TenantID, &w.Key, &w.Name, &w.BackendType, &w.DefaultCWD, &meta, &active, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	w.Active = active != 0
	w.Metadata = json.RawMessage(meta)
	w.AgentIDs = s.workstationAgents(w.ID)
	w.Permissions = len(s.ListWorkstationPermissions(w.TenantID, w.ID))
	return &w, nil
}

func (s *SQLiteStore) workstationAgents(workstationID string) []string {
	rows, err := s.db.Query(`SELECT agent_id FROM workstation_agents WHERE workstation_id=? ORDER BY agent_id`, workstationID)
	if err != nil {
		return []string{}
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

func (s *SQLiteStore) ListWorkstations(tenant string) []*Workstation {
	rows, err := s.db.Query(`SELECT id FROM workstations WHERE tenant_id=? ORDER BY workstation_key`, NormalizeTenant(tenant))
	if err != nil {
		return []*Workstation{}
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	out := make([]*Workstation, 0, len(ids))
	for _, id := range ids {
		if w, err := s.GetWorkstation(tenant, id); err == nil && w != nil {
			out = append(out, w)
		}
	}
	return out
}

func (s *SQLiteStore) PutWorkstation(w Workstation) (*Workstation, error) {
	w.TenantID = NormalizeTenant(w.TenantID)
	w.Key = strings.TrimSpace(w.Key)
	if w.Key == "" {
		return nil, errors.New("workstation_key is required")
	}
	now := formatTime(time.Now().UTC())
	if w.BackendType == "" {
		w.BackendType = "ssh"
	}
	if len(w.Metadata) == 0 {
		w.Metadata = json.RawMessage("{}")
	}
	w.UpdatedAt = now
	if w.ID == "" {
		w.ID = "ws_" + newID()
		w.CreatedAt = now
		_, err := s.db.Exec(`INSERT INTO workstations(id, tenant_id, workstation_key, name, backend_type, default_cwd, metadata, active, created_by, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			w.ID, w.TenantID, w.Key, w.Name, w.BackendType, w.DefaultCWD, string(w.Metadata),
			boolInt(w.Active), w.CreatedBy, w.CreatedAt, w.UpdatedAt)
		if err != nil {
			return nil, err
		}
		return &w, nil
	}
	res, err := s.db.Exec(`UPDATE workstations SET workstation_key=?, name=?, backend_type=?, default_cwd=?, metadata=?, active=?, updated_at=?
		WHERE id=? AND tenant_id=?`,
		w.Key, w.Name, w.BackendType, w.DefaultCWD, string(w.Metadata), boolInt(w.Active), w.UpdatedAt, w.ID, w.TenantID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	if w.AgentIDs == nil {
		w.AgentIDs = s.workstationAgents(w.ID)
	}
	return &w, nil
}

func (s *SQLiteStore) DeleteWorkstation(tenant, id string) error {
	res, err := s.db.Exec(`DELETE FROM workstations WHERE (id=? OR workstation_key=?) AND tenant_id=?`,
		strings.TrimSpace(id), strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) ListWorkstationPermissions(tenant, workstationID string) []*WorkstationPermission {
	rows, err := s.db.Query(`SELECT id, tenant_id, workstation_id, pattern, enabled, created_by, created_at
		FROM workstation_permissions WHERE tenant_id=? AND workstation_id=? ORDER BY pattern`,
		NormalizeTenant(tenant), strings.TrimSpace(workstationID))
	if err != nil {
		return []*WorkstationPermission{}
	}
	defer rows.Close()
	out := []*WorkstationPermission{}
	for rows.Next() {
		var (
			p       WorkstationPermission
			enabled int
		)
		if rows.Scan(&p.ID, &p.TenantID, &p.WorkstationID, &p.Pattern, &enabled, &p.CreatedBy, &p.CreatedAt) != nil {
			continue
		}
		p.Enabled = enabled != 0
		out = append(out, &p)
	}
	return out
}

func (s *SQLiteStore) PutWorkstationPermission(p WorkstationPermission) (*WorkstationPermission, error) {
	p.TenantID = NormalizeTenant(p.TenantID)
	p.Pattern = strings.TrimSpace(p.Pattern)
	if p.Pattern == "" {
		return nil, errors.New("pattern is required")
	}
	if p.ID == "" {
		p.ID = "wsp_" + newID()
		p.CreatedAt = formatTime(time.Now().UTC())
		_, err := s.db.Exec(`INSERT INTO workstation_permissions(id, tenant_id, workstation_id, pattern, enabled, created_by, created_at)
			VALUES(?,?,?,?,?,?,?)`, p.ID, p.TenantID, p.WorkstationID, p.Pattern, boolInt(p.Enabled), p.CreatedBy, p.CreatedAt)
		if err != nil {
			return nil, err
		}
		return &p, nil
	}
	res, err := s.db.Exec(`UPDATE workstation_permissions SET pattern=?, enabled=? WHERE id=? AND tenant_id=?`,
		p.Pattern, boolInt(p.Enabled), p.ID, p.TenantID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return &p, nil
}

func (s *SQLiteStore) DeleteWorkstationPermission(tenant, id string) error {
	res, err := s.db.Exec(`DELETE FROM workstation_permissions WHERE id=? AND tenant_id=?`, strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) SetWorkstationPermissionEnabled(tenant, id string, enabled bool) error {
	res, err := s.db.Exec(`UPDATE workstation_permissions SET enabled=? WHERE id=? AND tenant_id=?`,
		boolInt(enabled), strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) ListWorkstationActivity(tenant, workstationID string, limit int) []*WorkstationActivity {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, workstation_id, agent_id, action, detail, created_at
		FROM workstation_activity WHERE tenant_id=? AND workstation_id=? ORDER BY created_at DESC LIMIT ?`,
		NormalizeTenant(tenant), strings.TrimSpace(workstationID), limit)
	if err != nil {
		return []*WorkstationActivity{}
	}
	defer rows.Close()
	out := []*WorkstationActivity{}
	for rows.Next() {
		var a WorkstationActivity
		if rows.Scan(&a.ID, &a.TenantID, &a.WorkstationID, &a.AgentID, &a.Action, &a.Detail, &a.CreatedAt) != nil {
			continue
		}
		out = append(out, &a)
	}
	return out
}

func (s *SQLiteStore) LinkWorkstationAgent(tenant, workstationID, agentID string) error {
	_, err := s.db.Exec(`INSERT INTO workstation_agents(tenant_id, workstation_id, agent_id, created_at)
		VALUES(?,?,?,?) ON CONFLICT(workstation_id, agent_id) DO NOTHING`,
		NormalizeTenant(tenant), strings.TrimSpace(workstationID), strings.TrimSpace(agentID), formatTime(time.Now().UTC()))
	return err
}

func (s *SQLiteStore) UnlinkWorkstationAgent(tenant, workstationID, agentID string) error {
	res, err := s.db.Exec(`DELETE FROM workstation_agents WHERE tenant_id=? AND workstation_id=? AND agent_id=?`,
		NormalizeTenant(tenant), strings.TrimSpace(workstationID), strings.TrimSpace(agentID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
