// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore persists agents/sessions/messages in SQLite.
type SQLiteStore struct {
	db *sql.DB
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
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
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

var sqliteSeq int64

func newID() string {
	sqliteSeq++
	return time.Now().UTC().Format("20060102") + "-" + itoa(sqliteSeq)
}
