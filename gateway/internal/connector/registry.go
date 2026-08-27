// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package connector

import (
	"context"
	"errors"
	"sort"
	"sync"
)

var (
	ErrNotFound = errors.New("connector not found")
	ErrExists   = errors.New("connector already registered")
)

type entry struct {
	conn    Connector
	enabled bool
}

// Registry holds named connectors. Safe for concurrent use. In-memory, no network.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*entry
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]*entry)}
}

// Register adds a connector. Duplicate names return ErrExists.
func (r *Registry) Register(c Connector) error {
	if c == nil {
		return errors.New("nil connector")
	}
	name := c.Name()
	if name == "" {
		return errors.New("connector name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[name]; ok {
		return ErrExists
	}
	r.entries[name] = &entry{conn: c, enabled: true}
	return nil
}

// Replace overwrites an existing registration (used when CRUD updates config).
func (r *Registry) Replace(c Connector) error {
	if c == nil {
		return errors.New("nil connector")
	}
	name := c.Name()
	if name == "" {
		return errors.New("connector name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	enabled := true
	if prev, ok := r.entries[name]; ok {
		enabled = prev.enabled
	}
	r.entries[name] = &entry{conn: c, enabled: enabled}
	return nil
}

// Lookup returns a gated view of the named connector.
func (r *Registry) Lookup(name string) (Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	if !ok {
		return nil, ErrNotFound
	}
	return &gated{name: name, inner: e.conn, enabled: e.enabled}, nil
}

// Peek returns the inner connector even when disabled (for listing tools).
func (r *Registry) Peek(name string) (Connector, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	if !ok {
		return nil, false, ErrNotFound
	}
	return e.conn, e.enabled, nil
}

// List returns registered connectors (gated). Order is by name.
func (r *Registry) List() []Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.entries))
	for n := range r.entries {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Connector, 0, len(names))
	for _, n := range names {
		e := r.entries[n]
		out = append(out, &gated{name: n, inner: e.conn, enabled: e.enabled})
	}
	return out
}

// SetEnabled toggles a connector. Disabled Invoke returns ErrUnavailable.
func (r *Registry) SetEnabled(name string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[name]
	if !ok {
		return ErrNotFound
	}
	e.enabled = enabled
	return nil
}

// Enabled reports whether the named connector is enabled.
func (r *Registry) Enabled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	if !ok {
		return false
	}
	return e.enabled
}

type gated struct {
	name    string
	inner   Connector
	enabled bool
}

func (g *gated) Name() string { return g.name }

func (g *gated) ListTools(ctx context.Context) ([]Tool, error) {
	if !g.enabled {
		return nil, ErrUnavailable
	}
	return g.inner.ListTools(ctx)
}

func (g *gated) Invoke(ctx context.Context, tool string, args map[string]any) (*InvokeResult, error) {
	if !g.enabled {
		return nil, ErrUnavailable
	}
	return g.inner.Invoke(ctx, tool, args)
}

func (g *gated) Health(ctx context.Context) error {
	if !g.enabled {
		return ErrUnavailable
	}
	return g.inner.Health(ctx)
}
