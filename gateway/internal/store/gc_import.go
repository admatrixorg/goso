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

// ListImportedMemdocVault returns this importer's vault rows (gc-vdoc:).
// Other gc- prefixes are left in place.
func (s *SQLiteStore) ListImportedMemdocVault(tenant string) ([]VaultDoc, error) {
	tenant = NormalizeTenant(tenant)
	rows, err := s.db.Query(`SELECT `+vaultDocSelect+` FROM vault_docs WHERE `+s.memdocWhere(memdocVaultGlob)+` ORDER BY path`, tenant)
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

// DeleteImportedMemdocs deletes this importer's rows in one tenant:
// memories gc-doc:, sessions gc-docsess: that no longer hold a note or a
// message, and vault docs gc-vdoc:. A gc-ep: row is not a match.
// SQLite uses GLOB because LIKE is case-insensitive. Postgres LIKE is
// case-sensitive, so that dialect uses LIKE.
func (s *SQLiteStore) DeleteImportedMemdocs(tenant string) (GCCounts, error) {
	tenant = NormalizeTenant(tenant)
	tx, err := s.db.Begin()
	if err != nil {
		return GCCounts{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var c GCCounts
	memWhere := s.memdocWhere(memdocMemoryGlob)
	sessWhere := s.memdocWhere(memdocSessionGlob)
	vaultWhere := s.memdocWhere(memdocVaultGlob)
	if err := tx.QueryRow(`SELECT COUNT(*) FROM memories WHERE `+memWhere, tenant).Scan(&c.Memories); err != nil {
		return GCCounts{}, err
	}
	if _, err := tx.Exec(`DELETE FROM memories WHERE `+memWhere, tenant); err != nil {
		return GCCounts{}, err
	}
	sessKeep := sessWhere + ` AND NOT EXISTS (SELECT 1 FROM memories m WHERE m.session_id = sessions.id)
		AND NOT EXISTS (SELECT 1 FROM messages g WHERE g.session_id = sessions.id)`
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sessions WHERE `+sessKeep, tenant).Scan(&c.Sessions); err != nil {
		return GCCounts{}, err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE `+sessKeep, tenant); err != nil {
		return GCCounts{}, err
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM vault_docs WHERE `+vaultWhere, tenant).Scan(&c.VaultDocs); err != nil {
		return GCCounts{}, err
	}
	if _, err := tx.Exec(`DELETE FROM vault_links WHERE from_id IN (SELECT id FROM vault_docs WHERE `+vaultWhere+`)`, tenant); err != nil {
		return GCCounts{}, err
	}
	if _, err := tx.Exec(`UPDATE vault_links SET to_id='' WHERE to_id IN (SELECT id FROM vault_docs WHERE `+vaultWhere+`)`, tenant); err != nil {
		return GCCounts{}, err
	}
	if _, err := tx.Exec(`DELETE FROM vault_docs WHERE `+vaultWhere, tenant); err != nil {
		return GCCounts{}, err
	}
	if err := tx.Commit(); err != nil {
		return GCCounts{}, err
	}
	return c, nil
}

const (
	memdocMemoryGlob  = "gc-doc:*"
	memdocSessionGlob = "gc-docsess:*"
	memdocVaultGlob   = "gc-vdoc:*"
)

// memdocWhere matches one importer prefix. glob is a constant of the form
// "gc-doc:*". SQLite GLOB is case-sensitive. Postgres rewrites to LIKE.
func (s *SQLiteStore) memdocWhere(glob string) string {
	if s != nil && s.pg {
		return `tenant_id=? AND id LIKE '` + strings.ReplaceAll(glob, "*", "%") + `'`
	}
	return `tenant_id=? AND id GLOB '` + glob + `'`
}

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

// ListImportedMemdocVault returns vault rows whose id starts with gc-vdoc:.
func (s *Store) ListImportedMemdocVault(tenant string) ([]VaultDoc, error) {
	tenant = NormalizeTenant(tenant)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []VaultDoc
	for _, d := range s.vaultDocs {
		if d != nil && SameTenant(d.TenantID, tenant) && strings.HasPrefix(d.ID, "gc-vdoc:") {
			out = append(out, *copyVaultDoc(d))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	if out == nil {
		out = []VaultDoc{}
	}
	return out, nil
}

// DeleteImportedMemdocs deletes gc-doc memories, empty gc-docsess sessions, and gc-vdoc vault rows.
func (s *Store) DeleteImportedMemdocs(tenant string) (GCCounts, error) {
	tenant = NormalizeTenant(tenant)
	s.mu.Lock()
	defer s.mu.Unlock()
	var c GCCounts
	for sid, list := range s.memories {
		kept := make([]*Memory, 0, len(list))
		for _, m := range list {
			if m != nil && SameTenant(m.TenantID, tenant) && strings.HasPrefix(m.ID, "gc-doc:") {
				c.Memories++
				continue
			}
			kept = append(kept, m)
		}
		s.memories[sid] = kept
	}
	for id, sess := range s.sessions {
		if sess == nil || !SameTenant(sess.TenantID, tenant) || !strings.HasPrefix(id, "gc-docsess:") {
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
		if d == nil || !SameTenant(d.TenantID, tenant) || !strings.HasPrefix(id, "gc-vdoc:") {
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
