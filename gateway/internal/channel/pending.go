// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

var (
	ErrPendingNotFound        = errors.New("not found")
	ErrPendingBusy            = errors.New("compact in progress")
	ErrPendingConfirm         = errors.New("confirm does not match")
	ErrPendingConfirmRequired = errors.New("confirm is required")
)

const (
	maxPendingChannel = 64
	maxPendingDest    = 128
)

// Enqueue is one inbound message to hold while an agent is busy or offline.
// Text is never stored — listings cannot leak payloads.
type Enqueue struct {
	Channel  string
	Dest     string
	AgentID  string
	TenantID string
	At       time.Time
}

// PublicGroup is the GET listing row. No token/code/secret/content fields.
type PublicGroup struct {
	ID            string    `json:"id"`
	Channel       string    `json:"channel"`
	Dest          string    `json:"dest"`
	AgentID       string    `json:"agent_id,omitempty"`
	Count         int       `json:"count"`
	OldestAt      time.Time `json:"oldest_at"`
	NewestAt      time.Time `json:"newest_at"`
	AgeMS         int64     `json:"age_ms"`
	Compacted     bool      `json:"compacted,omitempty"`
	Compacting    bool      `json:"compacting,omitempty"`
	CompactedFrom int       `json:"compacted_from,omitempty"`
}

type pendingGroup struct {
	ID            string
	Channel       string
	Dest          string
	AgentID       string
	TenantID      string
	Count         int
	OldestAt      time.Time
	NewestAt      time.Time
	Compacted     bool
	Compacting    bool
	CompactedFrom int
}

// Pending is an in-memory per-group channel buffer.
type Pending struct {
	mu     sync.Mutex
	seq    atomic.Int64
	groups map[string]*pendingGroup // id -> group
	index  map[string]string        // tenant|channel|dest|agent -> id
	hook   func()                   // test: runs while compacting
}

// HookBusy marks the named group compacting (tests).
func (p *Pending) HookBusy() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, g := range p.groups {
		if g != nil {
			g.Compacting = true
			return
		}
	}
}

var (
	defaultPendingMu sync.Mutex
	defaultPending   = NewPending()
)

// NewPending returns an empty buffer.
func NewPending() *Pending {
	return &Pending{
		groups: map[string]*pendingGroup{},
		index:  map[string]string{},
	}
}

// DefaultPending is the process-wide buffer used by inbound + HTTP.
func DefaultPending() *Pending {
	defaultPendingMu.Lock()
	defer defaultPendingMu.Unlock()
	return defaultPending
}

// SetDefaultPending replaces the process-wide buffer (tests).
func SetDefaultPending(p *Pending) {
	defaultPendingMu.Lock()
	defer defaultPendingMu.Unlock()
	if p == nil {
		p = NewPending()
	}
	defaultPending = p
}

func groupKey(tenant, channel, dest, agentID string) string {
	return store.NormalizeTenant(tenant) + "|" + channel + "|" + dest + "|" + agentID
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if n > 0 && len(s) > n {
		return s[:n]
	}
	return s
}

func (p *Pending) nextID() string {
	n := p.seq.Add(1)
	return "pg_" + itoaPending(n)
}

