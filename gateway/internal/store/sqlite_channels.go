// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

func (s *SQLiteStore) PutChannelConfig(cfg ChannelConfig) error {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return ErrNotFound
	}
	cfg.UpdatedAt = time.Now().UTC()
	allow, _ := json.Marshal(cfg.AllowFrom)
	if cfg.AllowFrom == nil {
		allow = []byte("[]")
	}
	en := 0
	if cfg.Enabled {
		en = 1
	}
	ment := 0
	if cfg.RequireMention {
		ment = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO channel_config(name, enabled, agent_id, dm_policy, group_policy, require_mention, allow_from, updated_at)
		 VALUES(?,?,?,?,?,?,?,?)
		 ON CONFLICT(name) DO UPDATE SET
		   enabled=excluded.enabled,
		   agent_id=excluded.agent_id,
		   dm_policy=excluded.dm_policy,
		   group_policy=excluded.group_policy,
		   require_mention=excluded.require_mention,
		   allow_from=excluded.allow_from,
		   updated_at=excluded.updated_at`,
		name, en, strings.TrimSpace(cfg.AgentID), cfg.DMPolicy, cfg.GroupPolicy, ment, string(allow), cfg.UpdatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

func (s *SQLiteStore) GetChannelConfig(name string) (*ChannelConfig, error) {
	name = strings.TrimSpace(name)
	row := s.db.QueryRow(
		`SELECT name, enabled, agent_id, dm_policy, group_policy, require_mention, allow_from, updated_at FROM channel_config WHERE name=?`,
		name,
	)
	c, err := scanChannelConfig(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return c, nil
}

func (s *SQLiteStore) ListChannelConfigs() []*ChannelConfig {
	rows, err := s.db.Query(`SELECT name, enabled, agent_id, dm_policy, group_policy, require_mention, allow_from, updated_at FROM channel_config ORDER BY name`)
	if err != nil {
		return []*ChannelConfig{}
	}
	defer rows.Close()
	out := make([]*ChannelConfig, 0)
	for rows.Next() {
		c, err := scanChannelConfig(rows)
		if err != nil {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (s *SQLiteStore) CreateChannelPairing(p ChannelPairing) (*ChannelPairing, error) {
	p.Channel = strings.TrimSpace(p.Channel)
	p.SenderID = strings.TrimSpace(p.SenderID)
	if p.Channel == "" || p.SenderID == "" {
		return nil, ErrNotFound
	}
	if p.ID == "" {
		p.ID = newID()
	}
	if strings.TrimSpace(p.Status) == "" {
		p.Status = "pending"
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	approved := ""
	if !p.ApprovedAt.IsZero() {
		approved = p.ApprovedAt.UTC().Format(time.RFC3339)
	}
	exp := ""
	if !p.ExpiresAt.IsZero() {
		exp = p.ExpiresAt.UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`INSERT INTO channel_pairing(id, channel, sender_id, code_hash, status, expires_at, created_at, approved_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		p.ID, p.Channel, p.SenderID, p.CodeHash, p.Status, exp, p.CreatedAt.UTC().Format(time.RFC3339), approved,
	)
	if err != nil {
		return nil, err
	}
	return cloneChannelPairing(&p), nil
}

func (s *SQLiteStore) GetChannelPairing(id string) (*ChannelPairing, error) {
	id = strings.TrimSpace(id)
	row := s.db.QueryRow(
		`SELECT id, channel, sender_id, code_hash, status, expires_at, created_at, approved_at FROM channel_pairing WHERE id=?`,
		id,
	)
	p, err := scanChannelPairing(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

func (s *SQLiteStore) ListChannelPairings() []*ChannelPairing {
	rows, err := s.db.Query(`SELECT id, channel, sender_id, code_hash, status, expires_at, created_at, approved_at FROM channel_pairing ORDER BY created_at`)
	if err != nil {
		return []*ChannelPairing{}
	}
	defer rows.Close()
	out := make([]*ChannelPairing, 0)
	for rows.Next() {
		p, err := scanChannelPairing(rows)
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (s *SQLiteStore) UpdateChannelPairing(p ChannelPairing) (*ChannelPairing, error) {
	p.ID = strings.TrimSpace(p.ID)
	if p.ID == "" {
		return nil, ErrNotFound
	}
	if _, err := s.GetChannelPairing(p.ID); err != nil {
		return nil, err
	}
	approved := ""
	if !p.ApprovedAt.IsZero() {
		approved = p.ApprovedAt.UTC().Format(time.RFC3339)
	}
	exp := ""
	if !p.ExpiresAt.IsZero() {
		exp = p.ExpiresAt.UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`UPDATE channel_pairing SET channel=?, sender_id=?, code_hash=?, status=?, expires_at=?, created_at=?, approved_at=? WHERE id=?`,
		p.Channel, p.SenderID, p.CodeHash, p.Status, exp, p.CreatedAt.UTC().Format(time.RFC3339), approved, p.ID,
	)
	if err != nil {
		return nil, err
	}
	return cloneChannelPairing(&p), nil
}

func (s *SQLiteStore) CountPendingChannelPairings(channel, sender string, now time.Time) int {
	channel = strings.TrimSpace(channel)
	sender = strings.TrimSpace(sender)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	n := 0
	for _, p := range s.ListChannelPairings() {
		if p.Channel != channel || p.SenderID != sender || p.Status != "pending" {
			continue
		}
		if !p.ExpiresAt.IsZero() && !p.ExpiresAt.After(now) {
			continue
		}
		n++
	}
	return n
}

func (s *SQLiteStore) DeleteSecret(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNotFound
	}
	_, err := s.db.Exec(`DELETE FROM secrets WHERE name=?`, name)
	return err
}

func scanChannelConfig(sc scanner) (*ChannelConfig, error) {
	var c ChannelConfig
	var en, ment int
	var allow, updated, agent string
	if err := sc.Scan(&c.Name, &en, &agent, &c.DMPolicy, &c.GroupPolicy, &ment, &allow, &updated); err != nil {
		return nil, err
	}
	c.Enabled = en != 0
	c.RequireMention = ment != 0
	c.AgentID = agent
	if allow != "" {
		_ = json.Unmarshal([]byte(allow), &c.AllowFrom)
	}
	if t, err := time.Parse(time.RFC3339, updated); err == nil {
		c.UpdatedAt = t
	}
	return &c, nil
}

func scanChannelPairing(sc scanner) (*ChannelPairing, error) {
	var p ChannelPairing
	var exp, created, approved string
	if err := sc.Scan(&p.ID, &p.Channel, &p.SenderID, &p.CodeHash, &p.Status, &exp, &created, &approved); err != nil {
		return nil, err
	}
	if t, err := time.Parse(time.RFC3339, exp); err == nil {
		p.ExpiresAt = t
	}
	if t, err := time.Parse(time.RFC3339, created); err == nil {
		p.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, approved); err == nil {
		p.ApprovedAt = t
	}
	return &p, nil
}
