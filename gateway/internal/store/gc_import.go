// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// GCCounts is how many gc- rows a removal deleted in one tenant.
// A session is deleted only when it has no remaining notes or messages,
// so a live note written into an imported session keeps that session.
type GCCounts struct {
	Memories  int
	Sessions  int
	VaultDocs int
}

// InsertSessionIfNew inserts sess with its id when that id is absent.
// An existing id is left unchanged (ON CONFLICT DO NOTHING).
// The agent must already exist in the same tenant.
func (s *SQLiteStore) InsertSessionIfNew(sess Session) (bool, error) {
	sess.ID = strings.TrimSpace(sess.ID)
	if sess.ID == "" {
		return false, errors.New("id is required")
	}
	if sess.AgentID == "" {
		return false, errors.New("agent_id is required")
	}
	sess.TenantID = NormalizeTenant(sess.TenantID)
	ag, err := s.GetAgent(sess.AgentID)
	if err != nil || ag == nil || !SameTenant(ag.TenantID, sess.TenantID) {
		return false, errors.New("agent not found")
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now().UTC()
	} else {
		sess.CreatedAt = sess.CreatedAt.UTC()
	}
	res, err := s.db.Exec(`INSERT OR IGNORE INTO sessions(id, agent_id, label, created_at, tenant_id, prompt_mode, model_override, provider_override, archived) VALUES(?,?,?,?,?,?,?,?,?)`,
		sess.ID, sess.AgentID, sess.Label, formatTime(sess.CreatedAt), sess.TenantID, sess.PromptMode, sess.ModelOverride, sess.ProviderOverride, s.archivedArg(sess.Archived))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// InsertMemoryIfNew inserts m with its id and customer_key when that id is absent.
// The customer key is stored as given. It is not derived from the session label.
// An empty customer_key is refused. An existing id is left unchanged.
func (s *SQLiteStore) InsertMemoryIfNew(m Memory) (bool, error) {
	m.ID = strings.TrimSpace(m.ID)
	if m.ID == "" {
		return false, errors.New("id is required")
	}
	if strings.TrimSpace(m.CustomerKey) == "" {
		return false, errors.New("customer_key is required")
	}
	if m.SessionID == "" {
		return false, errors.New("session_id is required")
	}
	if strings.TrimSpace(m.Body) == "" {
		return false, errors.New("body is required")
	}
	m.Kind = NormalizeMemoryKind(m.Kind)
	m.TenantID = NormalizeTenant(m.TenantID)
	sess, err := s.GetSession(m.SessionID)
	if err != nil || sess == nil || !SameTenant(sess.TenantID, m.TenantID) {
		return false, errors.New("session not found")
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	} else {
		m.CreatedAt = m.CreatedAt.UTC()
	}
	res, err := s.db.Exec(`INSERT OR IGNORE INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
		m.ID, m.SessionID, m.Kind, m.Body, formatTime(m.CreatedAt), m.TenantID, m.CustomerKey)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ListGCVaultDocs returns vault rows in tenant whose id starts with gc-.
func (s *SQLiteStore) ListGCVaultDocs(tenant string) ([]VaultDoc, error) {
	tenant = NormalizeTenant(tenant)
	rows, err := s.db.Query(`SELECT `+vaultDocSelect+` FROM vault_docs WHERE tenant_id=? AND substr(id,1,3)=? ORDER BY path`, tenant, gcPrefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VaultDoc
	for rows.Next() {
		d, err := scanVaultDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	if out == nil {
		out = []VaultDoc{}
	}
	return out, rows.Err()
}

// DeleteGCPrefix deletes gc- memories, empty gc- sessions, and gc- vault rows
// in one tenant. A session that still has a note or a message is kept.
// Rows in other tenants are not deleted.
func (s *SQLiteStore) DeleteGCPrefix(tenant string) (GCCounts, error) {
	tenant = NormalizeTenant(tenant)
	tx, err := s.db.Begin()
	if err != nil {
		return GCCounts{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var c GCCounts
	if err := tx.QueryRow(`SELECT COUNT(*) FROM memories WHERE tenant_id=? AND substr(id,1,3)=?`, tenant, gcPrefix).Scan(&c.Memories); err != nil {
		return GCCounts{}, err
	}
	if _, err := tx.Exec(`DELETE FROM memories WHERE tenant_id=? AND substr(id,1,3)=?`, tenant, gcPrefix); err != nil {
		return GCCounts{}, err
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sessions WHERE tenant_id=? AND substr(id,1,3)=?
		AND NOT EXISTS (SELECT 1 FROM memories m WHERE m.session_id = sessions.id)
		AND NOT EXISTS (SELECT 1 FROM messages g WHERE g.session_id = sessions.id)`, tenant, gcPrefix).Scan(&c.Sessions); err != nil {
		return GCCounts{}, err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE tenant_id=? AND substr(id,1,3)=?
		AND NOT EXISTS (SELECT 1 FROM memories m WHERE m.session_id = sessions.id)
		AND NOT EXISTS (SELECT 1 FROM messages g WHERE g.session_id = sessions.id)`, tenant, gcPrefix); err != nil {
		return GCCounts{}, err
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM vault_docs WHERE tenant_id=? AND substr(id,1,3)=?`, tenant, gcPrefix).Scan(&c.VaultDocs); err != nil {
		return GCCounts{}, err
	}
	if _, err := tx.Exec(`DELETE FROM vault_links WHERE from_id IN (SELECT id FROM vault_docs WHERE tenant_id=? AND substr(id,1,3)=?)`, tenant, gcPrefix); err != nil {
		return GCCounts{}, err
	}
	if _, err := tx.Exec(`UPDATE vault_links SET to_id='' WHERE to_id IN (SELECT id FROM vault_docs WHERE tenant_id=? AND substr(id,1,3)=?)`, tenant, gcPrefix); err != nil {
		return GCCounts{}, err
	}
	if _, err := tx.Exec(`DELETE FROM vault_docs WHERE tenant_id=? AND substr(id,1,3)=?`, tenant, gcPrefix); err != nil {
		return GCCounts{}, err
	}
	if err := tx.Commit(); err != nil {
		return GCCounts{}, err
	}
	return c, nil
}

const gcPrefix = "gc-"

// InsertSessionIfNew inserts sess with its id when that id is absent.
func (s *Store) InsertSessionIfNew(sess Session) (bool, error) {
	sess.ID = strings.TrimSpace(sess.ID)
	if sess.ID == "" {
		return false, errors.New("id is required")
	}
	if sess.AgentID == "" {
		return false, errors.New("agent_id is required")
	}
	sess.TenantID = NormalizeTenant(sess.TenantID)
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now().UTC()
	} else {
		sess.CreatedAt = sess.CreatedAt.UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ag, ok := s.agents[sess.AgentID]
	if !ok || ag == nil || !SameTenant(ag.TenantID, sess.TenantID) {
		return false, errors.New("agent not found")
	}
	if _, ok := s.sessions[sess.ID]; ok {
		return false, nil
	}
	cp := sess
	s.sessions[cp.ID] = &cp
	return true, nil
}

// InsertMemoryIfNew inserts m with its id and customer_key when that id is absent.
func (s *Store) InsertMemoryIfNew(m Memory) (bool, error) {
	m.ID = strings.TrimSpace(m.ID)
	if m.ID == "" {
		return false, errors.New("id is required")
	}
	if strings.TrimSpace(m.CustomerKey) == "" {
		return false, errors.New("customer_key is required")
	}
	if m.SessionID == "" {
		return false, errors.New("session_id is required")
	}
	if strings.TrimSpace(m.Body) == "" {
		return false, errors.New("body is required")
	}
	m.Kind = NormalizeMemoryKind(m.Kind)
	m.TenantID = NormalizeTenant(m.TenantID)
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	} else {
		m.CreatedAt = m.CreatedAt.UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[m.SessionID]
	if !ok || sess == nil || !SameTenant(sess.TenantID, m.TenantID) {
		return false, errors.New("session not found")
	}
	for _, list := range s.memories {
		for _, cur := range list {
			if cur != nil && cur.ID == m.ID {
				return false, nil
			}
		}
	}
	cp := m
	s.memories[cp.SessionID] = append(s.memories[cp.SessionID], &cp)
	return true, nil
}

// ListGCVaultDocs returns vault rows in tenant whose id starts with gc-.
func (s *Store) ListGCVaultDocs(tenant string) ([]VaultDoc, error) {
	tenant = NormalizeTenant(tenant)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []VaultDoc
	for _, d := range s.vaultDocs {
		if d != nil && SameTenant(d.TenantID, tenant) && strings.HasPrefix(d.ID, gcPrefix) {
			out = append(out, *copyVaultDoc(d))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	if out == nil {
		out = []VaultDoc{}
	}
	return out, nil
}

// DeleteGCPrefix deletes gc- memories, empty gc- sessions, and gc- vault rows.
func (s *Store) DeleteGCPrefix(tenant string) (GCCounts, error) {
	tenant = NormalizeTenant(tenant)
	s.mu.Lock()
	defer s.mu.Unlock()
	var c GCCounts
	for sid, list := range s.memories {
		kept := make([]*Memory, 0, len(list))
		for _, m := range list {
			if m != nil && SameTenant(m.TenantID, tenant) && strings.HasPrefix(m.ID, gcPrefix) {
				c.Memories++
				continue
			}
			kept = append(kept, m)
		}
		s.memories[sid] = kept
	}
	for id, sess := range s.sessions {
		if sess == nil || !SameTenant(sess.TenantID, tenant) || !strings.HasPrefix(id, gcPrefix) {
			continue
		}
		if len(s.memories[id]) > 0 || len(s.messages[id]) > 0 {
			continue
		}
		delete(s.sessions, id)
		delete(s.memories, id)
		delete(s.messages, id)
		c.Sessions++
	}
	for id, d := range s.vaultDocs {
		if d == nil || !SameTenant(d.TenantID, tenant) || !strings.HasPrefix(id, gcPrefix) {
			continue
		}
		delete(s.vaultDocs, id)
		delete(s.vaultLinks, id)
		for from, list := range s.vaultLinks {
			for i := range list {
				if list[i].ToID == id {
					list[i].ToID = ""
				}
			}
			s.vaultLinks[from] = list
		}
		c.VaultDocs++
	}
	return c, nil
}