func itoaPending(n int64) string {
	if n <= 0 {
		return "0"
	}
	b := make([]byte, 0, 20)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// Enqueue holds one message in the matching group. Content is discarded.
func (p *Pending) Enqueue(in Enqueue) PublicGroup {
	if p == nil {
		return PublicGroup{}
	}
	ch := clip(in.Channel, maxPendingChannel)
	dest := clip(in.Dest, maxPendingDest)
	if ch == "" || dest == "" {
		return PublicGroup{}
	}
	agentID := strings.TrimSpace(in.AgentID)
	tenant := store.NormalizeTenant(in.TenantID)
	at := in.At
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	key := groupKey(tenant, ch, dest, agentID)

	p.mu.Lock()
	defer p.mu.Unlock()
	id := p.index[key]
	g := p.groups[id]
	if g == nil {
		id = p.nextID()
		g = &pendingGroup{
			ID:       id,
			Channel:  ch,
			Dest:     dest,
			AgentID:  agentID,
			TenantID: tenant,
			Count:    1,
			OldestAt: at,
			NewestAt: at,
		}
		p.groups[id] = g
		p.index[key] = id
		return publicOf(g, at)
	}
	g.Count++
	g.Compacted = false
	if at.Before(g.OldestAt) {
		g.OldestAt = at
	}
	if at.After(g.NewestAt) {
		g.NewestAt = at
	}
	return publicOf(g, at)
}

// Has reports whether a group already exists for this destination.
func (p *Pending) Has(tenant, channel, dest, agentID string) bool {
	if p == nil {
		return false
	}
	ch := clip(channel, maxPendingChannel)
	d := clip(dest, maxPendingDest)
	if ch == "" || d == "" {
		return false
	}
	key := groupKey(store.NormalizeTenant(tenant), ch, d, strings.TrimSpace(agentID))
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.groups[p.index[key]]
	return ok
}

// List returns public groups for tenant, newest first. Never includes payloads.
func (p *Pending) List(tenant string, now time.Time) []PublicGroup {
	if p == nil {
		return []PublicGroup{}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	want := store.NormalizeTenant(tenant)
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]PublicGroup, 0, len(p.groups))
	for _, g := range p.groups {
		if g == nil || !store.SameTenant(g.TenantID, want) {
			continue
		}
		out = append(out, publicOf(g, now))
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].NewestAt.After(out[i].NewestAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// Get returns one public group or ErrPendingNotFound.
func (p *Pending) Get(id, tenant string, now time.Time) (PublicGroup, error) {
	if p == nil {
		return PublicGroup{}, ErrPendingNotFound
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	g := p.groups[strings.TrimSpace(id)]
	if g == nil || !store.SameTenant(g.TenantID, store.NormalizeTenant(tenant)) {
		return PublicGroup{}, ErrPendingNotFound
	}
	return publicOf(g, now), nil
}

// ConfirmOK is true when typed matches the group id, dest, or channel:dest.
func ConfirmOK(typed string, g PublicGroup) bool {
	v := strings.TrimSpace(typed)
	if v == "" {
		return false
	}
	if v == g.ID || v == g.Dest {
		return true
	}
	return v == g.Channel+":"+g.Dest
}

// Compact collapses a group to a single stub. Requires a matching confirm name.
func (p *Pending) Compact(id, tenant, confirm string) (PublicGroup, error) {
	if p == nil {
		return PublicGroup{}, ErrPendingNotFound
	}
	if strings.TrimSpace(confirm) == "" {
		return PublicGroup{}, ErrPendingConfirmRequired
	}
	now := time.Now().UTC()
	p.mu.Lock()
	g := p.groups[strings.TrimSpace(id)]
	if g == nil || !store.SameTenant(g.TenantID, store.NormalizeTenant(tenant)) {
		p.mu.Unlock()
		return PublicGroup{}, ErrPendingNotFound
	}
	if !ConfirmOK(confirm, publicOf(g, now)) {
		p.mu.Unlock()
		return PublicGroup{}, ErrPendingConfirm
	}
	if g.Compacting {
		p.mu.Unlock()
		return PublicGroup{}, ErrPendingBusy
	}
	g.Compacting = true
	hook := p.hook
	p.mu.Unlock()

	if hook != nil {
		hook()
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	g.Compacting = false
	if g.Count < 1 {
		g.Count = 1
	}
	if g.CompactedFrom < g.Count {
		g.CompactedFrom = g.Count
	}
	g.Count = 1
	g.Compacted = true
	return publicOf(g, time.Now().UTC()), nil
}

// Clear deletes a group. Requires a matching confirm name.
func (p *Pending) Clear(id, tenant, confirm string) error {
	if p == nil {
		return ErrPendingNotFound
	}
	if strings.TrimSpace(confirm) == "" {
		return ErrPendingConfirmRequired
	}
	now := time.Now().UTC()
	p.mu.Lock()
	defer p.mu.Unlock()
	g := p.groups[strings.TrimSpace(id)]
	if g == nil || !store.SameTenant(g.TenantID, store.NormalizeTenant(tenant)) {
		return ErrPendingNotFound
	}
	if g.Compacting {
		return ErrPendingBusy
	}
	if !ConfirmOK(confirm, publicOf(g, now)) {
		return ErrPendingConfirm
	}
	key := groupKey(g.TenantID, g.Channel, g.Dest, g.AgentID)
	delete(p.groups, g.ID)
	delete(p.index, key)
	return nil
}

func publicOf(g *pendingGroup, now time.Time) PublicGroup {
	age := now.Sub(g.OldestAt).Milliseconds()
	if age < 0 {
		age = 0
	}
	return PublicGroup{
		ID:            g.ID,
		Channel:       g.Channel,
		Dest:          g.Dest,
		AgentID:       g.AgentID,
		Count:         g.Count,
		OldestAt:      g.OldestAt,
		NewestAt:      g.NewestAt,
		AgeMS:         age,
		Compacted:     g.Compacted,
		Compacting:    g.Compacting,
		CompactedFrom: g.CompactedFrom,
	}
}

// BufferIfNeeded enqueues when the agent is offline/disabled or the dest already has a buffer.
func BufferIfNeeded(p *Pending, agent *store.Agent, channel, dest string) bool {
	if p == nil {
		p = DefaultPending()
	}
	ch := clip(channel, maxPendingChannel)
	d := clip(dest, maxPendingDest)
	if ch == "" || d == "" {
		return false
	}
	agentID := ""
	tenantID := ""
	enabled := false
	if agent != nil {
		agentID = agent.ID
		tenantID = agent.TenantID
		enabled = agent.Enabled
	}
	if enabled && !p.Has(tenantID, ch, d, agentID) {
		return false
	}
	p.Enqueue(Enqueue{Channel: ch, Dest: d, AgentID: agentID, TenantID: tenantID})
	return true
}
