// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"
)

// Hook is one tenant-configured lifecycle hook. AgentID empty means every
// agent of the tenant. Matcher is a tool-name glob for PreToolUse and
// PostToolUse; empty or "*" matches every tool. Config is handler JSON.
// SecretEnc is the sealed HMAC signing secret for http hooks; it is never
// serialised.
type Hook struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	AgentID     string    `json:"agent_id,omitempty"`
	Event       string    `json:"event"`
	Matcher     string    `json:"matcher,omitempty"`
	HandlerType string    `json:"handler_type"`
	Config      string    `json:"-"`
	SecretEnc   string    `json:"-"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// HookStore is implemented by Store, SQLiteStore and PostgresStore. Every
// method takes the tenant; a hook of another tenant answers ErrNotFound.
type HookStore interface {
	CreateHook(Hook) (*Hook, error)
	GetHook(tenant, id string) (*Hook, error)
	ListHooks(tenant string) ([]*Hook, error)
	UpdateHook(Hook) (*Hook, error)
	DeleteHook(tenant, id string) error
}

var (
	_ HookStore = (*Store)(nil)
	_ HookStore = (*SQLiteStore)(nil)
)

// Hooks returns the hook store behind st, or nil when st has none.
func Hooks(st StoreIface) HookStore {
	if h, ok := st.(HookStore); ok {
		return h
	}
	return nil
}

func normalizeHook(h *Hook) error {
	h.TenantID = NormalizeTenant(h.TenantID)
	h.AgentID = strings.TrimSpace(h.AgentID)
	h.Event = strings.TrimSpace(h.Event)
	h.Matcher = strings.TrimSpace(h.Matcher)
	h.HandlerType = strings.TrimSpace(h.HandlerType)
	if strings.TrimSpace(h.Config) == "" {
		h.Config = "{}"
	}
	if h.Event == "" || h.HandlerType == "" {
		return errors.New("event and handler_type are required")
	}
	return nil
}

func (s *SQLiteStore) migrateHooks() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS hooks (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			agent_id TEXT,
			event TEXT NOT NULL,
			matcher TEXT NOT NULL DEFAULT '',
			handler_type TEXT NOT NULL,
			config TEXT NOT NULL DEFAULT '{}',
			secret_enc TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_hooks_tenant ON hooks(tenant_id, event)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

const hookCols = `id, tenant_id, agent_id, event, matcher, handler_type, config, secret_enc, enabled, created_at, updated_at`

func scanHook(sc scanner) (*Hook, error) {
	var h Hook
	var agent sql.NullString
	var en int
	var created, updated string
	if err := sc.Scan(&h.ID, &h.TenantID, &agent, &h.Event, &h.Matcher, &h.HandlerType, &h.Config, &h.SecretEnc, &en, &created, &updated); err != nil {
		return nil, err
	}
	h.AgentID = agent.String
	h.Enabled = en != 0
	h.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	h.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &h, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// CreateHook inserts a hook. ID and timestamps are assigned here.
func (s *SQLiteStore) CreateHook(h Hook) (*Hook, error) {
	if err := normalizeHook(&h); err != nil {
		return nil, err
	}
	h.ID = newID()
	h.CreatedAt = time.Now().UTC()
	h.UpdatedAt = h.CreatedAt
	_, err := s.db.Exec(`INSERT INTO hooks(`+hookCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		h.ID, h.TenantID, nullable(h.AgentID), h.Event, h.Matcher, h.HandlerType, h.Config, h.SecretEnc, boolInt(h.Enabled),
		formatTime(h.CreatedAt), formatTime(h.UpdatedAt))
	if err != nil {
		return nil, err
	}
	cp := h
	return &cp, nil
}

// GetHook returns one hook of this tenant.
func (s *SQLiteStore) GetHook(tenant, id string) (*Hook, error) {
	h, err := scanHook(s.db.QueryRow(`SELECT `+hookCols+` FROM hooks WHERE id=? AND tenant_id=?`, strings.TrimSpace(id), NormalizeTenant(tenant)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return h, err
}

// ListHooks returns this tenant's hooks, oldest first.
func (s *SQLiteStore) ListHooks(tenant string) ([]*Hook, error) {
	rows, err := s.db.Query(`SELECT `+hookCols+` FROM hooks WHERE tenant_id=? ORDER BY created_at, id`, NormalizeTenant(tenant))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Hook{}
	for rows.Next() {
		h, err := scanHook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// UpdateHook replaces the editable fields of a hook of h.TenantID. The
// tenant and creation time never change.
func (s *SQLiteStore) UpdateHook(h Hook) (*Hook, error) {
	if err := normalizeHook(&h); err != nil {
		return nil, err
	}
	cur, err := s.GetHook(h.TenantID, h.ID)
	if err != nil {
		return nil, err
	}
	h.CreatedAt = cur.CreatedAt
	h.UpdatedAt = time.Now().UTC()
	_, err = s.db.Exec(`UPDATE hooks SET agent_id=?, event=?, matcher=?, handler_type=?, config=?, secret_enc=?, enabled=?, updated_at=? WHERE id=? AND tenant_id=?`,
		nullable(h.AgentID), h.Event, h.Matcher, h.HandlerType, h.Config, h.SecretEnc, boolInt(h.Enabled), formatTime(h.UpdatedAt), h.ID, h.TenantID)
	if err != nil {
		return nil, err
	}
	cp := h
	return &cp, nil
}

// DeleteHook removes one hook of this tenant.
func (s *SQLiteStore) DeleteHook(tenant, id string) error {
	res, err := s.db.Exec(`DELETE FROM hooks WHERE id=? AND tenant_id=?`, strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- in-memory ---

func (s *Store) CreateHook(h Hook) (*Hook, error) {
	if err := normalizeHook(&h); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hooks == nil {
		s.hooks = map[string]*Hook{}
	}
	h.ID = s.nextID()
	h.CreatedAt = time.Now().UTC()
	h.UpdatedAt = h.CreatedAt
	cp := h
	s.hooks[h.ID] = &cp
	out := cp
	return &out, nil
}

func (s *Store) GetHook(tenant, id string) (*Hook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hooks[strings.TrimSpace(id)]
	if !ok || !SameTenant(h.TenantID, tenant) {
		return nil, ErrNotFound
	}
	cp := *h
	return &cp, nil
}

func (s *Store) ListHooks(tenant string) ([]*Hook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*Hook{}
	for _, h := range s.hooks {
		if SameTenant(h.TenantID, tenant) {
			cp := *h
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) UpdateHook(h Hook) (*Hook, error) {
	if err := normalizeHook(&h); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.hooks[h.ID]
	if !ok || !SameTenant(cur.TenantID, h.TenantID) {
		return nil, ErrNotFound
	}
	h.TenantID = cur.TenantID
	h.CreatedAt = cur.CreatedAt
	h.UpdatedAt = time.Now().UTC()
	cp := h
	s.hooks[h.ID] = &cp
	out := cp
	return &out, nil
}

func (s *Store) DeleteHook(tenant, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.hooks[strings.TrimSpace(id)]
	if !ok || !SameTenant(h.TenantID, tenant) {
		return ErrNotFound
	}
	delete(s.hooks, h.ID)
	return nil
}
