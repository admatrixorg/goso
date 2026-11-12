// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"strings"
)

// gcImportLabel is the session label written for an imported archive.
// It is not a channel label, so the customer-key backfill cannot stamp it.
const gcImportLabel = "gc-import"

// InsertGCSession inserts one archived session with the caller's id.
// ON CONFLICT (id) DO NOTHING: an existing session is left as it is,
// including a live session that already owns that id.
// The label is always gc-import. Kind and archive are not taken from the caller.
func (s *SQLiteStore) InsertGCSession(sess Session) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("store is closed")
	}
	if !strings.HasPrefix(sess.ID, "gc-") {
		return false, errors.New("session id prefix")
	}
	if strings.TrimSpace(sess.AgentID) == "" {
		return false, errors.New("agent_id is required")
	}
	if !TenantOK(sess.TenantID) {
		return false, errors.New("tenant_id is invalid")
	}
	if sess.CreatedAt.IsZero() {
		return false, errors.New("created_at is required")
	}
	res, err := s.db.Exec(`INSERT INTO sessions(id, agent_id, label, created_at, tenant_id, prompt_mode, model_override, provider_override, archived) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`,
		sess.ID, sess.AgentID, gcImportLabel, formatTime(sess.CreatedAt.UTC()), sess.TenantID, "", "", "", s.archivedArg(true))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// InsertGCMemory inserts one episodic note with the caller's id, created_at,
// and customer_key. ON CONFLICT (id) DO NOTHING. PutMemory and SaveSummary
// are not used: PutMemory mints a new id and overwrites customer_key from the
// session label, and SaveSummary overwrites the latest episodic in the session.
func (s *SQLiteStore) InsertGCMemory(m Memory) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("store is closed")
	}
	if !strings.HasPrefix(m.ID, "gc-") {
		return false, errors.New("memory id prefix")
	}
	if !strings.HasPrefix(m.SessionID, "gc-") {
		return false, errors.New("session id prefix")
	}
	if strings.TrimSpace(m.Body) == "" {
		return false, errors.New("body is required")
	}
	if !TenantOK(m.TenantID) {
		return false, errors.New("tenant_id is invalid")
	}
	if m.CreatedAt.IsZero() {
		return false, errors.New("created_at is required")
	}
	res, err := s.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`,
		m.ID, m.SessionID, KindEpisodic, m.Body, formatTime(m.CreatedAt.UTC()), m.TenantID, m.CustomerKey)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CountGCPrefix counts memories and sessions this tenant would lose to
// DeleteGCPrefix. A gc- session that still has a non-gc memory is not counted.
func (s *SQLiteStore) CountGCPrefix(tenant string) (memories, sessions int, err error) {
	if err := gcTenant(tenant); err != nil {
		return 0, 0, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM memories WHERE tenant_id=? AND id LIKE 'gc-%'`, tenant).Scan(&memories); err != nil {
		return 0, 0, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE tenant_id=? AND id LIKE 'gc-%' AND NOT EXISTS (SELECT 1 FROM memories m WHERE m.session_id = sessions.id AND m.id NOT LIKE 'gc-%')`, tenant).Scan(&sessions); err != nil {
		return 0, 0, err
	}
	return memories, sessions, nil
}

// DeleteGCPrefix deletes this tenant's imported memories and sessions whose
// ids start with gc-. A gc- session that still holds a non-gc memory is kept,
// and that memory is kept. Other tenants are not touched.
func (s *SQLiteStore) DeleteGCPrefix(tenant string) (memories, sessions int, err error) {
	if err := gcTenant(tenant); err != nil {
		return 0, 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM memories WHERE tenant_id=? AND id LIKE 'gc-%'`, tenant)
	if err != nil {
		return 0, 0, err
	}
	memN, err := res.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	res, err = tx.Exec(`DELETE FROM sessions WHERE tenant_id=? AND id LIKE 'gc-%' AND NOT EXISTS (SELECT 1 FROM memories m WHERE m.session_id = sessions.id AND m.id NOT LIKE 'gc-%')`, tenant)
	if err != nil {
		return 0, 0, err
	}
	sessN, err := res.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return int(memN), int(sessN), nil
}

func gcTenant(tenant string) error {
	if !TenantOK(tenant) {
		return errors.New("tenant_id is invalid")
	}
	return nil
}
