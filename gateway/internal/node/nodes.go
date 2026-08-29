// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package node

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrConfirm         = errors.New("confirm does not match")
	ErrConfirmRequired = errors.New("confirm is required")
	ErrDisplayRequired = errors.New("display is required")
	ErrExpired         = errors.New("pairing request expired")
	ErrStatus          = errors.New("node not pending")
	ErrNotPaired       = errors.New("node not paired")
	ErrCap             = errors.New("too many pending pairing requests")
)

const (
	pendingTTL    = 10 * time.Minute
	staleAfter    = 5 * time.Minute
	maxDisplay    = 64
	maxPending    = 32
	kindDashboard = "dashboard"
	statusPending = "pending"
	statusPaired  = "paired"
	statusDenied  = "denied"
	statusRevoked = "revoked"
	healthPending = "pending"
	healthOK      = "ok"
	healthStale   = "stale"
	healthExpired = "expired"
)

// Request is a dashboard device asking for access. Codes and tokens are never stored.
type Request struct {
	Display  string
	Kind     string
	TenantID string
	At       time.Time
}

// Public is the GET row. No token/code/secret fields.
type Public struct {
	ID          string     `json:"id"`
	Display     string     `json:"display"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	Health      string     `json:"health"`
	RequestedAt time.Time  `json:"requested_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ApprovedAt  *time.Time `json:"approved_at,omitempty"`
	LastSeen    time.Time  `json:"last_seen"`
}

type device struct {
	ID          string
	TenantID    string
	Display     string
	Kind        string
	Status      string
	RequestedAt time.Time
	ExpiresAt   time.Time
	ApprovedAt  time.Time
	LastSeen    time.Time
}

// Nodes is an in-memory pending vs paired device registry.
type Nodes struct {
	mu   sync.Mutex
	seq  atomic.Int64
	now  func() time.Time
	rows map[string]*device
}

var (
	defaultMu    sync.Mutex
	defaultNodes = New()
)

// New returns an empty registry.
func New() *Nodes {
	return &Nodes{rows: map[string]*device{}}
}

// Default is the process-wide registry used by HTTP.
func Default() *Nodes {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	return defaultNodes
}

// SetDefault replaces the process-wide registry (tests).
func SetDefault(n *Nodes) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if n == nil {
		n = New()
	}
	defaultNodes = n
}

func (n *Nodes) clock() time.Time {
	if n != nil && n.now != nil {
		return n.now().UTC()
	}
	return time.Now().UTC()
}

func (n *Nodes) nextID() string {
	v := n.seq.Add(1)
	return "nd_" + itoa(v)
}

func itoa(v int64) string {
	if v <= 0 {
		return "0"
	}
	b := make([]byte, 0, 20)
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if max > 0 && len(s) > max {
		return s[:max]
	}
	return s
}

func kindOf(hint string) string {
	k := strings.ToLower(strings.TrimSpace(hint))
	if k == "" {
		return kindDashboard
	}
	return clip(k, 32)
}

// ConfirmOK is true when typed matches id or display.
func ConfirmOK(typed string, row Public) bool {
	v := strings.TrimSpace(typed)
	if v == "" {
		return false
	}
	return v == row.ID || v == row.Display
}

// RequestAccess records a pending device. Pairing codes and tokens are discarded.
func (n *Nodes) RequestAccess(in Request) (Public, error) {
	if n == nil {
		return Public{}, ErrNotFound
	}
	display := clip(in.Display, maxDisplay)
	if display == "" {
		return Public{}, ErrDisplayRequired
	}
	at := in.At
	if at.IsZero() {
		at = n.clock()
	} else {
		at = at.UTC()
	}
	tenant := store.NormalizeTenant(in.TenantID)
	kind := kindOf(in.Kind)

	n.mu.Lock()
	defer n.mu.Unlock()
	pending := 0
	for _, row := range n.rows {
		if row == nil || !store.SameTenant(row.TenantID, tenant) {
			continue
		}
		if row.Status == statusPending {
			pending++
		}
	}
	if pending >= maxPending {
		return Public{}, ErrCap
	}
	row := &device{
		ID:          n.nextID(),
		TenantID:    tenant,
		Display:     display,
		Kind:        kind,
		Status:      statusPending,
		RequestedAt: at,
		ExpiresAt:   at.Add(pendingTTL),
		LastSeen:    at,
	}
	n.rows[row.ID] = row
	return publicNode(row, at), nil
}

// ListPending returns unapproved, unrevoked pending requests for the tenant.
func (n *Nodes) ListPending(tenant string, now time.Time) []Public {
	return n.list(tenant, now, statusPending)
}

