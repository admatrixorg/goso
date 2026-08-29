// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore persists agents/sessions/messages in SQLite.
// PostgresStore embeds this type and sets pg so FTS5 is skipped.
type SQLiteStore struct {
	db       *sqlHandle
	fts      bool
	vaultFTS bool
	kgFTS    bool
	pg       bool
	vector   bool
}

var _ StoreIface = (*SQLiteStore)(nil)

// OpenSQLite opens (and migrates) a SQLite DB at path. Use ":memory:" for in-memory.
func OpenSQLite(path string) (*SQLiteStore, error) {
	if err := RefusePostgres(path); err != nil {
		return nil, err
	}
	if path == "" {
		path = ":memory:"
	}
	// Ensure directory exists for file DBs.
	if path != ":memory:" && path != "" {
		if dir := filepath.Dir(path); dir != "." && dir != "" {
			// best-effort mkdir
			_ = fmt.Sprintf("%s", dir)
		}
	}
	dsn := path
	// modernc.org/sqlite uses file: URI; plain path also works for file.
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &SQLiteStore{db: &sqlHandle{db: db}}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			agent_key TEXT NOT NULL UNIQUE,
			display_name TEXT,
			model TEXT,
			llm_provider TEXT,
			created_at TEXT NOT NULL,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			enabled INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			label TEXT,
			created_at TEXT NOT NULL,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			prompt_mode TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(agent_id) REFERENCES agents(id)
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(session_id) REFERENCES sessions(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_agent ON sessions(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id)`,
		`CREATE TABLE IF NOT EXISTS connectors (
			name TEXT PRIMARY KEY,
			transport TEXT NOT NULL,
			endpoint TEXT,
			credential_ref TEXT,
			schema_version TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			manifest_url TEXT,
			manifest_json TEXT,
			timeout_ms INTEGER,
			retries INTEGER,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS agent_connectors (
			agent_id TEXT NOT NULL,
			connector_name TEXT NOT NULL,
			PRIMARY KEY(agent_id, connector_name),
			FOREIGN KEY(agent_id) REFERENCES agents(id),
			FOREIGN KEY(connector_name) REFERENCES connectors(name)
		)`,
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			body TEXT NOT NULL,
			created_at TEXT NOT NULL,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			FOREIGN KEY(session_id) REFERENCES sessions(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_session ON memories(session_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS vault_docs (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			path TEXT NOT NULL UNIQUE,
			sha256 TEXT NOT NULL,
			mtime TEXT NOT NULL,
			body TEXT,
			tenant_id TEXT NOT NULL DEFAULT 'default'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vault_docs_title ON vault_docs(title)`,
		`CREATE TABLE IF NOT EXISTS vault_links (
			from_id TEXT NOT NULL,
			to_id TEXT NOT NULL DEFAULT '',
			raw TEXT NOT NULL,
			PRIMARY KEY(from_id, raw)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vault_links_to ON vault_links(to_id)`,
		`CREATE TABLE IF NOT EXISTS teams (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			lead_agent_id TEXT,
			created_at TEXT NOT NULL,
			tenant_id TEXT NOT NULL DEFAULT 'default'
		)`,
		`CREATE TABLE IF NOT EXISTS team_members (
			team_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			role TEXT NOT NULL,
			PRIMARY KEY(team_id, agent_id)
		)`,
		`CREATE TABLE IF NOT EXISTS team_tasks (
			id TEXT PRIMARY KEY,
			team_id TEXT NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL,
			assignee_agent_id TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_team_tasks_team ON team_tasks(team_id, status)`,
		`CREATE TABLE IF NOT EXISTS team_messages (
			id TEXT PRIMARY KEY,
			team_id TEXT NOT NULL,
			from_agent_id TEXT,
			body TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_team_messages_team ON team_messages(team_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS agent_links (
			from_agent_id TEXT NOT NULL,
			to_agent_id TEXT NOT NULL,
			PRIMARY KEY(from_agent_id, to_agent_id)
		)`,
		`CREATE TABLE IF NOT EXISTS agent_metrics (
			agent_id TEXT PRIMARY KEY,
			chat_runs INTEGER NOT NULL DEFAULT 0,
			tool_errors INTEGER NOT NULL DEFAULT 0,
			tool_uses_json TEXT,
			advertised_json TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS evolution_applies (
			agent_id TEXT NOT NULL,
			suggestion_id TEXT NOT NULL,
			PRIMARY KEY(agent_id, suggestion_id)
		)`,
		`CREATE TABLE IF NOT EXISTS evolution_guardrails (
			agent_id TEXT PRIMARY KEY,
			payload TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS secrets (
			name TEXT PRIMARY KEY,
			nonce BLOB NOT NULL,
			ct BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tool_flags (
			name TEXT PRIMARY KEY,
			enabled INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS cron_jobs (
			id TEXT PRIMARY KEY,
			spec TEXT NOT NULL,
			session_id TEXT NOT NULL,
			message TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			last_run TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS llm_providers (
			name TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			base_url TEXT,
			model TEXT,
			tenant_id TEXT NOT NULL DEFAULT 'default'
		)`,
		`CREATE TABLE IF NOT EXISTS webhooks (
			id TEXT PRIMARY KEY,
			name TEXT,
			kind TEXT NOT NULL DEFAULT 'llm',
			agent_id TEXT,
			token_prefix TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			hmac_enc TEXT,
			require_hmac INTEGER NOT NULL DEFAULT 0,
			revoked INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			tenant_id TEXT NOT NULL DEFAULT 'default'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_webhooks_token_hash ON webhooks(token_hash)`,
		`CREATE TABLE IF NOT EXISTS webhook_jobs (
			id TEXT PRIMARY KEY,
			webhook_id TEXT NOT NULL,
			status TEXT NOT NULL,
			input TEXT,
			reply TEXT,
			error TEXT,
			callback_url TEXT,
			attempts INTEGER NOT NULL DEFAULT 0,
			next_attempt_at TEXT,
			idempotency_key TEXT,
			body_hash TEXT,
			lease_token TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_jobs_claim ON webhook_jobs(status, next_attempt_at)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_jobs_idem ON webhook_jobs(webhook_id, idempotency_key)`,
		`CREATE TABLE IF NOT EXISTS kg_entities (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			name TEXT NOT NULL,
			kind TEXT,
			body TEXT,
			valid_from TEXT NOT NULL,
			valid_until TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_kg_entities_tenant ON kg_entities(tenant_id)`,
		`CREATE TABLE IF NOT EXISTS kg_relations (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			from_id TEXT NOT NULL,
			to_id TEXT NOT NULL,
			rel TEXT NOT NULL,
			body TEXT,
			valid_from TEXT NOT NULL,
			valid_until TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_kg_relations_tenant ON kg_relations(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_kg_relations_from ON kg_relations(from_id)`,
		`CREATE INDEX IF NOT EXISTS idx_kg_relations_to ON kg_relations(to_id)`,
		`CREATE TABLE IF NOT EXISTS channel_config (
			name TEXT PRIMARY KEY,
			enabled INTEGER NOT NULL DEFAULT 1,
			agent_id TEXT NOT NULL DEFAULT '',
			dm_policy TEXT NOT NULL DEFAULT '',
			group_policy TEXT NOT NULL DEFAULT '',
			require_mention INTEGER NOT NULL DEFAULT 0,
			allow_from TEXT NOT NULL DEFAULT '[]',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS channel_pairing (
			id TEXT PRIMARY KEY,
			channel TEXT NOT NULL,
			sender_id TEXT NOT NULL,
			code_hash TEXT NOT NULL,
			status TEXT NOT NULL,
			expires_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			approved_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_channel_pairing_sender ON channel_pairing(channel, sender_id, status)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	_, _ = s.db.Exec(`ALTER TABLE agents ADD COLUMN instructions TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE agents ADD COLUMN orchestration_mode TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE agents ADD COLUMN llm_provider TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE agents ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`)
	_, _ = s.db.Exec(`ALTER TABLE agents ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`)
	_, _ = s.db.Exec(`ALTER TABLE agents ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE sessions ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`)
	_, _ = s.db.Exec(`ALTER TABLE sessions ADD COLUMN prompt_mode TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE memories ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`)
	_, _ = s.db.Exec(`ALTER TABLE vault_docs ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`)
	_, _ = s.db.Exec(`ALTER TABLE teams ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`)
	_, _ = s.db.Exec(`ALTER TABLE webhooks ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`)
	_, _ = s.db.Exec(`ALTER TABLE llm_providers ADD COLUMN tenant_id TEXT NOT NULL DEFAULT 'default'`)
	_, _ = s.db.Exec(`UPDATE agents SET tenant_id='default' WHERE tenant_id IS NULL OR tenant_id=''`)
	_, _ = s.db.Exec(`UPDATE sessions SET tenant_id='default' WHERE tenant_id IS NULL OR tenant_id=''`)
	_, _ = s.db.Exec(`UPDATE memories SET tenant_id='default' WHERE tenant_id IS NULL OR tenant_id=''`)
	_, _ = s.db.Exec(`UPDATE vault_docs SET tenant_id='default' WHERE tenant_id IS NULL OR tenant_id=''`)
	_, _ = s.db.Exec(`UPDATE teams SET tenant_id='default' WHERE tenant_id IS NULL OR tenant_id=''`)
	_, _ = s.db.Exec(`UPDATE webhooks SET tenant_id='default' WHERE tenant_id IS NULL OR tenant_id=''`)
	_, _ = s.db.Exec(`UPDATE llm_providers SET tenant_id='default' WHERE tenant_id IS NULL OR tenant_id=''`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_tenant ON agents(tenant_id)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_tenant ON sessions(tenant_id)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_memories_tenant ON memories(tenant_id)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_vault_docs_tenant ON vault_docs(tenant_id)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_teams_tenant ON teams(tenant_id)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_webhooks_tenant ON webhooks(tenant_id)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_llm_providers_tenant ON llm_providers(tenant_id)`)
	s.initFTS()
	s.initVaultFTS()
	s.initKGFTS()
	return nil
}

func (s *SQLiteStore) initFTS() {
	s.fts = false
	if s.pg {
		return
	}
	if _, err := s.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
		id UNINDEXED,
		session_id UNINDEXED,
		kind UNINDEXED,
		body
	)`); err != nil {
		return
	}
	triggers := []string{
		`CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
			INSERT INTO memory_fts(id, session_id, kind, body) VALUES (new.id, new.session_id, new.kind, new.body);
		END`,
		`CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
			DELETE FROM memory_fts WHERE id = old.id;
			INSERT INTO memory_fts(id, session_id, kind, body) VALUES (new.id, new.session_id, new.kind, new.body);
		END`,
		`CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
			INSERT INTO memory_fts(id, session_id, kind, body) VALUES (new.id, new.session_id, 'message', new.content);
		END`,
	}
	for _, stmt := range triggers {
		if _, err := s.db.Exec(stmt); err != nil {
			return
		}
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memory_fts`).Scan(&n); err != nil {
		return
	}
	if n == 0 {
		_, _ = s.db.Exec(`INSERT INTO memory_fts(id, session_id, kind, body) SELECT id, session_id, kind, body FROM memories`)
		_, _ = s.db.Exec(`INSERT INTO memory_fts(id, session_id, kind, body) SELECT id, session_id, 'message', content FROM messages`)
	}
	s.fts = true
}

