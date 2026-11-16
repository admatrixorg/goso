// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"sync"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Manager tracks Start/Stop and health by channel name: the older single bot
// per catalog ("telegram") and each tenant's bot ("<tenant>/telegram/<name>").
type Manager struct {
	mu        sync.Mutex
	running   map[string]bool
	failed    map[string]bool
	lastErr   map[string]string
	transport map[string]string
	Telegram  *Telegram

	// Store, LLM and Meter build tenant bots. When nil they come from Telegram.
	Store store.StoreIface
	LLM   llm.Provider
	Meter *billing.Store

	ctx       context.Context
	telegrams map[string]*Telegram
	// newTelegram builds a tenant bot. Tests replace it.
	newTelegram func(name string) *Telegram
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{
		running:   map[string]bool{},
		failed:    map[string]bool{},
		lastErr:   map[string]string{},
		transport: map[string]string{},
		telegrams: map[string]*Telegram{},
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
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	m.Reconcile()
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
	bots := m.telegrams
	m.telegrams = map[string]*Telegram{}
	m.ctx = nil
	m.running = map[string]bool{}
	m.mu.Unlock()
	for _, t := range bots {
		t.Stop()
	}
}

func (m *Manager) store() store.StoreIface {
	if m.Store != nil {
		return m.Store
	}
	if m.Telegram != nil {
		return m.Telegram.Store
	}
	return nil
}

func (m *Manager) buildTelegram(name string) *Telegram {
	if m.newTelegram != nil {
		return m.newTelegram(name)
	}
	t := &Telegram{Instance: name, Store: m.store(), LLM: m.LLM, Meter: m.Meter}
	if m.Telegram != nil {
		if t.LLM == nil {
			t.LLM = m.Telegram.LLM
		}
		if t.Meter == nil {
			t.Meter = m.Telegram.Meter
		}
		t.HTTPClient = m.Telegram.HTTPClient
		t.APIBase = m.Telegram.APIBase
	}
	return t
}

// Reconcile starts every enabled tenant Telegram bot that has a token and
// stops every running one that is no longer enabled or lost its token. It does
// nothing before StartAll, after StopAll, or in Lite mode. Bots start in the
// background so an HTTP caller does not wait on Telegram.
func (m *Manager) Reconcile() {
	if m == nil || store.LiteEnabled() {
		return
	}
	st := m.store()
	m.mu.Lock()
	ctx := m.ctx
	m.mu.Unlock()
	if ctx == nil || st == nil {
		return
	}
	want := map[string]bool{}
	for _, cfg := range st.ListChannelConfigs() {
		if cfg == nil || !cfg.Enabled || CatalogOf(cfg.Name) != "telegram" {
			continue
		}
		if _, owned := TenantOf(cfg.Name); !owned {
			continue
		}
		if _, _, set := Credential(st, cfg.Name, KindBot, nil); set {
			want[cfg.Name] = true
		}
	}
	var stop []*Telegram
	var start []*Telegram
	m.mu.Lock()
	for name, t := range m.telegrams {
		if !want[name] {
			stop = append(stop, t)
			delete(m.telegrams, name)
			m.running[name] = false
			m.failed[name] = false
			m.lastErr[name] = ""
		}
	}
	for name := range want {
		if _, ok := m.telegrams[name]; ok {
			continue
		}
		t := m.buildTelegram(name)
		m.telegrams[name] = t
		start = append(start, t)
	}
	m.mu.Unlock()
	for _, t := range stop {
		t.Stop()
	}
	for _, t := range start {
		go t.Start(ctx, m)
	}
}

// Restart stops one tenant bot and starts it again if it is still wanted, so a
// new token takes effect.
func (m *Manager) Restart(name string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	t := m.telegrams[name]
	delete(m.telegrams, name)
	m.mu.Unlock()
	if t != nil {
		t.Stop()
	}
	m.Reconcile()
}

// TelegramBot returns the running bot for a channel name: the older single bot
// for "telegram", or a tenant bot. Nil when it is not running.
func (m *Manager) TelegramBot(name string) *Telegram {
	if m == nil {
		return nil
	}
	if name == "telegram" {
		return m.Telegram
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.telegrams[name]
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
