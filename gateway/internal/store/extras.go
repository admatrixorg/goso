package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

// AgentHeartbeat is one agent's periodic self-check schedule.
type AgentHeartbeat struct {
	ID               string `json:"id"`
	TenantID         string `json:"-"`
	AgentID          string `json:"agentId"`
	Enabled          bool   `json:"enabled"`
	IntervalSec      int    `json:"intervalSec"`
	Prompt           string `json:"prompt,omitempty"`
	Checklist        string `json:"checklist,omitempty"`
	Model            string `json:"model,omitempty"`
	IsolatedSession  bool   `json:"isolatedSession"`
	LightContext     bool   `json:"lightContext"`
	AckMaxChars      int    `json:"ackMaxChars"`
	MaxRetries       int    `json:"maxRetries"`
	ActiveHoursStart string `json:"activeHoursStart,omitempty"`
	ActiveHoursEnd   string `json:"activeHoursEnd,omitempty"`
	Timezone         string `json:"timezone,omitempty"`
	Channel          string `json:"channel,omitempty"`
	ChatID           string `json:"chatId,omitempty"`
	NextRunAt        string `json:"nextRunAt,omitempty"`
	LastRunAt        string `json:"lastRunAt,omitempty"`
	LastStatus       string `json:"lastStatus,omitempty"`
	LastError        string `json:"lastError,omitempty"`
	RunCount         int    `json:"runCount"`
	SuppressCount    int    `json:"suppressCount"`
	CreatedAt        string `json:"createdAt,omitempty"`
	UpdatedAt        string `json:"updatedAt,omitempty"`
}

// MCPServer is one external MCP tool server. APIKey is never returned.
type MCPServer struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"-"`
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	Transport   string          `json:"transport"`
	Command     string          `json:"command"`
	Args        json.RawMessage `json:"args"`
	URL         string          `json:"url"`
	Headers     json.RawMessage `json:"headers"`
	Env         json.RawMessage `json:"env"`
	APIKey      string          `json:"-"`
	ToolPrefix  string          `json:"tool_prefix"`
	TimeoutSec  int             `json:"timeout_sec"`
	Settings    json.RawMessage `json:"settings,omitempty"`
	Enabled     bool            `json:"enabled"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// CLIBinary is one wrapped command-line tool.
type CLIBinary struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"-"`
	BinaryName     string          `json:"binary_name"`
	BinaryPath     string          `json:"binary_path,omitempty"`
	Description    string          `json:"description"`
	DenyArgs       json.RawMessage `json:"deny_args"`
	DenyVerbose    json.RawMessage `json:"deny_verbose"`
	TimeoutSeconds int             `json:"timeout_seconds"`
	Tips           string          `json:"tips"`
	Enabled        bool            `json:"enabled"`
	CreatedBy      string          `json:"created_by"`
	IsGlobal       bool            `json:"is_global"`
	AdapterName    string          `json:"adapter_name,omitempty"`
	HasSecret      bool            `json:"has_secret"`
	CredentialN    int             `json:"credential_count"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

// CLICredential is one user's stored secret for a binary. Secret is never loaded.
type CLICredential struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"-"`
	BinaryID       string          `json:"binary_id"`
	UserID         string          `json:"user_id"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	CredentialType string          `json:"credential_type,omitempty"`
	HostScope      string          `json:"host_scope,omitempty"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

// PricingOverride is one tenant's price for a model.
type PricingOverride struct {
	ID              string `json:"id"`
	TenantID        string `json:"-"`
	ProviderType    string `json:"provider_type"`
	ModelID         string `json:"model_id"`
	InputPrice      string `json:"input_price"`
	OutputPrice     string `json:"output_price"`
	CacheReadPrice  string `json:"cache_read_price"`
	CacheWritePrice string `json:"cache_write_price"`
	ReasoningPrice  string `json:"reasoning_price"`
	RequestPrice    string `json:"request_price"`
	ImagePrice      string `json:"image_price"`
	WebSearchPrice  string `json:"web_search_price"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// ExtrasStore is the heartbeat, MCP, CLI and pricing capability.
