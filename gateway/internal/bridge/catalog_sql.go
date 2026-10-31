// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"database/sql"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

type sqlCatalog struct {
	db *sql.DB
	pg bool
}

func newSQLCatalog(db *sql.DB, pg bool) (*sqlCatalog, error) {
	c := &sqlCatalog{db: db, pg: pg}
	if err := c.migrate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *sqlCatalog) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS omni_tenants (
			omni_id TEXT PRIMARY KEY,
			goso_id TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS omni_providers (
			tenant_id TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			name TEXT NOT NULL,
			provider_type TEXT NOT NULL,
			base_url TEXT NOT NULL DEFAULT '',
			models_json TEXT NOT NULL DEFAULT '[]',
			is_active INTEGER NOT NULL DEFAULT 1,
			key_nonce TEXT NOT NULL DEFAULT '',
			key_ct TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (tenant_id, provider_id)
		)`,
		`CREATE TABLE IF NOT EXISTS omni_agents (
			tenant_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			harness_type TEXT NOT NULL DEFAULT 'GOSO',
			provider_id TEXT NOT NULL DEFAULT '',
			model_name TEXT NOT NULL DEFAULT '',
			system_prompt TEXT NOT NULL DEFAULT '',
			temperature REAL,
			daily_reply_cap INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			harness_config_json TEXT NOT NULL DEFAULT '',
			params_json TEXT NOT NULL DEFAULT '',
			goso_agent_id TEXT NOT NULL DEFAULT '',
			goso_tenant_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (tenant_id, agent_id)
		)`,
		`CREATE TABLE IF NOT EXISTS omni_knowledge (
			tenant_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			doc_id TEXT NOT NULL,
			file_name TEXT NOT NULL DEFAULT '',
			mime_type TEXT NOT NULL DEFAULT '',
			file_size INTEGER NOT NULL DEFAULT 0,
			content TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (tenant_id, agent_id, doc_id)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := c.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (c *sqlCatalog) q(query string) string {
	if !c.pg {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

func (c *sqlCatalog) EnsureTenant(omniID string) (Tenant, error) {
	omniID = strings.TrimSpace(omniID)
	if omniID == "" {
		return Tenant{}, badRequest("tenant id is required")
	}
	gosoID := MapTenantID(omniID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := c.db.Exec(c.q(`INSERT INTO omni_tenants(omni_id, goso_id, created_at) VALUES(?,?,?) ON CONFLICT(omni_id) DO NOTHING`), omniID, gosoID, now)
	if err != nil {
		return Tenant{}, err
	}
	n, _ := res.RowsAffected()
	var got string
	if err := c.db.QueryRow(c.q(`SELECT goso_id FROM omni_tenants WHERE omni_id=?`), omniID).Scan(&got); err != nil {
		return Tenant{}, err
	}
	return Tenant{OmniID: omniID, GosoID: got, Created: n == 1}, nil
}

func (c *sqlCatalog) PutProvider(p Provider) error {
	if err := p.normalize(); err != nil {
		return err
	}
	nonce, ct, err := seal(p.APIKey)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	created := p.CreatedAt
	if created.IsZero() {
		if cur, gerr := c.GetProvider(p.TenantID, p.ID); gerr == nil && !cur.CreatedAt.IsZero() {
			created = cur.CreatedAt
		} else {
			created = now
		}
	}
	active := 0
	if p.Active {
		active = 1
	}
	_, err = c.db.Exec(c.q(`INSERT INTO omni_providers(
		tenant_id, provider_id, name, provider_type, base_url, models_json, is_active,
		key_nonce, key_ct, created_at, updated_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(tenant_id, provider_id) DO UPDATE SET
		name=excluded.name,
		provider_type=excluded.provider_type,
		base_url=excluded.base_url,
		models_json=excluded.models_json,
		is_active=excluded.is_active,
		key_nonce=excluded.key_nonce,
		key_ct=excluded.key_ct,
		updated_at=excluded.updated_at`),
		p.TenantID, p.ID, p.Name, p.Type, p.BaseURL, modelsJSON(p.Models), active,
		b64(nonce), b64(ct), created.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	return err
}

func (c *sqlCatalog) DeleteProvider(tenantID, providerID string) error {
	_, err := c.db.Exec(c.q(`DELETE FROM omni_providers WHERE tenant_id=? AND provider_id=?`), tenantID, providerID)
	return err
}

func (c *sqlCatalog) GetProvider(tenantID, providerID string) (Provider, error) {
	var p Provider
	var models, nonce, ct, created, updated string
	var active int
	err := c.db.QueryRow(c.q(`SELECT tenant_id, provider_id, name, provider_type, base_url, models_json, is_active, key_nonce, key_ct, created_at, updated_at
		FROM omni_providers WHERE tenant_id=? AND provider_id=?`), tenantID, providerID).
		Scan(&p.TenantID, &p.ID, &p.Name, &p.Type, &p.BaseURL, &models, &active, &nonce, &ct, &created, &updated)
	if err == sql.ErrNoRows {
		return Provider{}, ErrNotFound
	}
	if err != nil {
		return Provider{}, err
	}
	p.Models = parseModels(models)
	p.Active = active != 0
	p.CreatedAt = parseStamp(created)
	p.UpdatedAt = parseStamp(updated)
	key, err := openSeal(b64dec(nonce), b64dec(ct))
	if err != nil {
		return Provider{}, err
	}
	p.APIKey = key
	p.KeySet = key != ""
	return p, nil
}

func (c *sqlCatalog) ListProviders(tenantID string) ([]Provider, error) {
	rows, err := c.db.Query(c.q(`SELECT provider_id FROM omni_providers WHERE tenant_id=? ORDER BY provider_id`), tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Provider, 0, len(ids))
	for _, id := range ids {
		p, err := c.GetProvider(tenantID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (c *sqlCatalog) PutAgent(a Agent) error {
	if err := a.normalize(); err != nil {
		return err
	}
	now := time.Now().UTC()
	created := a.CreatedAt
	if prev, gerr := c.GetAgent(a.TenantID, a.ID); gerr == nil {
		if !prev.CreatedAt.IsZero() {
			created = prev.CreatedAt
		}
		if a.GosoAgentID == "" {
			a.GosoAgentID = prev.GosoAgentID
		}
		if a.GosoTenant == "" {
			a.GosoTenant = prev.GosoTenant
		}
	}
	if created.IsZero() {
		created = now
	}
	var temp any
	if a.Temperature != nil {
		temp = *a.Temperature
	}
	_, err := c.db.Exec(c.q(`INSERT INTO omni_agents(
		tenant_id, agent_id, name, description, harness_type, provider_id, model_name,
		system_prompt, temperature, daily_reply_cap, status, harness_config_json, params_json,
		goso_agent_id, goso_tenant_id, created_at, updated_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(tenant_id, agent_id) DO UPDATE SET
		name=excluded.name,
		description=excluded.description,
		harness_type=excluded.harness_type,
		provider_id=excluded.provider_id,
		model_name=excluded.model_name,
		system_prompt=excluded.system_prompt,
		temperature=excluded.temperature,
		daily_reply_cap=excluded.daily_reply_cap,
		status=excluded.status,
		harness_config_json=excluded.harness_config_json,
		params_json=excluded.params_json,
		goso_agent_id=excluded.goso_agent_id,
		goso_tenant_id=excluded.goso_tenant_id,
		updated_at=excluded.updated_at`),
		a.TenantID, a.ID, a.Name, a.Description, a.HarnessType, a.ProviderID, a.ModelName,
		a.SystemPrompt, temp, a.DailyReplyCap, a.Status, a.HarnessConfigJSON, a.ParamsJSON,
		a.GosoAgentID, a.GosoTenant, created.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	)
	return err
}

func (c *sqlCatalog) DeleteAgent(tenantID, agentID string) error {
	_, err := c.db.Exec(c.q(`DELETE FROM omni_agents WHERE tenant_id=? AND agent_id=?`), tenantID, agentID)
	return err
}

func (c *sqlCatalog) GetAgent(tenantID, agentID string) (Agent, error) {
	var a Agent
	var temp sql.NullFloat64
	var created, updated string
	err := c.db.QueryRow(c.q(`SELECT tenant_id, agent_id, name, description, harness_type, provider_id, model_name,
		system_prompt, temperature, daily_reply_cap, status, harness_config_json, params_json,
		goso_agent_id, goso_tenant_id, created_at, updated_at
		FROM omni_agents WHERE tenant_id=? AND agent_id=?`), tenantID, agentID).
		Scan(&a.TenantID, &a.ID, &a.Name, &a.Description, &a.HarnessType, &a.ProviderID, &a.ModelName,
			&a.SystemPrompt, &temp, &a.DailyReplyCap, &a.Status, &a.HarnessConfigJSON, &a.ParamsJSON,
			&a.GosoAgentID, &a.GosoTenant, &created, &updated)
	if err == sql.ErrNoRows {
		return Agent{}, ErrNotFound
	}
	if err != nil {
		return Agent{}, err
	}
	if temp.Valid {
		v := temp.Float64
		a.Temperature = &v
	}
	a.CreatedAt = parseStamp(created)
	a.UpdatedAt = parseStamp(updated)
	return a, nil
}

func (c *sqlCatalog) BindGosoAgent(tenantID, agentID, gosoAgentID string) error {
	res, err := c.db.Exec(c.q(`UPDATE omni_agents SET goso_agent_id=? WHERE tenant_id=? AND agent_id=?`), gosoAgentID, tenantID, agentID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (c *sqlCatalog) ReplaceKnowledge(tenantID, agentID string, docs []KnowledgeDoc) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(c.q(`DELETE FROM omni_knowledge WHERE tenant_id=? AND agent_id=?`), tenantID, agentID); err != nil {
		return err
	}
	for _, d := range docs {
		if _, err := tx.Exec(c.q(`INSERT INTO omni_knowledge(tenant_id, agent_id, doc_id, file_name, mime_type, file_size, content) VALUES(?,?,?,?,?,?,?)`),
			tenantID, agentID, d.ID, d.FileName, d.MimeType, d.FileSize, d.Content); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *sqlCatalog) ListKnowledge(tenantID, agentID string) ([]KnowledgeDoc, error) {
	rows, err := c.db.Query(c.q(`SELECT doc_id, file_name, mime_type, file_size, content FROM omni_knowledge WHERE tenant_id=? AND agent_id=? ORDER BY doc_id`), tenantID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KnowledgeDoc
	for rows.Next() {
		var d KnowledgeDoc
		if err := rows.Scan(&d.ID, &d.FileName, &d.MimeType, &d.FileSize, &d.Content); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func b64(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

func b64dec(s string) []byte {
	if s == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	return b
}

func parseStamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, s)
	}
	return t
}
