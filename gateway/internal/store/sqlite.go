// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore persists agents/sessions/messages in SQLite.
type SQLiteStore struct {
	db  *sql.DB
	fts bool
}

// OpenSQLite opens (and migrates) a SQLite DB at path. Use ":memory:" for in-memory.
func OpenSQLite(path string) (*SQLiteStore, error) {
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
	s := &SQLiteStore{db: db}
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
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			label TEXT,
			created_at TEXT NOT NULL,
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
			FOREIGN KEY(session_id) REFERENCES sessions(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_session ON memories(session_id, created_at)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	s.initFTS()
	return nil
}

func (s *SQLiteStore) initFTS() {
	s.fts = false
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
	a.ID = newID()
	a.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO agents(id, agent_key, display_name, model, created_at) VALUES(?,?,?,?,?)`,
		a.ID, a.AgentKey, a.DisplayName, a.Model, formatTime(a.CreatedAt))
	if err != nil {
		// unique violation
		return nil, ErrExists
	}
	cp := a
	return &cp, nil
}

func (s *SQLiteStore) ListAgents() []*Agent {
	rows, err := s.db.Query(`SELECT id, agent_key, display_name, model, created_at FROM agents ORDER BY created_at`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Agent
	for rows.Next() {
		var a Agent
		var ts string
		if err := rows.Scan(&a.ID, &a.AgentKey, &a.DisplayName, &a.Model, &ts); err != nil {
			continue
		}
		a.CreatedAt = parseTime(ts)
		cp := a
		out = append(out, &cp)
	}
	if out == nil {
		out = []*Agent{}
	}
	return out
}

func (s *SQLiteStore) GetAgent(id string) (*Agent, error) {
	var a Agent
	var ts string
	err := s.db.QueryRow(`SELECT id, agent_key, display_name, model, created_at FROM agents WHERE id=?`, id).
		Scan(&a.ID, &a.AgentKey, &a.DisplayName, &a.Model, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.CreatedAt = parseTime(ts)
	return &a, nil
}

// --- Session ---

func (s *SQLiteStore) CreateSession(sess Session) (*Session, error) {
	if sess.AgentID == "" {
		return nil, errors.New("agent_id is required")
	}
	// check agent exists
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE id=?`, sess.AgentID).Scan(&cnt); err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, errors.New("agent not found")
	}
	sess.ID = newID()
	sess.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO sessions(id, agent_id, label, created_at) VALUES(?,?,?,?)`,
		sess.ID, sess.AgentID, sess.Label, formatTime(sess.CreatedAt))
	if err != nil {
		return nil, err
	}
	cp := sess
	return &cp, nil
}

func (s *SQLiteStore) ListSessions() []*Session {
	rows, err := s.db.Query(`SELECT id, agent_id, label, created_at FROM sessions ORDER BY created_at`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Session
	for rows.Next() {
		var sess Session
		var ts string
		if err := rows.Scan(&sess.ID, &sess.AgentID, &sess.Label, &ts); err != nil {
			continue
		}
		sess.CreatedAt = parseTime(ts)
		cp := sess
		out = append(out, &cp)
	}
	if out == nil {
		out = []*Session{}
	}
	return out
}

func (s *SQLiteStore) GetSession(id string) (*Session, error) {
	var sess Session
	var ts string
	err := s.db.QueryRow(`SELECT id, agent_id, label, created_at FROM sessions WHERE id=?`, id).
		Scan(&sess.ID, &sess.AgentID, &sess.Label, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
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
	sqliteSeq++
	return time.Now().UTC().Format("20060102") + "-" + itoa(sqliteSeq)
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
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, m.SessionID).Scan(&cnt); err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, errors.New("session not found")
	}
	m.ID = newID()
	m.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at) VALUES(?,?,?,?,?)`,
		m.ID, m.SessionID, m.Kind, m.Body, formatTime(m.CreatedAt))
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
	rows, err := s.db.Query(`SELECT id, session_id, kind, body, created_at FROM memories WHERE session_id=? ORDER BY created_at`, sessionID)
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
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, sessionID).Scan(&cnt); err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, errors.New("session not found")
	}
	row := s.db.QueryRow(`SELECT id, session_id, kind, body, created_at FROM memories WHERE session_id=? AND kind=? ORDER BY created_at DESC LIMIT 1`, sessionID, KindEpisodic)
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
	return s.PutMemory(Memory{SessionID: sessionID, Kind: KindEpisodic, Body: body})
}

func (s *SQLiteStore) LatestSummary(sessionID string) (*Memory, error) {
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, sessionID).Scan(&cnt); err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, ErrNotFound
	}
	row := s.db.QueryRow(`SELECT id, session_id, kind, body, created_at FROM memories WHERE session_id=? AND kind=? ORDER BY created_at DESC LIMIT 1`, sessionID, KindEpisodic)
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
		) LIMIT 50`, q, q)
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
	var ts string
	if err := sc.Scan(&m.ID, &m.SessionID, &m.Kind, &m.Body, &ts); err != nil {
		return nil, err
	}
	m.CreatedAt = parseTime(ts)
	return &m, nil
}
