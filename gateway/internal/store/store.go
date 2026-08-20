// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"sync"
	"time"
)

// Agent represents an AI agent.
type Agent struct {
	ID          string    `json:"id"`
	AgentKey    string    `json:"agent_key"`
	DisplayName string    `json:"display_name"`
	Model       string    `json:"model,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Session represents a conversation session.
type Session struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Label     string    `json:"label,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Message represents a chat message.
type Message struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"` // user | assistant
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// StoreIface is satisfied by both Store (memory) and SQLiteStore.
type StoreIface interface {
	CreateAgent(Agent) (*Agent, error)
	ListAgents() []*Agent
	GetAgent(string) (*Agent, error)
	CreateSession(Session) (*Session, error)
	ListSessions() []*Session
	GetSession(string) (*Session, error)
	AddMessage(Message) (*Message, error)
	ListMessages(string) ([]*Message, error)
}

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("already exists")
)

// Store is an in-memory store. Safe for concurrent use.
type Store struct {
	mu       sync.RWMutex
	agents   map[string]*Agent
	sessions map[string]*Session
	messages map[string][]*Message // session_id -> messages
	seq      int64
}

func Open(path string) (StoreIface, func() error, error) {
	if path == "" || path == ":memory:" {
		s := New()
		return s, func() error { return nil }, nil
	}
	s, err := OpenSQLite(path)
	if err != nil {
		return nil, nil, err
	}
	return s, s.Close, nil
}

func New() *Store {
	return &Store{
		agents:   make(map[string]*Agent),
		sessions: make(map[string]*Session),
		messages: make(map[string][]*Message),
	}
}

func (s *Store) nextID() string {
	s.seq++
	return time.Now().UTC().Format("20060102") + "-" + itoa(s.seq)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 20)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// --- Agent ---

func (s *Store) CreateAgent(a Agent) (*Agent, error) {
	if a.AgentKey == "" {
		return nil, errors.New("agent_key is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.agents {
		if v.AgentKey == a.AgentKey {
			return nil, ErrExists
		}
	}
	a.ID = s.nextID()
	a.CreatedAt = time.Now().UTC()
	cp := a
	s.agents[cp.ID] = &cp
	return &cp, nil
}

func (s *Store) ListAgents() []*Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Agent, 0, len(s.agents))
	for _, v := range s.agents {
		cp := *v
		out = append(out, &cp)
	}
	return out
}

func (s *Store) GetAgent(id string) (*Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.agents[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}

// --- Session ---

func (s *Store) CreateSession(sess Session) (*Session, error) {
	if sess.AgentID == "" {
		return nil, errors.New("agent_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[sess.AgentID]; !ok {
		return nil, errors.New("agent not found")
	}
	sess.ID = s.nextID()
	sess.CreatedAt = time.Now().UTC()
	cp := sess
	s.sessions[cp.ID] = &cp
	return &cp, nil
}

func (s *Store) ListSessions() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Session, 0, len(s.sessions))
	for _, v := range s.sessions {
		cp := *v
		out = append(out, &cp)
	}
	return out
}

func (s *Store) GetSession(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}

// --- Message ---

func (s *Store) AddMessage(m Message) (*Message, error) {
	if m.SessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if m.Role == "" {
		m.Role = "user"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[m.SessionID]; !ok {
		return nil, errors.New("session not found")
	}
	m.ID = s.nextID()
	m.CreatedAt = time.Now().UTC()
	cp := m
	s.messages[cp.SessionID] = append(s.messages[cp.SessionID], &cp)
	return &cp, nil
}

func (s *Store) ListMessages(sessionID string) ([]*Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	msgs := s.messages[sessionID]
	out := make([]*Message, len(msgs))
	for i, m := range msgs {
		cp := *m
		out[i] = &cp
	}
	if out == nil {
		out = []*Message{}
	}
	return out, nil
}