func (s *SQLiteStore) initVaultFTS() {
	s.vaultFTS = false
	if s.pg {
		return
	}
	if _, err := s.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS vault_fts USING fts5(
		id UNINDEXED,
		title,
		body,
		path UNINDEXED
	)`); err != nil {
		return
	}
	triggers := []string{
		`CREATE TRIGGER IF NOT EXISTS vault_docs_ai AFTER INSERT ON vault_docs BEGIN
			INSERT INTO vault_fts(id, title, body, path) VALUES (new.id, new.title, new.body, new.path);
		END`,
		`CREATE TRIGGER IF NOT EXISTS vault_docs_au AFTER UPDATE ON vault_docs BEGIN
			DELETE FROM vault_fts WHERE id = old.id;
			INSERT INTO vault_fts(id, title, body, path) VALUES (new.id, new.title, new.body, new.path);
		END`,
		`CREATE TRIGGER IF NOT EXISTS vault_docs_ad AFTER DELETE ON vault_docs BEGIN
			DELETE FROM vault_fts WHERE id = old.id;
		END`,
	}
	for _, stmt := range triggers {
		if _, err := s.db.Exec(stmt); err != nil {
			return
		}
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM vault_fts`).Scan(&n); err != nil {
		return
	}
	if n == 0 {
		_, _ = s.db.Exec(`INSERT INTO vault_fts(id, title, body, path) SELECT id, title, body, path FROM vault_docs`)
	}
	s.vaultFTS = true
}