type ExtrasStore interface {
	GetHeartbeat(tenant, agentID string) (*AgentHeartbeat, error)
	PutHeartbeat(AgentHeartbeat) (*AgentHeartbeat, error)
	ListHeartbeats(tenant string) []*AgentHeartbeat
	ListDueHeartbeats(now time.Time) []*AgentHeartbeat
	MarkHeartbeatRun(tenant, agentID, status, errMsg string) error
	ListMCPServers(tenant string) []*MCPServer
	PutMCPServer(MCPServer) (*MCPServer, error)
	DeleteMCPServer(tenant, id string) error
	SetMCPEnabled(tenant, id string, enabled bool) error
	ListCLIBinaries(tenant string) []*CLIBinary
	ListCLICredentials(tenant, binaryID string) []*CLICredential
	ListPricingOverrides(tenant string) []*PricingOverride
	PutPricingOverride(PricingOverride) (*PricingOverride, error)
	DeletePricingOverride(tenant, id string) error
}

var _ ExtrasStore = (*SQLiteStore)(nil)

// Extras returns the extras capability of st, or nil.
func Extras(st StoreIface) ExtrasStore {
	if x, ok := st.(ExtrasStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migrateExtras() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS agent_heartbeats (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			agent_id TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			interval_sec INTEGER NOT NULL DEFAULT 1800,
			prompt TEXT NOT NULL DEFAULT '',
			checklist TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			isolated_session INTEGER NOT NULL DEFAULT 1,
			light_context INTEGER NOT NULL DEFAULT 0,
			ack_max_chars INTEGER NOT NULL DEFAULT 300,
			max_retries INTEGER NOT NULL DEFAULT 2,
			active_hours_start TEXT NOT NULL DEFAULT '',
			active_hours_end TEXT NOT NULL DEFAULT '',
			timezone TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT '',
			chat_id TEXT NOT NULL DEFAULT '',
			next_run_at TEXT NOT NULL DEFAULT '',
			last_run_at TEXT NOT NULL DEFAULT '',
			last_status TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			run_count INTEGER NOT NULL DEFAULT 0,
			suppress_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_heartbeats_agent ON agent_heartbeats(tenant_id, agent_id)`,
		`CREATE TABLE IF NOT EXISTS mcp_servers (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			name TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			transport TEXT NOT NULL DEFAULT 'stdio',
			command TEXT NOT NULL DEFAULT '',
			args TEXT NOT NULL DEFAULT '[]',
			url TEXT NOT NULL DEFAULT '',
			headers TEXT NOT NULL DEFAULT '{}',
			env TEXT NOT NULL DEFAULT '{}',
			api_key TEXT NOT NULL DEFAULT '',
			tool_prefix TEXT NOT NULL DEFAULT '',
			timeout_sec INTEGER NOT NULL DEFAULT 60,
			settings TEXT NOT NULL DEFAULT '{}',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_mcp_tenant_name ON mcp_servers(tenant_id, name)`,
		`CREATE TABLE IF NOT EXISTS cli_binaries (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			binary_name TEXT NOT NULL,
			binary_path TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			deny_args TEXT NOT NULL DEFAULT '[]',
			deny_verbose TEXT NOT NULL DEFAULT '[]',
			timeout_seconds INTEGER NOT NULL DEFAULT 30,
			tips TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_by TEXT NOT NULL DEFAULT '',
			is_global INTEGER NOT NULL DEFAULT 1,
			adapter_name TEXT NOT NULL DEFAULT '',
			has_secret INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS cli_credentials (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			binary_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			metadata TEXT NOT NULL DEFAULT '{}',
			credential_type TEXT NOT NULL DEFAULT '',
			host_scope TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS pricing_overrides (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			provider_type TEXT NOT NULL DEFAULT '',
			model_id TEXT NOT NULL,
			input_price TEXT NOT NULL DEFAULT '',
			output_price TEXT NOT NULL DEFAULT '',
			cache_read_price TEXT NOT NULL DEFAULT '',
			cache_write_price TEXT NOT NULL DEFAULT '',
			reasoning_price TEXT NOT NULL DEFAULT '',
			request_price TEXT NOT NULL DEFAULT '',
			image_price TEXT NOT NULL DEFAULT '',
			web_search_price TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) GetHeartbeat(tenant, agentID string) (*AgentHeartbeat, error) {
	var (
		h       AgentHeartbeat
		enabled int
		isol    int
		light   int
	)
	err := s.db.QueryRow(`SELECT id, tenant_id, agent_id, enabled, interval_sec, prompt, checklist, model,
		isolated_session, light_context, ack_max_chars, max_retries, active_hours_start, active_hours_end,
		timezone, channel, chat_id, next_run_at, last_run_at, last_status, last_error, run_count, suppress_count,
		created_at, updated_at FROM agent_heartbeats WHERE tenant_id=? AND agent_id=?`,
		NormalizeTenant(tenant), strings.TrimSpace(agentID)).
		Scan(&h.ID, &h.TenantID, &h.AgentID, &enabled, &h.IntervalSec, &h.Prompt, &h.Checklist, &h.Model,
			&isol, &light, &h.AckMaxChars, &h.MaxRetries, &h.ActiveHoursStart, &h.ActiveHoursEnd,
			&h.Timezone, &h.Channel, &h.ChatID, &h.NextRunAt, &h.LastRunAt, &h.LastStatus, &h.LastError,
			&h.RunCount, &h.SuppressCount, &h.CreatedAt, &h.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	h.Enabled = enabled != 0
	h.IsolatedSession = isol != 0
	h.LightContext = light != 0
	return &h, nil
}

func (s *SQLiteStore) PutHeartbeat(h AgentHeartbeat) (*AgentHeartbeat, error) {
	h.TenantID = NormalizeTenant(h.TenantID)
	h.AgentID = strings.TrimSpace(h.AgentID)
	if h.AgentID == "" {
		return nil, errors.New("agent id is required")
	}
	if h.IntervalSec <= 0 {
		h.IntervalSec = 1800
	}
	if h.AckMaxChars <= 0 {
		h.AckMaxChars = 300
	}
	now := formatTime(time.Now().UTC())
	h.UpdatedAt = now
	cur, err := s.GetHeartbeat(h.TenantID, h.AgentID)
	if err == nil && cur != nil {
		h.ID = cur.ID
		h.CreatedAt = cur.CreatedAt
		if h.Prompt == "" {
			h.Prompt = cur.Prompt
		}
		if h.Checklist == "" {
			h.Checklist = cur.Checklist
		}
		if h.RunCount == 0 {
			h.RunCount = cur.RunCount
		}
		if h.SuppressCount == 0 {
			h.SuppressCount = cur.SuppressCount
		}
		if h.NextRunAt == "" && h.Enabled {
			h.NextRunAt = formatTime(time.Now().UTC().Add(time.Duration(h.IntervalSec) * time.Second))
		}
		_, uerr := s.db.Exec(`UPDATE agent_heartbeats SET enabled=?, interval_sec=?, prompt=?, checklist=?, model=?,
			isolated_session=?, light_context=?, ack_max_chars=?, max_retries=?, active_hours_start=?, active_hours_end=?,
			timezone=?, channel=?, chat_id=?, next_run_at=?, updated_at=? WHERE id=?`,
			boolInt(h.Enabled), h.IntervalSec, h.Prompt, h.Checklist, h.Model,
			boolInt(h.IsolatedSession), boolInt(h.LightContext), h.AckMaxChars, h.MaxRetries,
			h.ActiveHoursStart, h.ActiveHoursEnd, h.Timezone, h.Channel, h.ChatID, h.NextRunAt, h.UpdatedAt, h.ID)
		if uerr != nil {
			return nil, uerr
		}
		return &h, nil
	}
	h.ID = "hb_" + newID()
	h.CreatedAt = now
	if h.Enabled {
		h.NextRunAt = formatTime(time.Now().UTC().Add(time.Duration(h.IntervalSec) * time.Second))
	}
	_, ierr := s.db.Exec(`INSERT INTO agent_heartbeats(id, tenant_id, agent_id, enabled, interval_sec, prompt, checklist, model,
		isolated_session, light_context, ack_max_chars, max_retries, active_hours_start, active_hours_end,
		timezone, channel, chat_id, next_run_at, last_run_at, last_status, last_error, run_count, suppress_count,
		created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		h.ID, h.TenantID, h.AgentID, boolInt(h.Enabled), h.IntervalSec, h.Prompt, h.Checklist, h.Model,
		boolInt(h.IsolatedSession), boolInt(h.LightContext), h.AckMaxChars, h.MaxRetries,
		h.ActiveHoursStart, h.ActiveHoursEnd, h.Timezone, h.Channel, h.ChatID, h.NextRunAt, "", "", "",
		0, 0, h.CreatedAt, h.UpdatedAt)
	if ierr != nil {
		return nil, ierr
	}
	return &h, nil
}

func (s *SQLiteStore) ListHeartbeats(tenant string) []*AgentHeartbeat {
	rows, err := s.db.Query(`SELECT agent_id FROM agent_heartbeats WHERE tenant_id=?`, NormalizeTenant(tenant))
	if err != nil {
		return []*AgentHeartbeat{}
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	out := make([]*AgentHeartbeat, 0, len(ids))
	for _, id := range ids {
		if h, err := s.GetHeartbeat(tenant, id); err == nil && h != nil {
			out = append(out, h)
		}
	}
	return out
}

func (s *SQLiteStore) ListDueHeartbeats(now time.Time) []*AgentHeartbeat {
	rows, err := s.db.Query(`SELECT tenant_id, agent_id FROM agent_heartbeats
		WHERE enabled=1 AND (next_run_at='' OR next_run_at<=?) ORDER BY next_run_at`, formatTime(now.UTC()))
	if err != nil {
		return []*AgentHeartbeat{}
	}
	defer rows.Close()
	out := []*AgentHeartbeat{}
	for rows.Next() {
		var tenant, agent string
		if rows.Scan(&tenant, &agent) != nil {
			continue
		}
		if h, err := s.GetHeartbeat(tenant, agent); err == nil && h != nil {
			out = append(out, h)
		}
	}
	return out
}

func (s *SQLiteStore) MarkHeartbeatRun(tenant, agentID, status, errMsg string) error {
	now := time.Now().UTC()
	h, err := s.GetHeartbeat(tenant, agentID)
	if err != nil || h == nil {
		return ErrNotFound
	}
	next := ""
	if h.Enabled {
		next = formatTime(now.Add(time.Duration(h.IntervalSec) * time.Second))
	}
	_, uerr := s.db.Exec(`UPDATE agent_heartbeats SET last_run_at=?, last_status=?, last_error=?, run_count=run_count+1,
		next_run_at=?, updated_at=? WHERE id=?`, formatTime(now), status, errMsg, next, formatTime(now), h.ID)
	return uerr
}

func (s *SQLiteStore) ListMCPServers(tenant string) []*MCPServer {
	rows, err := s.db.Query(`SELECT id, tenant_id, name, display_name, transport, command, args, url, headers, env,
		tool_prefix, timeout_sec, settings, enabled, created_by, created_at, updated_at
		FROM mcp_servers WHERE tenant_id=? ORDER BY name`, NormalizeTenant(tenant))
	if err != nil {
		return []*MCPServer{}
	}
	defer rows.Close()
	out := []*MCPServer{}
	for rows.Next() {
		var (
			m       MCPServer
			enabled int
			args    string
			headers string
			env     string
			sett    string
		)
		if rows.Scan(&m.ID, &m.TenantID, &m.Name, &m.DisplayName, &m.Transport, &m.Command, &args, &m.URL,
			&headers, &env, &m.ToolPrefix, &m.TimeoutSec, &sett, &enabled, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt) != nil {
			continue
		}
		m.Enabled = enabled != 0
		m.Args = json.RawMessage(args)
		m.Headers = json.RawMessage(headers)
		m.Env = json.RawMessage(env)
		m.Settings = json.RawMessage(sett)
		out = append(out, &m)
	}
	return out
}

func (s *SQLiteStore) PutMCPServer(m MCPServer) (*MCPServer, error) {
	m.TenantID = NormalizeTenant(m.TenantID)
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" {
		return nil, errors.New("name is required")
	}
	now := formatTime(time.Now().UTC())
	if m.Transport == "" {
		m.Transport = "stdio"
	}
	if len(m.Args) == 0 {
		m.Args = json.RawMessage("[]")
	}
	if len(m.Headers) == 0 {
		m.Headers = json.RawMessage("{}")
	}
	if len(m.Env) == 0 {
		m.Env = json.RawMessage("{}")
	}
	if len(m.Settings) == 0 {
		m.Settings = json.RawMessage("{}")
	}
	if m.TimeoutSec <= 0 {
		m.TimeoutSec = 60
	}
	if m.ID == "" {
		m.ID = "mcp_" + newID()
		m.CreatedAt = now
		m.UpdatedAt = now
		_, err := s.db.Exec(`INSERT INTO mcp_servers(id, tenant_id, name, display_name, transport, command, args, url,
			headers, env, api_key, tool_prefix, timeout_sec, settings, enabled, created_by, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			m.ID, m.TenantID, m.Name, m.DisplayName, m.Transport, m.Command, string(m.Args), m.URL,
			string(m.Headers), string(m.Env), m.APIKey, m.ToolPrefix, m.TimeoutSec, string(m.Settings),
			boolInt(m.Enabled), m.CreatedBy, m.CreatedAt, m.UpdatedAt)
		if err != nil {
			return nil, err
		}
		return &m, nil
	}
	m.UpdatedAt = now
	res, err := s.db.Exec(`UPDATE mcp_servers SET display_name=?, transport=?, command=?, args=?, url=?, headers=?,
		env=?, api_key=COALESCE(NULLIF(?, ''), api_key), tool_prefix=?, timeout_sec=?, settings=?, enabled=?, updated_at=? WHERE id=? AND tenant_id=?`,
		m.DisplayName, m.Transport, m.Command, string(m.Args), m.URL, string(m.Headers), string(m.Env), m.APIKey,
		m.ToolPrefix, m.TimeoutSec, string(m.Settings), boolInt(m.Enabled), m.UpdatedAt, m.ID, m.TenantID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return &m, nil
}

func (s *SQLiteStore) DeleteMCPServer(tenant, id string) error {
	res, err := s.db.Exec(`DELETE FROM mcp_servers WHERE id=? AND tenant_id=?`, strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) SetMCPEnabled(tenant, id string, enabled bool) error {
	res, err := s.db.Exec(`UPDATE mcp_servers SET enabled=?, updated_at=? WHERE id=? AND tenant_id=?`,
		boolInt(enabled), formatTime(time.Now().UTC()), strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) ListCLIBinaries(tenant string) []*CLIBinary {
	rows, err := s.db.Query(`SELECT id, tenant_id, binary_name, binary_path, description, deny_args, deny_verbose,
		timeout_seconds, tips, enabled, created_by, is_global, adapter_name, has_secret, created_at, updated_at
		FROM cli_binaries WHERE tenant_id=? ORDER BY binary_name`, NormalizeTenant(tenant))
	if err != nil {
		return []*CLIBinary{}
	}
	defer rows.Close()
	out := []*CLIBinary{}
	for rows.Next() {
		var (
			b         CLIBinary
			enabled   int
			isGlobal  int
			hasSecret int
			denyArgs  string
			denyVerb  string
		)
		if rows.Scan(&b.ID, &b.TenantID, &b.BinaryName, &b.BinaryPath, &b.Description, &denyArgs, &denyVerb,
			&b.TimeoutSeconds, &b.Tips, &enabled, &b.CreatedBy, &isGlobal, &b.AdapterName, &hasSecret,
			&b.CreatedAt, &b.UpdatedAt) != nil {
			continue
		}
		b.Enabled = enabled != 0
		b.IsGlobal = isGlobal != 0
		b.HasSecret = hasSecret != 0
		b.DenyArgs = json.RawMessage(denyArgs)
		b.DenyVerbose = json.RawMessage(denyVerb)
		b.CredentialN = len(s.ListCLICredentials(tenant, b.ID))
		out = append(out, &b)
	}
	return out
}

func (s *SQLiteStore) ListCLICredentials(tenant, binaryID string) []*CLICredential {
	rows, err := s.db.Query(`SELECT id, tenant_id, binary_id, user_id, metadata, credential_type, host_scope, created_at, updated_at
		FROM cli_credentials WHERE tenant_id=? AND binary_id=? ORDER BY user_id`, NormalizeTenant(tenant), strings.TrimSpace(binaryID))
	if err != nil {
		return []*CLICredential{}
	}
	defer rows.Close()
	out := []*CLICredential{}
	for rows.Next() {
		var (
			c    CLICredential
			meta string
		)
		if rows.Scan(&c.ID, &c.TenantID, &c.BinaryID, &c.UserID, &meta, &c.CredentialType, &c.HostScope, &c.CreatedAt, &c.UpdatedAt) != nil {
			continue
		}
		c.Metadata = json.RawMessage(meta)
		out = append(out, &c)
	}
	return out
}

func (s *SQLiteStore) ListPricingOverrides(tenant string) []*PricingOverride {
	rows, err := s.db.Query(`SELECT id, tenant_id, provider_type, model_id, input_price, output_price,
		cache_read_price, cache_write_price, reasoning_price, request_price, image_price, web_search_price,
		enabled, created_at, updated_at FROM pricing_overrides WHERE tenant_id=? ORDER BY model_id`, NormalizeTenant(tenant))
	if err != nil {
		return []*PricingOverride{}
	}
	defer rows.Close()
	out := []*PricingOverride{}
	for rows.Next() {
		var (
			p       PricingOverride
			enabled int
		)
		if rows.Scan(&p.ID, &p.TenantID, &p.ProviderType, &p.ModelID, &p.InputPrice, &p.OutputPrice,
			&p.CacheReadPrice, &p.CacheWritePrice, &p.ReasoningPrice, &p.RequestPrice, &p.ImagePrice,
			&p.WebSearchPrice, &enabled, &p.CreatedAt, &p.UpdatedAt) != nil {
			continue
		}
		p.Enabled = enabled != 0
		out = append(out, &p)
	}
	return out
}

func (s *SQLiteStore) PutPricingOverride(p PricingOverride) (*PricingOverride, error) {
	p.TenantID = NormalizeTenant(p.TenantID)
	p.ModelID = strings.TrimSpace(p.ModelID)
	if p.ModelID == "" {
		return nil, errors.New("model_id is required")
	}
	now := formatTime(time.Now().UTC())
	if p.ID == "" {
		p.ID = "price_" + newID()
		p.CreatedAt = now
		p.UpdatedAt = now
		_, err := s.db.Exec(`INSERT INTO pricing_overrides(id, tenant_id, provider_type, model_id, input_price,
			output_price, cache_read_price, cache_write_price, reasoning_price, request_price, image_price,
			web_search_price, enabled, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.ID, p.TenantID, p.ProviderType, p.ModelID, p.InputPrice, p.OutputPrice, p.CacheReadPrice,
			p.CacheWritePrice, p.ReasoningPrice, p.RequestPrice, p.ImagePrice, p.WebSearchPrice,
			boolInt(p.Enabled), p.CreatedAt, p.UpdatedAt)
		if err != nil {
			return nil, err
		}
		return &p, nil
	}
	p.UpdatedAt = now
	res, err := s.db.Exec(`UPDATE pricing_overrides SET provider_type=?, model_id=?, input_price=?, output_price=?,
		cache_read_price=?, cache_write_price=?, reasoning_price=?, request_price=?, image_price=?, web_search_price=?,
		enabled=?, updated_at=? WHERE id=? AND tenant_id=?`,
		p.ProviderType, p.ModelID, p.InputPrice, p.OutputPrice, p.CacheReadPrice, p.CacheWritePrice,
		p.ReasoningPrice, p.RequestPrice, p.ImagePrice, p.WebSearchPrice, boolInt(p.Enabled), p.UpdatedAt, p.ID, p.TenantID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return &p, nil
}

func (s *SQLiteStore) DeletePricingOverride(tenant, id string) error {
	res, err := s.db.Exec(`DELETE FROM pricing_overrides WHERE id=? AND tenant_id=?`, strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