// ListPaired returns approved devices for the tenant.
func (n *Nodes) ListPaired(tenant string, now time.Time) []Public {
	return n.list(tenant, now, statusPaired)
}

func (n *Nodes) list(tenant string, now time.Time, status string) []Public {
	if n == nil {
		return []Public{}
	}
	if now.IsZero() {
		now = n.clock()
	} else {
		now = now.UTC()
	}
	tenant = store.NormalizeTenant(tenant)
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]Public, 0)
	for _, row := range n.rows {
		if row == nil || row.Status != status || !store.SameTenant(row.TenantID, tenant) {
			continue
		}
		out = append(out, publicNode(row, now))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RequestedAt.Equal(out[j].RequestedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].RequestedAt.Before(out[j].RequestedAt)
	})
	return out
}

// Get returns one node in the tenant, or not found.
func (n *Nodes) Get(id, tenant string, now time.Time) (Public, error) {
	row, err := n.get(id, tenant)
	if err != nil {
		return Public{}, err
	}
	if now.IsZero() {
		now = n.clock()
	}
	return publicNode(row, now.UTC()), nil
}

func (n *Nodes) get(id, tenant string) (*device, error) {
	if n == nil {
		return nil, ErrNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrNotFound
	}
	tenant = store.NormalizeTenant(tenant)
	n.mu.Lock()
	defer n.mu.Unlock()
	row := n.rows[id]
	if row == nil || !store.SameTenant(row.TenantID, tenant) {
		return nil, ErrNotFound
	}
	cp := *row
	return &cp, nil
}

// Approve marks a pending request paired. Confirm must match id or display.
func (n *Nodes) Approve(id, tenant, confirm string, now time.Time) (Public, error) {
	return n.setStatus(id, tenant, confirm, now, statusPending, statusPaired, ErrStatus)
}

// Deny marks a pending request denied.
func (n *Nodes) Deny(id, tenant, confirm string, now time.Time) (Public, error) {
	return n.setStatus(id, tenant, confirm, now, statusPending, statusDenied, ErrStatus)
}

// Revoke unpairs an approved device.
func (n *Nodes) Revoke(id, tenant, confirm string, now time.Time) (Public, error) {
	return n.setStatus(id, tenant, confirm, now, statusPaired, statusRevoked, ErrNotPaired)
}

func (n *Nodes) setStatus(id, tenant, confirm string, now time.Time, from, to string, wrong error) (Public, error) {
	if strings.TrimSpace(confirm) == "" {
		return Public{}, ErrConfirmRequired
	}
	if n == nil {
		return Public{}, ErrNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Public{}, ErrNotFound
	}
	if now.IsZero() {
		now = n.clock()
	} else {
		now = now.UTC()
	}
	tenant = store.NormalizeTenant(tenant)

	n.mu.Lock()
	defer n.mu.Unlock()
	row := n.rows[id]
	if row == nil || !store.SameTenant(row.TenantID, tenant) {
		return Public{}, ErrNotFound
	}
	if !ConfirmOK(confirm, publicNode(row, now)) {
		return Public{}, ErrConfirm
	}
	if row.Status != from {
		return Public{}, wrong
	}
	if to == statusPaired && !row.ExpiresAt.IsZero() && !row.ExpiresAt.After(now) {
		return Public{}, ErrExpired
	}
	row.Status = to
	row.LastSeen = now
	if to == statusPaired {
		row.ApprovedAt = now
	}
	return publicNode(row, now), nil
}

func publicNode(row *device, now time.Time) Public {
	if row == nil {
		return Public{}
	}
	out := Public{
		ID:          row.ID,
		Display:     row.Display,
		Kind:        row.Kind,
		Status:      row.Status,
		Health:      healthOf(row, now),
		RequestedAt: row.RequestedAt,
		LastSeen:    row.LastSeen,
	}
	if row.Status == statusPending && !row.ExpiresAt.IsZero() {
		exp := row.ExpiresAt
		out.ExpiresAt = &exp
	}
	if row.Status == statusPaired && !row.ApprovedAt.IsZero() {
		at := row.ApprovedAt
		out.ApprovedAt = &at
	}
	return out
}

func healthOf(row *device, now time.Time) string {
	if row == nil {
		return healthPending
	}
	switch row.Status {
	case statusPaired:
		if !row.LastSeen.IsZero() && now.Sub(row.LastSeen) >= staleAfter {
			return healthStale
		}
		return healthOK
	case statusPending:
		if !row.ExpiresAt.IsZero() && !row.ExpiresAt.After(now) {
			return healthExpired
		}
		return healthPending
	default:
		return row.Status
	}
}
