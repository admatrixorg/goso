// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ImportIDPrefix is the per-tenant removal prefix for rows this importer
// writes. Removal uses this prefix, not a wider match.
const ImportIDPrefix = "gc-"

// KGIDHit is a primary-key lookup that does not normalize provenance.
type KGIDHit struct {
	Found  bool
	Tenant string
}

const (
	insertImportedKGEntitySQL   = `INSERT INTO kg_entities(id, tenant_id, name, kind, body, valid_from, valid_until, created_at, agent_id, provenance) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT (id) DO NOTHING`
	insertImportedKGRelationSQL = `INSERT INTO kg_relations(id, tenant_id, from_id, to_id, rel, body, valid_from, valid_until, provenance) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT (id) DO NOTHING`
)

// InsertImportedKGEntity writes one row with the caller's id.
// ON CONFLICT (id) DO NOTHING: a second insert does not change the stored
// row. Provenance is stored as given. This method does not add a column
// and does not take a customer key. SPEC 182 adds customer_key; the
// follow-up that imports customer-bound rows has to pass that key in
// here. Until then, callers must only pass rows whose GoClaw user_id is
// empty.
func (s *SQLiteStore) InsertImportedKGEntity(e KGEntity) (bool, error) {
	e = normalizeImportedEntity(e)
	if err := validateImportedEntity(e); err != nil {
		return false, err
	}
	res, err := s.db.Exec(insertImportedKGEntitySQL,
		e.ID, e.TenantID, e.Name, e.Kind, e.Body,
		formatTime(e.ValidFrom), formatNullTime(e.ValidUntil), formatTime(e.CreatedAt),
		e.AgentID, e.Source)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// InsertImportedKGRelation writes one edge with the caller's id.
// ON CONFLICT (id) DO NOTHING. Both endpoints must already exist in the
// same tenant. Provenance is stored as given. There is no customer_key
// argument; see InsertImportedKGEntity.
func (s *SQLiteStore) InsertImportedKGRelation(rel KGRelation) (bool, error) {
	rel = normalizeImportedRelation(rel)
	if err := validateImportedRelation(rel); err != nil {
		return false, err
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM kg_entities WHERE id=? AND tenant_id=?`, rel.FromID, rel.TenantID).Scan(&n); err != nil {
		return false, err
	}
	if n != 1 {
		return false, errors.New("from entity not found")
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM kg_entities WHERE id=? AND tenant_id=?`, rel.ToID, rel.TenantID).Scan(&n); err != nil {
		return false, err
	}
	if n != 1 {
		return false, errors.New("to entity not found")
	}
	res, err := s.db.Exec(insertImportedKGRelationSQL,
		rel.ID, rel.TenantID, rel.FromID, rel.ToID, rel.Rel, rel.Body,
		formatTime(rel.ValidFrom), formatNullTime(rel.ValidUntil), rel.Source)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// LookupImportedKG reports whether id is already stored and which tenant
// owns it. table is kg_entities or kg_relations. The provenance column is
// not read and is not rewritten.
func (s *SQLiteStore) LookupImportedKG(table, id string) (KGIDHit, error) {
	table, err := importedKGTable(table)
	if err != nil {
		return KGIDHit{}, err
	}
	id = strings.TrimSpace(id)
	var tenant string
	err = s.db.QueryRow(`SELECT tenant_id FROM `+table+` WHERE id=?`, id).Scan(&tenant)
	if errors.Is(err, sql.ErrNoRows) {
		return KGIDHit{}, nil
	}
	if err != nil {
		return KGIDHit{}, err
	}
	return KGIDHit{Found: true, Tenant: NormalizeTenant(tenant)}, nil
}

// KGProvenance reads the provenance column without the posted/extracted
// fold GetKGEntity applies.
func (s *SQLiteStore) KGProvenance(table, id string) (string, error) {
	table, err := importedKGTable(table)
	if err != nil {
		return "", err
	}
	var p string
	err = s.db.QueryRow(`SELECT provenance FROM `+table+` WHERE id=?`, strings.TrimSpace(id)).Scan(&p)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return p, err
}

// CountImportedKG counts rows this importer may remove: id prefix gc- in
// one tenant. The match is case-sensitive on SQLite and Postgres.
func (s *SQLiteStore) CountImportedKG(tenant string) (int64, int64, error) {
	tenant = NormalizeTenant(tenant)
	ent, err := s.countImportedKGTable("kg_entities", tenant)
	if err != nil {
		return 0, 0, err
	}
	rel, err := s.countImportedKGTable("kg_relations", tenant)
	if err != nil {
		return 0, 0, err
	}
	return ent, rel, nil
}

// DeleteImportedKG deletes gc- rows in one tenant from kg_relations and
// kg_entities. Other ids, other tenants, and other tables stay.
func (s *SQLiteStore) DeleteImportedKG(tenant string) (int64, int64, error) {
	tenant = NormalizeTenant(tenant)
	rel, err := s.deleteImportedKGTable("kg_relations", tenant)
	if err != nil {
		return 0, 0, err
	}
	ent, err := s.deleteImportedKGTable("kg_entities", tenant)
	if err != nil {
		return 0, rel, err
	}
	return ent, rel, nil
}

func (s *SQLiteStore) countImportedKGTable(table, tenant string) (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+s.importedKGFilter(), tenant).Scan(&n)
	return n, err
}

func (s *SQLiteStore) deleteImportedKGTable(table, tenant string) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM `+table+` WHERE `+s.importedKGFilter(), tenant)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// importedKGFilter is case-sensitive. SQLite LIKE is not, so file DBs use
// GLOB. Postgres LIKE is case-sensitive.
func (s *SQLiteStore) importedKGFilter() string {
	if s.pg {
		return `tenant_id=? AND id LIKE 'gc-%'`
	}
	return `tenant_id=? AND id GLOB 'gc-*'`
}

func importedKGTable(table string) (string, error) {
	switch table {
	case "kg_entities", "kg_relations":
		return table, nil
	default:
		return "", errors.New("unknown kg table")
	}
}

func normalizeImportedEntity(e KGEntity) KGEntity {
	e.ID = strings.TrimSpace(e.ID)
	e.TenantID = NormalizeTenant(e.TenantID)
	e.Name = strings.TrimSpace(e.Name)
	e.Kind = strings.TrimSpace(e.Kind)
	e.Body = strings.TrimSpace(e.Body)
	e.AgentID = strings.TrimSpace(e.AgentID)
	e.Source = strings.TrimSpace(e.Source)
	e.ValidFrom = e.ValidFrom.UTC()
	e.CreatedAt = e.CreatedAt.UTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = e.ValidFrom
	}
	e.ValidUntil = cloneUntil(e.ValidUntil)
	return e
}

func validateImportedEntity(e KGEntity) error {
	if !strings.HasPrefix(e.ID, ImportIDPrefix) || e.Name == "" || e.Kind == "" || e.ValidFrom.IsZero() {
		return errors.New("kg import entity is incomplete")
	}
	return nil
}

func normalizeImportedRelation(rel KGRelation) KGRelation {
	rel.ID = strings.TrimSpace(rel.ID)
	rel.TenantID = NormalizeTenant(rel.TenantID)
	rel.FromID = strings.TrimSpace(rel.FromID)
	rel.ToID = strings.TrimSpace(rel.ToID)
	rel.Rel = strings.TrimSpace(rel.Rel)
	rel.Body = strings.TrimSpace(rel.Body)
	rel.Source = strings.TrimSpace(rel.Source)
	rel.ValidFrom = rel.ValidFrom.UTC()
	rel.ValidUntil = cloneUntil(rel.ValidUntil)
	rel.Exclusive = false
	return rel
}

func validateImportedRelation(rel KGRelation) error {
	if !strings.HasPrefix(rel.ID, ImportIDPrefix) || !strings.HasPrefix(rel.FromID, ImportIDPrefix) || !strings.HasPrefix(rel.ToID, ImportIDPrefix) || rel.Rel == "" || rel.ValidFrom.IsZero() {
		return errors.New("kg import relation is incomplete")
	}
	return nil
}

func cloneUntil(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

// InsertImportedKGEntity stores the row as given. An existing id is left
// unchanged. See SQLiteStore.InsertImportedKGEntity for the customer_key
// limit.
func (s *Store) InsertImportedKGEntity(e KGEntity) (bool, error) {
	e = normalizeImportedEntity(e)
	if err := validateImportedEntity(e); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kgEntities[e.ID] != nil {
		return false, nil
	}
	cp := e
	s.kgEntities[cp.ID] = &cp
	return true, nil
}

// InsertImportedKGRelation stores the edge as given when both endpoints
// are already in the same tenant. An existing id is left unchanged.
func (s *Store) InsertImportedKGRelation(rel KGRelation) (bool, error) {
	rel = normalizeImportedRelation(rel)
	if err := validateImportedRelation(rel); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	from := s.kgEntities[rel.FromID]
	to := s.kgEntities[rel.ToID]
	if from == nil || !SameTenant(from.TenantID, rel.TenantID) {
		return false, errors.New("from entity not found")
	}
	if to == nil || !SameTenant(to.TenantID, rel.TenantID) {
		return false, errors.New("to entity not found")
	}
	if s.kgRelations[rel.ID] != nil {
		return false, nil
	}
	cp := rel
	s.kgRelations[cp.ID] = &cp
	return true, nil
}

// LookupImportedKG reports an in-memory id without rewriting it.
func (s *Store) LookupImportedKG(table, id string) (KGIDHit, error) {
	if _, err := importedKGTable(table); err != nil {
		return KGIDHit{}, err
	}
	id = strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	switch table {
	case "kg_entities":
		if e := s.kgEntities[id]; e != nil {
			return KGIDHit{Found: true, Tenant: NormalizeTenant(e.TenantID)}, nil
		}
	case "kg_relations":
		if r := s.kgRelations[id]; r != nil {
			return KGIDHit{Found: true, Tenant: NormalizeTenant(r.TenantID)}, nil
		}
	}
	return KGIDHit{}, nil
}

// KGProvenance returns the stored provenance string. It does not fold
// unknown values to posted or extracted.
func (s *Store) KGProvenance(table, id string) (string, error) {
	if _, err := importedKGTable(table); err != nil {
		return "", err
	}
	id = strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	switch table {
	case "kg_entities":
		if e := s.kgEntities[id]; e != nil {
			return e.Source, nil
		}
	case "kg_relations":
		if r := s.kgRelations[id]; r != nil {
			return r.Source, nil
		}
	}
	return "", ErrNotFound
}

// CountImportedKG counts gc- ids in one tenant.
func (s *Store) CountImportedKG(tenant string) (int64, int64, error) {
	tenant = NormalizeTenant(tenant)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ent, rel int64
	for _, e := range s.kgEntities {
		if e != nil && SameTenant(e.TenantID, tenant) && strings.HasPrefix(e.ID, ImportIDPrefix) {
			ent++
		}
	}
	for _, r := range s.kgRelations {
		if r != nil && SameTenant(r.TenantID, tenant) && strings.HasPrefix(r.ID, ImportIDPrefix) {
			rel++
		}
	}
	return ent, rel, nil
}

// DeleteImportedKG removes gc- entities and relations in one tenant.
func (s *Store) DeleteImportedKG(tenant string) (int64, int64, error) {
	tenant = NormalizeTenant(tenant)
	s.mu.Lock()
	defer s.mu.Unlock()
	var rel int64
	for id, r := range s.kgRelations {
		if r != nil && SameTenant(r.TenantID, tenant) && strings.HasPrefix(id, ImportIDPrefix) {
			delete(s.kgRelations, id)
			rel++
		}
	}
	var ent int64
	for id, e := range s.kgEntities {
		if e != nil && SameTenant(e.TenantID, tenant) && strings.HasPrefix(id, ImportIDPrefix) {
			delete(s.kgEntities, id)
			ent++
		}
	}
	return ent, rel, nil
}