// Close closes the DB.
func (s *SQLiteStore) Close() error { return s.db.Close() }

func parseTime(v string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, v)
	if t.IsZero() {
		t, _ = time.Parse(time.RFC3339, v)
	}
	return t.UTC()
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// --- Agent ---

func (s *SQLiteStore) CreateAgent(a Agent) (*Agent, error) {
	if a.AgentKey == "" {
		return nil, errors.New("agent_key is required")
	}
	a.TenantID = NormalizeTenant(a.TenantID)
	if LiteEnabled() {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE tenant_id=?`, a.TenantID).Scan(&n); err != nil {
			return nil, err
		}
		if n >= LiteMaxAgents {
			return nil, ErrLiteCap
		}
	}
	a.ID = newID()
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now
	a.Enabled = true
	en := 1
	_, err := s.db.Exec(`INSERT INTO agents(id, agent_key, display_name, model, llm_provider, instructions, orchestration_mode, created_at, tenant_id, enabled, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.AgentKey, a.DisplayName, a.Model, a.LLMProvider, a.Instructions, a.OrchestrationMode, formatTime(a.CreatedAt), a.TenantID, en, formatTime(a.UpdatedAt))
	if err != nil {
		if LiteEnabled() {
			var n int
			if e := s.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE tenant_id=?`, a.TenantID).Scan(&n); e == nil && n >= LiteMaxAgents {
				return nil, ErrLiteCap
			}
		}
		return nil, ErrExists
	}
	cp := a
	return &cp, nil
}

func scanAgent(sc scanner) (*Agent, error) {
	var a Agent
	var ts, updated string
	var instructions, mode, llmProvider, tenant sql.NullString
	var enabled int
	if err := sc.Scan(&a.ID, &a.AgentKey, &a.DisplayName, &a.Model, &llmProvider, &instructions, &mode, &ts, &tenant, &enabled, &updated); err != nil {
		return nil, err
	}
	a.LLMProvider = llmProvider.String
	a.Instructions = instructions.String
	a.OrchestrationMode = mode.String
	a.TenantID = NormalizeTenant(tenant.String)
	a.CreatedAt = parseTime(ts)
	a.UpdatedAt = parseTime(updated)
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = a.CreatedAt
	}
	a.Enabled = enabled != 0
	return &a, nil
}

func (s *SQLiteStore) ListAgents() []*Agent {
	rows, err := s.db.Query(`SELECT id, agent_key, display_name, model, llm_provider, instructions, orchestration_mode, created_at, tenant_id, enabled, updated_at FROM agents ORDER BY created_at`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			continue
		}
		out = append(out, a)
	}
	if out == nil {
		out = []*Agent{}
	}
	return out
}

func (s *SQLiteStore) GetAgent(id string) (*Agent, error) {
	row := s.db.QueryRow(`SELECT id, agent_key, display_name, model, llm_provider, instructions, orchestration_mode, created_at, tenant_id, enabled, updated_at FROM agents WHERE id=?`, id)
	a, err := scanAgent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *SQLiteStore) UpdateAgent(a Agent) (*Agent, error) {
	if strings.TrimSpace(a.ID) == "" {
		return nil, errors.New("id is required")
	}
	cur, err := s.GetAgent(a.ID)
	if err != nil {
		return nil, err
	}
	if !a.UpdatedAt.IsZero() && !stampsMatch(cur.Stamp(), a.UpdatedAt) {
		return nil, ErrConflict
	}
	cur.Instructions = a.Instructions
	if strings.TrimSpace(a.OrchestrationMode) != "" {
		cur.OrchestrationMode = a.OrchestrationMode
	}
	if strings.TrimSpace(a.Model) != "" {
		cur.Model = a.Model
	}
	cur.LLMProvider = a.LLMProvider
	cur.Enabled = a.Enabled
	cur.UpdatedAt = nextStamp(cur.Stamp())
	en := 0
	if cur.Enabled {
		en = 1
	}
	_, err = s.db.Exec(`UPDATE agents SET instructions=?, orchestration_mode=?, model=?, llm_provider=?, enabled=?, updated_at=? WHERE id=?`,
		cur.Instructions, cur.OrchestrationMode, cur.Model, cur.LLMProvider, en, formatTime(cur.UpdatedAt), cur.ID)
	if err != nil {
		return nil, err
	}
	return cur, nil
}

func (s *SQLiteStore) DeleteAgent(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("id is required")
	}
	if _, err := s.GetAgent(id); err != nil {
		return err
	}
	var leadN int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM teams WHERE lead_agent_id=?`, id).Scan(&leadN); err != nil {
		return err
	}
	if leadN > 0 {
		return ErrConflict
	}
	rows, err := s.db.Query(`SELECT id FROM sessions WHERE agent_id=?`, id)
	if err != nil {
		return err
	}
	var sessIDs []string
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			continue
		}
		sessIDs = append(sessIDs, sid)
	}
	_ = rows.Close()
	for _, sid := range sessIDs {
		if err := s.DeleteSession(sid); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	_, _ = s.db.Exec(`DELETE FROM agent_connectors WHERE agent_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM agent_links WHERE from_agent_id=? OR to_agent_id=?`, id, id)
	_, _ = s.db.Exec(`DELETE FROM team_members WHERE agent_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM agent_metrics WHERE agent_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM evolution_applies WHERE agent_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM evolution_guardrails WHERE agent_id=?`, id)
	_, _ = s.db.Exec(`UPDATE channel_config SET agent_id='' WHERE agent_id=?`, id)
	res, err := s.db.Exec(`DELETE FROM agents WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Session ---

func (s *SQLiteStore) CreateSession(sess Session) (*Session, error) {
	if sess.AgentID == "" {
		return nil, errors.New("agent_id is required")
	}
	sess.TenantID = NormalizeTenant(sess.TenantID)
	ag, err := s.GetAgent(sess.AgentID)
	if err != nil {
		return nil, errors.New("agent not found")
	}
	if !SameTenant(ag.TenantID, sess.TenantID) {
		return nil, errors.New("agent not found")
	}
	sess.ID = newID()
	sess.CreatedAt = time.Now().UTC()
	_, err = s.db.Exec(`INSERT INTO sessions(id, agent_id, label, created_at, tenant_id, prompt_mode) VALUES(?,?,?,?,?,?)`,
		sess.ID, sess.AgentID, sess.Label, formatTime(sess.CreatedAt), sess.TenantID, sess.PromptMode)
	if err != nil {
		return nil, err
	}
	cp := sess
	return &cp, nil
}

func (s *SQLiteStore) ListSessions() []*Session {
	rows, err := s.db.Query(`SELECT id, agent_id, label, created_at, tenant_id, prompt_mode FROM sessions ORDER BY created_at`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Session
	for rows.Next() {
		sess, err := scanSessionRow(rows)
		if err != nil {
			continue
		}
		out = append(out, sess)
	}
	if out == nil {
		out = []*Session{}
	}
	return out
}

func (s *SQLiteStore) GetSession(id string) (*Session, error) {
	sess, err := scanSessionRow(s.db.QueryRow(`SELECT id, agent_id, label, created_at, tenant_id, prompt_mode FROM sessions WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *SQLiteStore) UpdateSession(sess Session) (*Session, error) {
	if strings.TrimSpace(sess.ID) == "" {
		return nil, errors.New("id is required")
	}
	cur, err := s.GetSession(sess.ID)
	if err != nil {
		return nil, err
	}
	cur.PromptMode = sess.PromptMode
	_, err = s.db.Exec(`UPDATE sessions SET prompt_mode=? WHERE id=?`, cur.PromptMode, cur.ID)
	if err != nil {
		return nil, err
	}
	return cur, nil
}

func (s *SQLiteStore) DeleteSession(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("id is required")
	}
	_, _ = s.db.Exec(`DELETE FROM messages WHERE session_id=?`, id)
	_, _ = s.db.Exec(`DELETE FROM memories WHERE session_id=?`, id)
	res, err := s.db.Exec(`DELETE FROM sessions WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type sessionScanner interface {
	Scan(dest ...any) error
}

func scanSessionRow(row sessionScanner) (*Session, error) {
	var sess Session
	var ts, tenant string
	if err := row.Scan(&sess.ID, &sess.AgentID, &sess.Label, &ts, &tenant, &sess.PromptMode); err != nil {
		return nil, err
	}
	sess.TenantID = NormalizeTenant(tenant)
	sess.CreatedAt = parseTime(ts)
	return &sess, nil
}

// --- Message ---

func (s *SQLiteStore) AddMessage(m Message) (*Message, error) {
	if m.SessionID == "" {
		return nil, errors.New("session_id is required")
	}
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, m.SessionID).Scan(&cnt); err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, errors.New("session not found")
	}
	if m.Role == "" {
		m.Role = "user"
	}
	m.ID = newID()
	m.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO messages(id, session_id, role, content, created_at) VALUES(?,?,?,?,?)`,
		m.ID, m.SessionID, m.Role, m.Content, formatTime(m.CreatedAt))
	if err != nil {
		return nil, err
	}
	cp := m
	return &cp, nil
}

func (s *SQLiteStore) ListMessages(sessionID string) ([]*Message, error) {
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, sessionID).Scan(&cnt); err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, ErrNotFound
	}
	rows, err := s.db.Query(`SELECT id, session_id, role, content, created_at FROM messages WHERE session_id=? ORDER BY created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		var m Message
		var ts string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &ts); err != nil {
			continue
		}
		m.CreatedAt = parseTime(ts)
		cp := m
		out = append(out, &cp)
	}
	if out == nil {
		out = []*Message{}
	}
	return out, nil
}

// --- Connector ---

func (s *SQLiteStore) CreateConnector(c ConnectorRecord) (*ConnectorRecord, error) {
	if c.Name == "" {
		return nil, errors.New("name is required")
	}
	if c.Transport == "" {
		c.Transport = "http"
	}
	c.CreatedAt = time.Now().UTC()
	en := 0
	if c.Enabled {
		en = 1
	}
	man := ""
	if len(c.ManifestJSON) > 0 {
		man = string(c.ManifestJSON)
	}
	_, err := s.db.Exec(`INSERT INTO connectors(name, transport, endpoint, credential_ref, schema_version, enabled, manifest_url, manifest_json, timeout_ms, retries, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		c.Name, c.Transport, c.Endpoint, c.CredentialRef, c.SchemaVersion, en, c.ManifestURL, man, c.TimeoutMS, c.Retries, formatTime(c.CreatedAt))
	if err != nil {
		return nil, ErrExists
	}
	cp := c
	return &cp, nil
}

