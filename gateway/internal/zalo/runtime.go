// Package zalo runs a personal account session: it logs in with the stored
// cookie set, keeps the listener alive, and exposes what it receives.
package zalo

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/zalo/protocol"
)

// Status is the state of one running session.
type Status struct {
	Channel   string                   `json:"channel"`
	Running   bool                     `json:"running"`
	UID       string                   `json:"uid,omitempty"`
	Name      string                   `json:"name,omitempty"`
	StartedAt string                   `json:"started_at,omitempty"`
	LastError string                   `json:"last_error,omitempty"`
	Received  int                      `json:"received"`
	Last      *protocol.InboundMessage `json:"last,omitempty"`
}

type session struct {
	mu       sync.Mutex
	sess     *protocol.Session
	listener *protocol.Listener
	cancel   context.CancelFunc
	uid      string
	name     string
	started  time.Time
	lastErr  string
	inbox    []protocol.InboundMessage
}

// Manager owns the running sessions by channel name.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*session
	inboxCap int
}

// NewManager builds an empty manager.
func NewManager() *Manager {
	return &Manager{sessions: map[string]*session{}, inboxCap: 200}
}

// Start logs in with the credential and runs the listener until ctx ends.
func (m *Manager) Start(ctx context.Context, channel string, cred *protocol.Credential) (*Status, error) {
	if m == nil {
		return nil, fmt.Errorf("zalo: manager is nil")
	}
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return nil, fmt.Errorf("zalo: channel name is required")
	}
	sess, err := protocol.NewSessionWithCredential(cred)
	if err != nil {
		return nil, err
	}
	loginCtx, cancelLogin := context.WithTimeout(ctx, 30*time.Second)
	in, lerr := sess.GetLoginInfo(loginCtx)
	cancelLogin()
	if lerr != nil {
		return nil, fmt.Errorf("zalo: login failed: %w", lerr)
	}
	if urls := sess.WebSocketURLs(); len(urls) > 0 {
		sess.WSURL = pickWSURL(urls[0])
	}
	s := &session{sess: sess, uid: in.UID, name: in.Name, started: time.Now().UTC()}
	ln, lerr2 := protocol.NewListener(sess, func(msg protocol.InboundMessage) {
		s.mu.Lock()
		s.inbox = append(s.inbox, msg)
		if len(s.inbox) > m.inboxCap {
			s.inbox = s.inbox[len(s.inbox)-m.inboxCap:]
		}
		s.mu.Unlock()
	})
	if lerr2 != nil {
		return nil, lerr2
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.listener = ln
	s.cancel = cancel
	m.mu.Lock()
	if old, ok := m.sessions[channel]; ok && old.cancel != nil {
		old.cancel()
	}
	m.sessions[channel] = s
	m.mu.Unlock()
	go func() {
		if rerr := ln.Start(runCtx); rerr != nil {
			s.mu.Lock()
			s.lastErr = rerr.Error()
			s.mu.Unlock()
		}
	}()
	return s.status(channel), nil
}

// Stop ends one session.
func (m *Manager) Stop(channel string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	s := m.sessions[strings.TrimSpace(channel)]
	delete(m.sessions, strings.TrimSpace(channel))
	m.mu.Unlock()
	if s != nil {
		if s.listener != nil {
			s.listener.Stop()
		}
		if s.cancel != nil {
			s.cancel()
		}
	}
}

// Status reports one channel, or a stopped status when it is not running.
func (m *Manager) Status(channel string) Status {
	if m == nil {
		return Status{Channel: channel}
	}
	m.mu.Lock()
	s := m.sessions[strings.TrimSpace(channel)]
	m.mu.Unlock()
	if s == nil {
		return Status{Channel: channel}
	}
	return *s.status(channel)
}

// Inbox returns the newest messages for one channel.
func (m *Manager) Inbox(channel string, limit int) []protocol.InboundMessage {
	if m == nil {
		return []protocol.InboundMessage{}
	}
	m.mu.Lock()
	s := m.sessions[strings.TrimSpace(channel)]
	m.mu.Unlock()
	if s == nil {
		return []protocol.InboundMessage{}
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.inbox) <= limit {
		out := make([]protocol.InboundMessage, len(s.inbox))
		copy(out, s.inbox)
		return out
	}
	out := make([]protocol.InboundMessage, limit)
	copy(out, s.inbox[len(s.inbox)-limit:])
	return out
}

func (s *session) status(channel string) *Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &Status{
		Channel: channel, Running: true, UID: s.uid, Name: s.name,
		StartedAt: s.started.Format(time.RFC3339), LastError: s.lastErr,
		Received: len(s.inbox),
	}
	if n := len(s.inbox); n > 0 {
		last := s.inbox[n-1]
		st.Last = &last
	}
	return st
}

// pickWSURL keeps a wss scheme; Zalo hands out a bare host in some responses.
func pickWSURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if strings.HasPrefix(raw, "wss://") || strings.HasPrefix(raw, "ws://") {
		return raw
	}
	return "wss://" + strings.TrimPrefix(raw, "//")
}
