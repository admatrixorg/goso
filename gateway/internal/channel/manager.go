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
// per catalog ("telegram") and each tenant's bot ("<tenant>/telegram/<name>",
// "<tenant>/zalo-oa/<name>").
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

	ctx  context.Context
	bots map[string]liveBot
	// newTelegram and newZaloBot build tenant bots. Tests replace them.
	newTelegram func(name string) *Telegram
	newZaloBot  func(name string) *ZaloBot
	newLark     func(name string) *Lark
}

// liveBot is a tenant bot the manager starts and stops.
type liveBot interface {
	Start(ctx context.Context, mgr *Manager)
	Stop()
}

// liveCatalogs are the catalogs whose tenant bots run live, each started when
// enabled with a bot token.
var liveCatalogs = map[string]bool{"telegram": true, "zalo-oa": true, "feishu": true}

// liveCredsSet reports a tenant bot that has what it needs to start: a bot
// token, or for Lark an app id and app secret.
func liveCredsSet(st store.StoreIface, name string) bool {
	if CatalogOf(name) == "feishu" {
		return larkCredsSet(st, name)
	}
	_, _, set := Credential(st, name, KindBot, nil)
	return set
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{
		running:   map[string]bool{},
		failed:    map[string]bool{},
		lastErr:   map[string]string{},
		transport: map[string]string{},
		bots:      map[string]liveBot{},
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
	bots := m.bots
	m.bots = map[string]liveBot{}
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

func (m *Manager) buildZaloBot(name string) *ZaloBot {
	if m.newZaloBot != nil {
		return m.newZaloBot(name)
	}
	z := &ZaloBot{Instance: name, Store: m.store(), LLM: m.LLM, Meter: m.Meter}
	if m.Telegram != nil {
		if z.LLM == nil {
			z.LLM = m.Telegram.LLM
		}
		if z.Meter == nil {
			z.Meter = m.Telegram.Meter
		}
	}
	return z
}

func (m *Manager) buildLark(name string) *Lark {
	if m.newLark != nil {
		return m.newLark(name)
	}
	l := &Lark{Instance: name, Store: m.store(), LLM: m.LLM, Meter: m.Meter}
	if m.Telegram != nil {
		if l.LLM == nil {
			l.LLM = m.Telegram.LLM
		}
		if l.Meter == nil {
			l.Meter = m.Telegram.Meter
		}
	}
	return l
}

func (m *Manager) buildBot(name string) liveBot {
	switch CatalogOf(name) {
	case "zalo-oa":
		return m.buildZaloBot(name)
	case "feishu":
		return m.buildLark(name)
	}
	return m.buildTelegram(name)
}

// Reconcile starts every enabled tenant bot (Telegram, Zalo Bot, Lark) that has its credentials and
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
		if cfg == nil || !cfg.Enabled || !liveCatalogs[CatalogOf(cfg.Name)] {
			continue
		}
		if _, owned := TenantOf(cfg.Name); !owned {
			continue
		}
		if liveCredsSet(st, cfg.Name) {
			want[cfg.Name] = true
		}
	}
	var stop []liveBot
	var start []liveBot
	m.mu.Lock()
	for name, t := range m.bots {
		if !want[name] {
			stop = append(stop, t)
			delete(m.bots, name)
			m.running[name] = false
			m.failed[name] = false
			m.lastErr[name] = ""
		}
	}
	for name := range want {
		if _, ok := m.bots[name]; ok {
			continue
		}
		t := m.buildBot(name)
		m.bots[name] = t
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
	t := m.bots[name]
	delete(m.bots, name)
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
	t, _ := m.bots[name].(*Telegram)
	return t
}

// ZaloBot returns a tenant's running Zalo Bot, or nil.
func (m *Manager) ZaloBot(name string) *ZaloBot {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	z, _ := m.bots[name].(*ZaloBot)
	return z
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

// Lark returns a tenant's running Lark bot, or nil.
func (m *Manager) Lark(name string) *Lark {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	l, _ := m.bots[name].(*Lark)
	return l
}