func (s *SQLiteStore) ListConnectors() []*ConnectorRecord {
	rows, err := s.db.Query(`SELECT name, transport, endpoint, credential_ref, schema_version, enabled, manifest_url, manifest_json, timeout_ms, retries, created_at FROM connectors ORDER BY created_at`)
	if err != nil {
		return []*ConnectorRecord{}
	}
	defer rows.Close()
	var out []*ConnectorRecord
	for rows.Next() {
		c, err := scanConnector(rows)
		if err != nil {
			continue
		}
		out = append(out, c)
	}
	if out == nil {
		out = []*ConnectorRecord{}
	}
	return out
}

func (s *SQLiteStore) GetConnector(name string) (*ConnectorRecord, error) {
	row := s.db.QueryRow(`SELECT name, transport, endpoint, credential_ref, schema_version, enabled, manifest_url, manifest_json, timeout_ms, retries, created_at FROM connectors WHERE name=?`, name)
	c, err := scanConnector(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *SQLiteStore) SetConnectorEnabled(name string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(`UPDATE connectors SET enabled=? WHERE name=?`, en, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) UpdateConnector(name string, enabled *bool, endpoint *string, credentialRef *string) (*ConnectorRecord, error) {
	cur, err := s.GetConnector(name)
	if err != nil {
		return nil, err
	}
	if enabled != nil {
		cur.Enabled = *enabled
	}
	if endpoint != nil {
		cur.Endpoint = *endpoint
	}
	if credentialRef != nil {
		cur.CredentialRef = *credentialRef
	}
	en := 0
	if cur.Enabled {
		en = 1
	}
	_, err = s.db.Exec(`UPDATE connectors SET enabled=?, endpoint=?, credential_ref=? WHERE name=?`,
		en, cur.Endpoint, cur.CredentialRef, name)
	if err != nil {
		return nil, err
	}
	return cur, nil
}

func (s *SQLiteStore) GetToolFlag(name string) bool {
	name = strings.TrimSpace(name)
	var en int
	err := s.db.QueryRow(`SELECT enabled FROM tool_flags WHERE name=?`, name).Scan(&en)
	if err != nil {
		return false
	}
	return en != 0
}

func (s *SQLiteStore) SetToolFlag(name string, enabled bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name is required")
	}
	en := 0
	if enabled {
		en = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO tool_flags(name, enabled) VALUES(?,?)
		 ON CONFLICT(name) DO UPDATE SET enabled=excluded.enabled`,
		name, en,
	)
	return err
}

func (s *SQLiteStore) ListToolFlags() map[string]bool {
	out := map[string]bool{}
	rows, err := s.db.Query(`SELECT name, enabled FROM tool_flags`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var en int
		if err := rows.Scan(&name, &en); err != nil {
			continue
		}
		out[name] = en != 0
	}
	return out
}

func (s *SQLiteStore) LinkAgentConnector(agentID, connectorName string) error {
	if agentID == "" || connectorName == "" {
		return errors.New("agent_id and connector_name are required")
	}
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE id=?`, agentID).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		return errors.New("agent not found")
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM connectors WHERE name=?`, connectorName).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		return errors.New("connector not found")
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO agent_connectors(agent_id, connector_name) VALUES(?,?)`, agentID, connectorName)
	return err
}

