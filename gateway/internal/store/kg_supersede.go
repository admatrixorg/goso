// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"strings"
	"time"
)

// Temporal invalidation for the L2 graph (Zep-style: facts are never edited in
// place; a superseded entity or relation gets valid_until and stops being
// "current" for kgCurrent readers). Scoped by tenant at the store layer.

// KGIsCurrent reports whether a validity window is open right now.
func KGIsCurrent(from time.Time, until *time.Time) bool { return kgCurrent(from, until) }

func supersedeAt(until time.Time) time.Time {
	if until.IsZero() {
		return time.Now().UTC()
	}
	return until.UTC()
}

// SupersedeKGEntity closes an entity's validity window.
func (s *Store) SupersedeKGEntity(tenant, id string, until time.Time) error {
	tenant = NormalizeTenant(tenant)
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("id is required")
	}
	u := supersedeAt(until)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.kgEntities[id]
	if !ok || e == nil || !SameTenant(e.TenantID, tenant) {
		return ErrNotFound
	}
	e.ValidUntil = &u
	return nil
}

// SupersedeKGRelation closes a relation's validity window.
func (s *Store) SupersedeKGRelation(tenant, id string, until time.Time) error {
	tenant = NormalizeTenant(tenant)
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("id is required")
	}
	u := supersedeAt(until)
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.kgRelations[id]
	if !ok || r == nil || !SameTenant(r.TenantID, tenant) {
		return ErrNotFound
	}
	r.ValidUntil = &u
	return nil
}

// supersedeExclusiveLocked closes every current relation with the same
// (tenant, from_id, rel) but a different to_id. Caller holds s.mu.
func (s *Store) supersedeExclusiveLocked(rel KGRelation) {
	now := time.Now().UTC()
	for _, r := range s.kgRelations {
		if r == nil || !SameTenant(r.TenantID, rel.TenantID) || r.FromID != rel.FromID || r.Rel != rel.Rel || r.ToID == rel.ToID {
			continue
		}
		if kgCurrent(r.ValidFrom, r.ValidUntil) {
			u := now
			r.ValidUntil = &u
		}
	}
}

// SupersedeKGEntity (SQL stores).
func (s *SQLiteStore) SupersedeKGEntity(tenant, id string, until time.Time) error {
	tenant = NormalizeTenant(tenant)
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("id is required")
	}
	res, err := s.db.Exec(`UPDATE kg_entities SET valid_until=? WHERE id=? AND tenant_id=?`, formatTime(supersedeAt(until)), id, tenant)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SupersedeKGRelation (SQL stores).
func (s *SQLiteStore) SupersedeKGRelation(tenant, id string, until time.Time) error {
	tenant = NormalizeTenant(tenant)
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("id is required")
	}
	res, err := s.db.Exec(`UPDATE kg_relations SET valid_until=? WHERE id=? AND tenant_id=?`, formatTime(supersedeAt(until)), id, tenant)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// supersedeExclusive (SQL stores): see supersedeExclusiveLocked.
func (s *SQLiteStore) supersedeExclusive(rel KGRelation) {
	now := formatTime(time.Now().UTC())
	_, _ = s.db.Exec(`UPDATE kg_relations SET valid_until=?
		WHERE tenant_id=? AND from_id=? AND rel=? AND to_id<>?
		AND (valid_until IS NULL OR valid_until='' OR valid_until > ?)`,
		now, rel.TenantID, rel.FromID, rel.Rel, rel.ToID, now)
}
