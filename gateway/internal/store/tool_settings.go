// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ToolSettings is one builtin tool's JSON settings for one tenant (for the
// media tools: {"providers":[...]}). Settings never hold a provider key; a
// provider entry names a saved provider whose key stays in secrets.
type ToolSettings struct {
	TenantID  string          `json:"tenant_id"`
	Tool      string          `json:"tool"`
	Settings  json.RawMessage `json:"settings"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ToolSettingsStore is implemented by Store, SQLiteStore and PostgresStore.
// It is a separate interface so StoreIface fakes keep compiling; callers
// type-assert. Every method takes the tenant; a read never crosses tenants.
type ToolSettingsStore interface {
	GetToolSettings(tenant, tool string) (*ToolSettings, error)
	PutToolSettings(ToolSettings) (*ToolSettings, error)
	DeleteToolSettings(tenant, tool string) error
	ListToolSettings(tenant string) ([]*ToolSettings, error)
}

// MaxToolSettingsBytes caps one settings document.
const MaxToolSettingsBytes = 64 << 10

var (
	_ ToolSettingsStore = (*Store)(nil)
	_ ToolSettingsStore = (*SQLiteStore)(nil)
	_ ToolSettingsStore = (*PostgresStore)(nil)
)

func normToolSettingsKey(tenant, tool string) (string, string, error) {
	tenant = NormalizeTenant(tenant)
	tool = strings.TrimSpace(tool)
	if tool == "" || len(tool) > 100 {
		return "", "", errors.New("tool is required")
	}
	return tenant, tool, nil
}

func validToolSettingsJSON(raw json.RawMessage) (json.RawMessage, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, errors.New("settings are required")
	}
	if len(raw) > MaxToolSettingsBytes {
		return nil, errors.New("settings too large")
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, errors.New("settings must be a JSON object")
	}
	return append(json.RawMessage(nil), raw...), nil
}

func cloneToolSettings(in *ToolSettings) *ToolSettings {
	if in == nil {
		return nil
	}
	cp := *in
	cp.Settings = append(json.RawMessage(nil), in.Settings...)
	return &cp
}

// ---- memory Store ----

func toolSettingsMapKey(tenant, tool string) string { return tenant + "\x00" + tool }

func (s *Store) GetToolSettings(tenant, tool string) (*ToolSettings, error) {
	tenant, tool, err := normToolSettingsKey(tenant, tool)
	if err != nil {
		return nil, ErrNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	row, ok := s.toolSettings[toolSettingsMapKey(tenant, tool)]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneToolSettings(row), nil
}

func (s *Store) PutToolSettings(in ToolSettings) (*ToolSettings, error) {
	tenant, tool, err := normToolSettingsKey(in.TenantID, in.Tool)
	if err != nil {
		return nil, err
	}
	raw, err := validToolSettingsJSON(in.Settings)
	if err != nil {
		return nil, err
	}
	row := &ToolSettings{TenantID: tenant, Tool: tool, Settings: raw, UpdatedAt: time.Now().UTC()}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.toolSettings == nil {
		s.toolSettings = map[string]*ToolSettings{}
	}
	s.toolSettings[toolSettingsMapKey(tenant, tool)] = row
	return cloneToolSettings(row), nil
}

func (s *Store) DeleteToolSettings(tenant, tool string) error {
	tenant, tool, err := normToolSettingsKey(tenant, tool)
	if err != nil {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := toolSettingsMapKey(tenant, tool)
	if _, ok := s.toolSettings[k]; !ok {
		return ErrNotFound
	}
	delete(s.toolSettings, k)
	return nil
}

func (s *Store) ListToolSettings(tenant string) ([]*ToolSettings, error) {
	tenant = NormalizeTenant(tenant)
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*ToolSettings{}
	for _, row := range s.toolSettings {
		if row.TenantID == tenant {
			out = append(out, cloneToolSettings(row))
		}
	}
	sortToolSettings(out)
	return out, nil
}

func sortToolSettings(rows []*ToolSettings) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].Tool < rows[j-1].Tool; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// ---- SQLite / Postgres ----

func (s *SQLiteStore) migrateToolSettings() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS builtin_tool_settings (
		tenant_id TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		settings TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (tenant_id, tool_name)
	)`)
	return err
}

func (s *SQLiteStore) GetToolSettings(tenant, tool string) (*ToolSettings, error) {
	tenant, tool, err := normToolSettingsKey(tenant, tool)
	if err != nil {
		return nil, ErrNotFound
	}
	var raw, stamp string
	err = s.db.QueryRow(
		`SELECT settings, updated_at FROM builtin_tool_settings WHERE tenant_id=? AND tool_name=?`,
		tenant, tool,
	).Scan(&raw, &stamp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ToolSettings{TenantID: tenant, Tool: tool, Settings: json.RawMessage(raw), UpdatedAt: parseToolSettingsStamp(stamp)}, nil
}

func (s *SQLiteStore) PutToolSettings(in ToolSettings) (*ToolSettings, error) {
	tenant, tool, err := normToolSettingsKey(in.TenantID, in.Tool)
	if err != nil {
		return nil, err
	}
	raw, err := validToolSettingsJSON(in.Settings)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if _, err := s.db.Exec(
		`INSERT INTO builtin_tool_settings(tenant_id, tool_name, settings, updated_at) VALUES(?,?,?,?)
		 ON CONFLICT(tenant_id, tool_name) DO UPDATE SET settings=excluded.settings, updated_at=excluded.updated_at`,
		tenant, tool, string(raw), now.Format(time.RFC3339Nano),
	); err != nil {
		return nil, err
	}
	return &ToolSettings{TenantID: tenant, Tool: tool, Settings: raw, UpdatedAt: now}, nil
}

func (s *SQLiteStore) DeleteToolSettings(tenant, tool string) error {
	tenant, tool, err := normToolSettingsKey(tenant, tool)
	if err != nil {
		return ErrNotFound
	}
	res, err := s.db.Exec(`DELETE FROM builtin_tool_settings WHERE tenant_id=? AND tool_name=?`, tenant, tool)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) ListToolSettings(tenant string) ([]*ToolSettings, error) {
	tenant = NormalizeTenant(tenant)
	rows, err := s.db.Query(
		`SELECT tool_name, settings, updated_at FROM builtin_tool_settings WHERE tenant_id=? ORDER BY tool_name`,
		tenant,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ToolSettings{}
	for rows.Next() {
		var tool, raw, stamp string
		if err := rows.Scan(&tool, &raw, &stamp); err != nil {
			return nil, err
		}
		out = append(out, &ToolSettings{TenantID: tenant, Tool: tool, Settings: json.RawMessage(raw), UpdatedAt: parseToolSettingsStamp(stamp)})
	}
	return out, rows.Err()
}

func parseToolSettingsStamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
