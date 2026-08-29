// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"strings"
	"time"
)

// ChannelConfig is non-secret per-catalog-name channel settings (SPEC 084).
type ChannelConfig struct {
	Name            string    `json:"name"`
	Enabled         bool      `json:"enabled"`
	AgentID         string    `json:"agent_id,omitempty"`
	DMPolicy        string    `json:"dm_policy"`
	GroupPolicy     string    `json:"group_policy"`
	RequireMention  bool      `json:"require_mention"`
	AllowFrom       []string  `json:"allow_from,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ChannelPairing is a hashed DM pairing request (not 077 view-token pairing).
type ChannelPairing struct {
	ID         string    `json:"id"`
	Channel    string    `json:"channel"`
	SenderID   string    `json:"sender_id"`
	CodeHash   string    `json:"-"`
	Status     string    `json:"status"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	ApprovedAt time.Time `json:"approved_at,omitempty"`
}

func cloneChannelConfig(c *ChannelConfig) *ChannelConfig {
	if c == nil {
		return nil
	}
	cp := *c
	if c.AllowFrom != nil {
		cp.AllowFrom = append([]string(nil), c.AllowFrom...)
	}
	return &cp
}

func cloneChannelPairing(p *ChannelPairing) *ChannelPairing {
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

// PutChannelConfig upserts non-secret config by catalog name.
func (s *Store) PutChannelConfig(cfg ChannelConfig) error {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return ErrNotFound
	}
	cfg.Name = name
	cfg.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channelConfig == nil {
		s.channelConfig = map[string]*ChannelConfig{}
	}
	s.channelConfig[name] = cloneChannelConfig(&cfg)
	return nil
}

// GetChannelConfig returns one row or ErrNotFound.
func (s *Store) GetChannelConfig(name string) (*ChannelConfig, error) {
	name = strings.TrimSpace(name)
	s.mu.RLock()
	defer s.mu.RUnlock()
	row, ok := s.channelConfig[name]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneChannelConfig(row), nil
}

// ListChannelConfigs returns all configs.
func (s *Store) ListChannelConfigs() []*ChannelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ChannelConfig, 0, len(s.channelConfig))
	for _, c := range s.channelConfig {
		out = append(out, cloneChannelConfig(c))
	}
	return out
}

// CreateChannelPairing inserts a pending (or given) pairing row.
func (s *Store) CreateChannelPairing(p ChannelPairing) (*ChannelPairing, error) {
	p.Channel = strings.TrimSpace(p.Channel)
	p.SenderID = strings.TrimSpace(p.SenderID)
	if p.Channel == "" || p.SenderID == "" {
		return nil, ErrNotFound
	}
	if strings.TrimSpace(p.Status) == "" {
		p.Status = "pending"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channelPairing == nil {
		s.channelPairing = map[string]*ChannelPairing{}
	}
	if p.ID == "" {
		p.ID = s.nextID()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	cp := cloneChannelPairing(&p)
	s.channelPairing[p.ID] = cp
	return cloneChannelPairing(cp), nil
}

// GetChannelPairing returns one pairing by id.
func (s *Store) GetChannelPairing(id string) (*ChannelPairing, error) {
	id = strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	row, ok := s.channelPairing[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneChannelPairing(row), nil
}

// ListChannelPairings returns all pairing rows.
func (s *Store) ListChannelPairings() []*ChannelPairing {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ChannelPairing, 0, len(s.channelPairing))
	for _, p := range s.channelPairing {
		out = append(out, cloneChannelPairing(p))
	}
	return out
}

// UpdateChannelPairing replaces a pairing row by id.
func (s *Store) UpdateChannelPairing(p ChannelPairing) (*ChannelPairing, error) {
	p.ID = strings.TrimSpace(p.ID)
	if p.ID == "" {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channelPairing[p.ID]; !ok {
		return nil, ErrNotFound
	}
	cp := cloneChannelPairing(&p)
	s.channelPairing[p.ID] = cp
	return cloneChannelPairing(cp), nil
}

// CountPendingChannelPairings counts pending, unexpired rows for sender+channel.
func (s *Store) CountPendingChannelPairings(channel, sender string, now time.Time) int {
	channel = strings.TrimSpace(channel)
	sender = strings.TrimSpace(sender)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, p := range s.channelPairing {
		if p.Channel != channel || p.SenderID != sender {
			continue
		}
		if p.Status != "pending" {
			continue
		}
		if !p.ExpiresAt.IsZero() && !p.ExpiresAt.After(now) {
			continue
		}
		n++
	}
	return n
}

// DeleteSecret removes a ciphertext blob. Missing name is a no-op.
func (s *Store) DeleteSecret(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.secrets, name)
	return nil
}