func (s *SQLiteStore) ListAgentConnectors(agentID string) ([]string, error) {
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE id=?`, agentID).Scan(&cnt); err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, ErrNotFound
	}
	rows, err := s.db.Query(`SELECT connector_name FROM agent_connectors WHERE agent_id=? ORDER BY connector_name`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		out = append(out, name)
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanConnector(sc scanner) (*ConnectorRecord, error) {
	var c ConnectorRecord
	var ts string
	var enabled int
	var man string
	err := sc.Scan(&c.Name, &c.Transport, &c.Endpoint, &c.CredentialRef, &c.SchemaVersion, &enabled, &c.ManifestURL, &man, &c.TimeoutMS, &c.Retries, &ts)
	if err != nil {
		return nil, err
	}
	c.Enabled = enabled != 0
	if man != "" {
		c.ManifestJSON = json.RawMessage(man)
	}
	c.CreatedAt = parseTime(ts)
	return &c, nil
}

var sqliteSeq int64

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		n := atomic.AddInt64(&sqliteSeq, 1)
		return time.Now().UTC().Format("20060102") + "-" + itoa(n) + "-" + itoa(time.Now().UnixNano())
	}
	return time.Now().UTC().Format("20060102") + "-" + hex.EncodeToString(b[:])
}

// --- Memory ---

func (s *SQLiteStore) PutMemory(m Memory) (*Memory, error) {
	if m.SessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if strings.TrimSpace(m.Body) == "" {
		return nil, errors.New("body is required")
	}
	if m.Kind == "" {
		m.Kind = KindEpisodic
	}
	m.TenantID = NormalizeTenant(m.TenantID)
	sess, err := s.GetSession(m.SessionID)
	if err != nil {
		return nil, errors.New("session not found")
	}
	if !SameTenant(sess.TenantID, m.TenantID) {
		return nil, errors.New("session not found")
	}
	m.ID = newID()
	m.CreatedAt = time.Now().UTC()
	_, err = s.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id) VALUES(?,?,?,?,?,?)`,
		m.ID, m.SessionID, m.Kind, m.Body, formatTime(m.CreatedAt), m.TenantID)
	if err != nil {
		return nil, err
	}
	cp := m
	return &cp, nil
}

