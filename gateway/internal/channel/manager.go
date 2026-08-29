// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"sync"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Manager tracks Start/Stop and health for one instance per catalog name.
type Manager struct {
	mu        sync.Mutex
	running   map[string]bool
	failed    map[string]bool
	lastErr   map[string]string
	transport map[string]string
	Telegram  *Telegram
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{
		running:   map[string]bool{},
		failed:    map[string]bool{},
		lastErr:   map[string]string{},
		transport: map[string]string{},
	}
}

// StartAll starts phase-1 live listeners. Phase-2 stays parked. Lite skips Start.
func (m *Manager) StartAll(ctx context.Context) {
	if m == nil {
		return
	}
	if store.LiteEnabled() {
		return
	}
	if m.Telegram != nil {
		m.Telegram.Start(ctx, m)
	}
}

// StopAll stops started listeners.
func (m *Manager) StopAll() {
	if m == nil {
		return
	}
	if m.Telegram != nil {
		m.Telegram.Stop()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = map[string]bool{}
}

// SetRunning marks health running (clears failed).
func (m *Manager) SetRunning(name, transport string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running[name] = true
	m.failed[name] = false
	m.lastErr[name] = ""
	if transport != "" {
		m.transport[name] = transport
	}
}

// SetFailed records a redacted public error.
func (m *Manager) SetFailed(name, err string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running[name] = false
	m.failed[name] = true
	m.lastErr[name] = redactErr(err)
}

// Running reports an active listener.
func (m *Manager) Running(name string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[name]
}

// Failed reports a start/runtime failure.
func (m *Manager) Failed(name string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failed[name]
}

// LastError is the public redacted error.
func (m *Manager) LastError(name string) string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr[name]
}

// Transport overrides catalog default when set.
func (m *Manager) Transport(name string) string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.transport[name]
}


