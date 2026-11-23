package store

import (
	"strings"
	"time"
)

// SkillRow is one registered skill.
type SkillRow struct {
	ID          string `json:"id"`
	TenantID    string `json:"-"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	OwnerID     string `json:"owner_id,omitempty"`
	Visibility  string `json:"visibility"`
	Version     int    `json:"version"`
	Status      string `json:"status"`
	FilePath    string `json:"file_path,omitempty"`
	FileSize    int64  `json:"file_size"`
	FileHash    string `json:"file_hash,omitempty"`
	Tags        string `json:"tags,omitempty"`
	Deps        string `json:"deps,omitempty"`
	Frontmatter string `json:"frontmatter,omitempty"`
	IsSystem    bool   `json:"is_system"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	AgentGrants int    `json:"agent_grants"`
}

// SkillGrant is one agent's permission on a skill.
type SkillGrant struct {
	ID            string `json:"id"`
	TenantID      string `json:"-"`
	SkillID       string `json:"skill_id"`
	AgentID       string `json:"agent_id"`
	PinnedVersion int    `json:"pinned_version"`
	GrantedBy     string `json:"granted_by,omitempty"`
	CanManage     bool   `json:"can_manage"`
	CreatedAt     string `json:"created_at"`
}

// SkillEvolution is one skill's improvement settings.
type SkillEvolution struct {
	TenantID      string `json:"-"`
	SkillID       string `json:"skill_id"`
	Enabled       bool   `json:"enabled"`
	AutoApply     bool   `json:"auto_apply"`
	MinSampleSize int    `json:"min_sample_size"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

// SkillStore is the skill registry, grant and evolution capability.
type SkillStore interface {
	ListSkillRows(tenant string) []*SkillRow
	GetSkillRow(tenant, idOrSlug string) (*SkillRow, error)
	PutSkillRow(SkillRow) (*SkillRow, error)
	DeleteSkillRow(tenant, id string) error
	ListSkillGrants(tenant, skillID string) []*SkillGrant
	ListAgentSkillGrants(tenant, agentID string) []*SkillGrant
	PutSkillGrant(SkillGrant) (*SkillGrant, error)
	DeleteSkillGrant(tenant, skillID, agentID string) error
	GetSkillEvolution(tenant, skillID string) *SkillEvolution
	PutSkillEvolution(SkillEvolution) error
}

// Skills returns the skill registry capability of st, or nil.
func Skills(st StoreIface) SkillStore {
	if x, ok := st.(SkillStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migrateSkills() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS skills (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			name TEXT NOT NULL,
			slug TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			owner_id TEXT NOT NULL DEFAULT '',
			visibility TEXT NOT NULL DEFAULT 'private',
			version INTEGER NOT NULL DEFAULT 1,
			status TEXT NOT NULL DEFAULT 'active',
			file_path TEXT NOT NULL DEFAULT '',
			file_size INTEGER NOT NULL DEFAULT 0,
			file_hash TEXT NOT NULL DEFAULT '',
			tags TEXT NOT NULL DEFAULT '',
			deps TEXT NOT NULL DEFAULT '{}',
			frontmatter TEXT NOT NULL DEFAULT '{}',
			is_system INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_skills_tenant_slug ON skills(tenant_id, slug)`,
		`CREATE TABLE IF NOT EXISTS skill_versions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			skill_id TEXT NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			content TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS skill_agent_grants (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			skill_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			pinned_version INTEGER NOT NULL DEFAULT 1,
			granted_by TEXT NOT NULL DEFAULT '',
			can_manage INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_skill_agent_grant ON skill_agent_grants(skill_id, agent_id)`,
		`CREATE TABLE IF NOT EXISTS skill_evolution_settings (
			tenant_id TEXT NOT NULL DEFAULT 'default',
			skill_id TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			auto_apply INTEGER NOT NULL DEFAULT 0,
			min_sample_size INTEGER NOT NULL DEFAULT 20,
			updated_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (tenant_id, skill_id)
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

const skillColumns = `id, tenant_id, name, slug, description, owner_id, visibility, version, status, file_path, file_size, file_hash, tags, deps, frontmatter, is_system, enabled, created_at, updated_at`

func scanSkillRow(sc scanner) (*SkillRow, error) {
	var r SkillRow
	var isSystem, enabled int
	err := sc.Scan(&r.ID, &r.TenantID, &r.Name, &r.Slug, &r.Description, &r.OwnerID,
		&r.Visibility, &r.Version, &r.Status, &r.FilePath, &r.FileSize, &r.FileHash,
		&r.Tags, &r.Deps, &r.Frontmatter, &isSystem, &enabled, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.IsSystem = isSystem != 0
	r.Enabled = enabled != 0
	return &r, nil
}

func (s *SQLiteStore) ListSkillRows(tenant string) []*SkillRow {
	rows, err := s.db.Query(`SELECT `+skillColumns+` FROM skills WHERE tenant_id=? ORDER BY is_system DESC, name`, NormalizeTenant(tenant))
	if err != nil {
		return []*SkillRow{}
	}
	defer rows.Close()
	out := []*SkillRow{}
	for rows.Next() {
		if r, err := scanSkillRow(rows); err == nil {
			out = append(out, r)
		}
	}
	return out
}

func (s *SQLiteStore) GetSkillRow(tenant, idOrSlug string) (*SkillRow, error) {
	want := strings.TrimSpace(idOrSlug)
	row := s.db.QueryRow(`SELECT `+skillColumns+` FROM skills WHERE tenant_id=? AND (id=? OR slug=? OR name=?)`,
		NormalizeTenant(tenant), want, want, want)
	r, err := scanSkillRow(row)
	if err != nil {
		return nil, ErrNotFound
	}
	return r, nil
}

func (s *SQLiteStore) PutSkillRow(in SkillRow) (*SkillRow, error) {
	in.TenantID = NormalizeTenant(in.TenantID)
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(in.Slug)
	if in.Slug == "" {
		in.Slug = strings.ToLower(strings.ReplaceAll(in.Name, " ", "-"))
	}
	if in.Name == "" || in.Slug == "" {
		return nil, ErrNotFound
	}
	if in.Visibility == "" {
		in.Visibility = "tenant"
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.Version == 0 {
		in.Version = 1
	}
	now := formatTime(time.Now().UTC())
	if in.ID == "" {
		in.ID = "sk_" + newID()
		in.CreatedAt = now
		in.UpdatedAt = now
		_, err := s.db.Exec(`INSERT INTO skills(`+skillColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			in.ID, in.TenantID, in.Name, in.Slug, in.Description, in.OwnerID, in.Visibility,
			in.Version, in.Status, in.FilePath, in.FileSize, in.FileHash, in.Tags, in.Deps,
			in.Frontmatter, boolInt(in.IsSystem), boolInt(in.Enabled), in.CreatedAt, in.UpdatedAt)
		if err != nil {
			return nil, err
		}
		return &in, nil
	}
	in.UpdatedAt = now
	res, err := s.db.Exec(`UPDATE skills SET name=?, slug=?, description=?, owner_id=?, visibility=?, version=?, status=?, file_path=?, file_size=?, file_hash=?, tags=?, deps=?, frontmatter=?, is_system=?, enabled=?, updated_at=? WHERE id=? AND tenant_id=?`,
		in.Name, in.Slug, in.Description, in.OwnerID, in.Visibility, in.Version, in.Status,
		in.FilePath, in.FileSize, in.FileHash, in.Tags, in.Deps, in.Frontmatter,
		boolInt(in.IsSystem), boolInt(in.Enabled), in.UpdatedAt, in.ID, in.TenantID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return &in, nil
}

func (s *SQLiteStore) DeleteSkillRow(tenant, id string) error {
	res, err := s.db.Exec(`DELETE FROM skills WHERE id=? AND tenant_id=?`, strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, _ = s.db.Exec(`DELETE FROM skill_agent_grants WHERE skill_id=?`, id)
	return nil
}

const skillGrantColumns = `id, tenant_id, skill_id, agent_id, pinned_version, granted_by, can_manage, created_at`

func (s *SQLiteStore) ListSkillGrants(tenant, skillID string) []*SkillGrant {
	rows, err := s.db.Query(`SELECT `+skillGrantColumns+` FROM skill_agent_grants WHERE tenant_id=? AND skill_id=?`, NormalizeTenant(tenant), strings.TrimSpace(skillID))
	if err != nil {
		return []*SkillGrant{}
	}
	defer rows.Close()
	out := []*SkillGrant{}
	for rows.Next() {
		var g SkillGrant
		var cm int
		if rows.Scan(&g.ID, &g.TenantID, &g.SkillID, &g.AgentID, &g.PinnedVersion, &g.GrantedBy, &cm, &g.CreatedAt) != nil {
			continue
		}
		g.CanManage = cm != 0
		out = append(out, &g)
	}
	return out
}

func (s *SQLiteStore) ListAgentSkillGrants(tenant, agentID string) []*SkillGrant {
	rows, err := s.db.Query(`SELECT `+skillGrantColumns+` FROM skill_agent_grants WHERE tenant_id=? AND agent_id=?`, NormalizeTenant(tenant), strings.TrimSpace(agentID))
	if err != nil {
		return []*SkillGrant{}
	}
	defer rows.Close()
	out := []*SkillGrant{}
	for rows.Next() {
		var g SkillGrant
		var cm int
		if rows.Scan(&g.ID, &g.TenantID, &g.SkillID, &g.AgentID, &g.PinnedVersion, &g.GrantedBy, &cm, &g.CreatedAt) != nil {
			continue
		}
		g.CanManage = cm != 0
		out = append(out, &g)
	}
	return out
}

func (s *SQLiteStore) PutSkillGrant(g SkillGrant) (*SkillGrant, error) {
	g.TenantID = NormalizeTenant(g.TenantID)
	if strings.TrimSpace(g.SkillID) == "" || strings.TrimSpace(g.AgentID) == "" {
		return nil, ErrNotFound
	}
	if g.ID == "" {
		g.ID = "skg_" + newID()
	}
	if g.CreatedAt == "" {
		g.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.Exec(`INSERT INTO skill_agent_grants(`+skillGrantColumns+`) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(skill_id, agent_id) DO UPDATE SET pinned_version=excluded.pinned_version, can_manage=excluded.can_manage`,
		g.ID, g.TenantID, g.SkillID, g.AgentID, g.PinnedVersion, g.GrantedBy, boolInt(g.CanManage), g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *SQLiteStore) DeleteSkillGrant(tenant, skillID, agentID string) error {
	res, err := s.db.Exec(`DELETE FROM skill_agent_grants WHERE tenant_id=? AND skill_id=? AND agent_id=?`,
		NormalizeTenant(tenant), strings.TrimSpace(skillID), strings.TrimSpace(agentID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) GetSkillEvolution(tenant, skillID string) *SkillEvolution {
	var ev SkillEvolution
	var enabled, autoApply int
	err := s.db.QueryRow(`SELECT tenant_id, skill_id, enabled, auto_apply, min_sample_size, updated_at FROM skill_evolution_settings WHERE tenant_id=? AND skill_id=?`,
		NormalizeTenant(tenant), strings.TrimSpace(skillID)).
		Scan(&ev.TenantID, &ev.SkillID, &enabled, &autoApply, &ev.MinSampleSize, &ev.UpdatedAt)
	if err != nil {
		return &SkillEvolution{TenantID: NormalizeTenant(tenant), SkillID: strings.TrimSpace(skillID), MinSampleSize: 20}
	}
	ev.Enabled = enabled != 0
	ev.AutoApply = autoApply != 0
	return &ev
}

func (s *SQLiteStore) PutSkillEvolution(ev SkillEvolution) error {
	if ev.MinSampleSize <= 0 {
		ev.MinSampleSize = 20
	}
	now := formatTime(time.Now().UTC())
	_, err := s.db.Exec(`INSERT INTO skill_evolution_settings(tenant_id, skill_id, enabled, auto_apply, min_sample_size, updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(tenant_id, skill_id) DO UPDATE SET enabled=excluded.enabled, auto_apply=excluded.auto_apply, min_sample_size=excluded.min_sample_size, updated_at=excluded.updated_at`,
		NormalizeTenant(ev.TenantID), strings.TrimSpace(ev.SkillID), boolInt(ev.Enabled), boolInt(ev.AutoApply), ev.MinSampleSize, now)
	return err
}