func (s *SQLiteStore) ListMemories(sessionID string) ([]*Memory, error) {
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, sessionID).Scan(&cnt); err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, ErrNotFound
	}
	rows, err := s.db.Query(`SELECT id, session_id, kind, body, created_at, tenant_id FROM memories WHERE session_id=? ORDER BY created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	if out == nil {
		out = []*Memory{}
	}
	return out, nil
}

func (s *SQLiteStore) SaveSummary(sessionID, body string) (*Memory, error) {
	if sessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("body is required")
	}
	sess, err := s.GetSession(sessionID)
	if err != nil {
		return nil, errors.New("session not found")
	}
	row := s.db.QueryRow(`SELECT id, session_id, kind, body, created_at, tenant_id FROM memories WHERE session_id=? AND kind=? ORDER BY created_at DESC LIMIT 1`, sessionID, KindEpisodic)
	existing, err := scanMemory(row)
	if err == nil && existing != nil {
		existing.Body = body
		existing.CreatedAt = time.Now().UTC()
		_, err = s.db.Exec(`UPDATE memories SET body=?, created_at=? WHERE id=?`, existing.Body, formatTime(existing.CreatedAt), existing.ID)
		if err != nil {
			return nil, err
		}
		return existing, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return s.PutMemory(Memory{SessionID: sessionID, Kind: KindEpisodic, Body: body, TenantID: sess.TenantID})
}

func (s *SQLiteStore) LatestSummary(sessionID string) (*Memory, error) {
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, sessionID).Scan(&cnt); err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, ErrNotFound
	}
	row := s.db.QueryRow(`SELECT id, session_id, kind, body, created_at, tenant_id FROM memories WHERE session_id=? AND kind=? ORDER BY created_at DESC LIMIT 1`, sessionID, KindEpisodic)
	m, err := scanMemory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *SQLiteStore) SearchMemory(q string) ([]SearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []SearchHit{}, nil
	}
	if s.fts {
		if hits, err := s.searchFTS(q); err == nil {
			return hits, nil
		}
	}
	return s.searchInstr(q)
}

func (s *SQLiteStore) searchFTS(q string) ([]SearchHit, error) {
	phrase := strings.ReplaceAll(q, `"`, `""`)
	rows, err := s.db.Query(`SELECT id, session_id, kind, snippet(memory_fts, 3, '', '', '…', 16)
		FROM memory_fts WHERE memory_fts MATCH ? ORDER BY rank LIMIT 50`, `"`+phrase+`"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ID, &h.SessionID, &h.Kind, &h.Snippet); err != nil {
			continue
		}
		out = append(out, h)
	}
	if out == nil {
		out = []SearchHit{}
	}
	return out, nil
}

func (s *SQLiteStore) searchInstr(q string) ([]SearchHit, error) {
	rows, err := s.db.Query(`SELECT id, session_id, kind, body FROM (
			SELECT id, session_id, kind, body FROM memories WHERE instr(lower(body), lower(?)) > 0
			UNION ALL
			SELECT id, session_id, 'message', content FROM messages WHERE instr(lower(content), lower(?)) > 0
		) AS hits LIMIT 50`, q, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchHit
	for rows.Next() {
		var id, sid, kind, body string
		if err := rows.Scan(&id, &sid, &kind, &body); err != nil {
			continue
		}
		out = append(out, SearchHit{ID: id, SessionID: sid, Kind: kind, Snippet: SnippetAround(body, q, 80)})
	}
	if out == nil {
		out = []SearchHit{}
	}
	return out, nil
}

func scanMemory(sc scanner) (*Memory, error) {
	var m Memory
	var ts, tenant string
	if err := sc.Scan(&m.ID, &m.SessionID, &m.Kind, &m.Body, &ts, &tenant); err != nil {
		return nil, err
	}
	m.TenantID = NormalizeTenant(tenant)
	m.CreatedAt = parseTime(ts)
	return &m, nil
}
