// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (s *SQLiteStore) initKGFTS() {
	s.kgFTS = false
	if s.pg {
		return
	}
	if _, err := s.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS kg_fts USING fts5(
		id UNINDEXED,
		source UNINDEXED,
		name,
		body
	)`); err != nil {
		return
	}
	triggers := []string{
		`CREATE TRIGGER IF NOT EXISTS kg_entities_ai AFTER INSERT ON kg_entities BEGIN
			INSERT INTO kg_fts(id, source, name, body) VALUES (new.id, 'entity', new.name, new.body);
		END`,
		`CREATE TRIGGER IF NOT EXISTS kg_entities_au AFTER UPDATE ON kg_entities BEGIN
			DELETE FROM kg_fts WHERE id = old.id;
			INSERT INTO kg_fts(id, source, name, body) VALUES (new.id, 'entity', new.name, new.body);
		END`,
		`CREATE TRIGGER IF NOT EXISTS kg_entities_ad AFTER DELETE ON kg_entities BEGIN
			DELETE FROM kg_fts WHERE id = old.id;
		END`,
		`CREATE TRIGGER IF NOT EXISTS kg_relations_ai AFTER INSERT ON kg_relations BEGIN
			INSERT INTO kg_fts(id, source, name, body) VALUES (new.id, 'relation', new.rel, new.body);
		END`,
		`CREATE TRIGGER IF NOT EXISTS kg_relations_au AFTER UPDATE ON kg_relations BEGIN
			DELETE FROM kg_fts WHERE id = old.id;
			INSERT INTO kg_fts(id, source, name, body) VALUES (new.id, 'relation', new.rel, new.body);
		END`,
		`CREATE TRIGGER IF NOT EXISTS kg_relations_ad AFTER DELETE ON kg_relations BEGIN
			DELETE FROM kg_fts WHERE id = old.id;
		END`,
	}
	for _, stmt := range triggers {
		if _, err := s.db.Exec(stmt); err != nil {
			return
		}
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM kg_fts`).Scan(&n); err != nil {
		return
	}
	if n == 0 {
		_, _ = s.db.Exec(`INSERT INTO kg_fts(id, source, name, body) SELECT id, 'entity', name, body FROM kg_entities`)
		_, _ = s.db.Exec(`INSERT INTO kg_fts(id, source, name, body) SELECT id, 'relation', rel, body FROM kg_relations`)
	}
	s.kgFTS = true
}

func (s *SQLiteStore) PutKGEntity(e KGEntity) (*KGEntity, error) {
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" {
		return nil, errors.New("name is required")
	}
	e.Kind = strings.TrimSpace(e.Kind)
	if e.Kind == "" {
		e.Kind = "entity"
	}
	e.Body = strings.TrimSpace(e.Body)
	e.TenantID = NormalizeTenant(e.TenantID)
	e.ValidFrom, e.ValidUntil = stampKGTimes(e.ValidFrom, e.ValidUntil)
	e.ID = newID()
	e.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO kg_entities(id, tenant_id, name, kind, body, valid_from, valid_until, created_at) VALUES(?,?,?,?,?,?,?,?)`,
		e.ID, e.TenantID, e.Name, e.Kind, e.Body, formatTime(e.ValidFrom), formatNullTime(e.ValidUntil), formatTime(e.CreatedAt))
	if err != nil {
		return nil, err
	}
	cp := e
	return &cp, nil
}

func (s *SQLiteStore) GetKGEntity(id string) (*KGEntity, error) {
	id = strings.TrimSpace(id)
	row := s.db.QueryRow(`SELECT id, tenant_id, name, kind, body, valid_from, valid_until, created_at FROM kg_entities WHERE id=?`, id)
	e, err := scanKGEntity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return e, err
}

func (s *SQLiteStore) PutKGRelation(rel KGRelation) (*KGRelation, error) {
	rel.FromID = strings.TrimSpace(rel.FromID)
	rel.ToID = strings.TrimSpace(rel.ToID)
	rel.Rel = strings.TrimSpace(rel.Rel)
	if rel.FromID == "" || rel.ToID == "" {
		return nil, errors.New("from_id and to_id are required")
	}
	if rel.Rel == "" {
		return nil, errors.New("rel is required")
	}
	rel.Body = strings.TrimSpace(rel.Body)
	rel.TenantID = NormalizeTenant(rel.TenantID)
	rel.ValidFrom, rel.ValidUntil = stampKGTimes(rel.ValidFrom, rel.ValidUntil)
	from, err := s.GetKGEntity(rel.FromID)
	if err != nil {
		return nil, errors.New("from entity not found")
	}
	to, err := s.GetKGEntity(rel.ToID)
	if err != nil {
		return nil, errors.New("to entity not found")
	}
	if !SameTenant(from.TenantID, rel.TenantID) || !SameTenant(to.TenantID, rel.TenantID) {
		return nil, errors.New("from entity not found")
	}
	rel.ID = newID()
	_, err = s.db.Exec(`INSERT INTO kg_relations(id, tenant_id, from_id, to_id, rel, body, valid_from, valid_until) VALUES(?,?,?,?,?,?,?,?)`,
		rel.ID, rel.TenantID, rel.FromID, rel.ToID, rel.Rel, rel.Body, formatTime(rel.ValidFrom), formatNullTime(rel.ValidUntil))
	if err != nil {
		return nil, err
	}
	cp := rel
	return &cp, nil
}

func (s *SQLiteStore) ListKGRelations(entityID string) ([]*KGRelation, error) {
	entityID = strings.TrimSpace(entityID)
	if _, err := s.GetKGEntity(entityID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, from_id, to_id, rel, body, valid_from, valid_until FROM kg_relations WHERE from_id=? OR to_id=?`, entityID, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*KGRelation
	for rows.Next() {
		r, err := scanKGRelation(rows)
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	if out == nil {
		out = []*KGRelation{}
	}
	return out, nil
}

func (s *SQLiteStore) ExpandKG(id string) (*KGExpand, error) {
	ent, err := s.GetKGEntity(id)
	if err != nil {
		return nil, err
	}
	rels, err := s.ListKGRelations(id)
	if err != nil {
		return nil, err
	}
	out := &KGExpand{Entity: ent, Relations: []KGRelationExpand{}}
	for _, r := range rels {
		if r == nil || !kgCurrent(r.ValidFrom, r.ValidUntil) {
			continue
		}
		if !SameTenant(r.TenantID, ent.TenantID) {
			continue
		}
		view := KGRelationExpand{KGRelation: *r}
		if from, err := s.GetKGEntity(r.FromID); err == nil && from != nil {
			view.FromName = from.Name
		}
		if to, err := s.GetKGEntity(r.ToID); err == nil && to != nil {
			view.ToName = to.Name
		}
		out.Relations = append(out.Relations, view)
	}
	return out, nil
}

func (s *SQLiteStore) SearchProgressive(q, tenant string) ([]KGSearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []KGSearchHit{}, nil
	}
	tenant = NormalizeTenant(tenant)
	l1, err := s.searchProgressiveL1(q, tenant)
	if err != nil {
		return nil, err
	}
	l2, err := s.searchProgressiveL2(q, tenant)
	if err != nil {
		return nil, err
	}
	out := append(l1, l2...)
	if out == nil {
		out = []KGSearchHit{}
	}
	return out, nil
}

func (s *SQLiteStore) searchProgressiveL1(q, tenant string) ([]KGSearchHit, error) {
	if s.fts {
		if hits, err := s.searchProgressiveL1FTS(q, tenant); err == nil {
			return hits, nil
		}
	}
	rows, err := s.db.Query(`SELECT id, session_id, kind, body, tenant_id FROM (
			SELECT id, session_id, kind, body, tenant_id FROM memories WHERE tenant_id=? AND instr(lower(body), lower(?)) > 0
			UNION ALL
			SELECT m.id, m.session_id, 'message', m.content, s.tenant_id FROM messages m
			JOIN sessions s ON s.id = m.session_id
			WHERE s.tenant_id=? AND instr(lower(m.content), lower(?)) > 0
		) AS hits LIMIT ?`, tenant, q, tenant, q, progressivePerTier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KGSearchHit
	for rows.Next() {
		var h KGSearchHit
		var body string
		if err := rows.Scan(&h.ID, &h.SessionID, &h.Kind, &body, &h.TenantID); err != nil {
			continue
		}
		h.TenantID = NormalizeTenant(h.TenantID)
		h.Snippet = SnippetAround(body, q, 80)
		h.Tier = TierL1
		out = append(out, h)
	}
	if out == nil {
		out = []KGSearchHit{}
	}
	return out, nil
}

func (s *SQLiteStore) searchProgressiveL1FTS(q, tenant string) ([]KGSearchHit, error) {
	phrase := strings.ReplaceAll(q, `"`, `""`)
	rows, err := s.db.Query(`SELECT f.id, f.session_id, f.kind, snippet(memory_fts, 3, '', '', '…', 16),
		COALESCE(mem.tenant_id, sess.tenant_id, 'default')
		FROM memory_fts f
		LEFT JOIN memories mem ON mem.id = f.id
		LEFT JOIN sessions sess ON sess.id = f.session_id
		WHERE memory_fts MATCH ?
		AND COALESCE(mem.tenant_id, sess.tenant_id, 'default') = ?
		ORDER BY rank LIMIT ?`, `"`+phrase+`"`, tenant, progressivePerTier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KGSearchHit
	for rows.Next() {
		var h KGSearchHit
		if err := rows.Scan(&h.ID, &h.SessionID, &h.Kind, &h.Snippet, &h.TenantID); err != nil {
			continue
		}
		h.TenantID = NormalizeTenant(h.TenantID)
		h.Tier = TierL1
		out = append(out, h)
	}
	if out == nil {
		out = []KGSearchHit{}
	}
	return out, nil
}

func (s *SQLiteStore) searchProgressiveL2(q, tenant string) ([]KGSearchHit, error) {
	if s.kgFTS {
		if hits, err := s.searchProgressiveL2FTS(q, tenant); err == nil {
			return hits, nil
		}
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, name, kind, body, valid_from, valid_until, created_at FROM kg_entities
		WHERE tenant_id=? AND (instr(lower(name), lower(?)) > 0 OR instr(lower(body), lower(?)) > 0) LIMIT ?`, tenant, q, q, progressivePerTier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KGSearchHit
	for rows.Next() {
		e, err := scanKGEntity(rows)
		if err != nil || e == nil || !kgCurrent(e.ValidFrom, e.ValidUntil) {
			continue
		}
		out = append(out, KGSearchHit{
			ID: e.ID, TenantID: e.TenantID, Kind: e.Kind, Name: e.Name,
			Snippet: SnippetAround(e.Name+" "+e.Body, q, 80), Tier: TierL2,
		})
	}
	if out == nil {
		out = []KGSearchHit{}
	}
	return out, nil
}

func (s *SQLiteStore) searchProgressiveL2FTS(q, tenant string) ([]KGSearchHit, error) {
	phrase := strings.ReplaceAll(q, `"`, `""`)
	rows, err := s.db.Query(`SELECT f.id
		FROM kg_fts f
		JOIN kg_entities e ON e.id = f.id
		WHERE kg_fts MATCH ? AND f.source = 'entity' AND e.tenant_id = ?
		ORDER BY rank LIMIT ?`, `"`+phrase+`"`, tenant, progressivePerTier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KGSearchHit
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		e, err := s.GetKGEntity(id)
		if err != nil || e == nil || !kgCurrent(e.ValidFrom, e.ValidUntil) {
			continue
		}
		out = append(out, KGSearchHit{
			ID: e.ID, TenantID: e.TenantID, Kind: e.Kind, Name: e.Name,
			Snippet: SnippetAround(e.Name+" "+e.Body, q, 80), Tier: TierL2,
		})
	}
	if out == nil {
		out = []KGSearchHit{}
	}
	return out, nil
}

func formatNullTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return formatTime(*t)
}

func scanKGEntity(sc scanner) (*KGEntity, error) {
	var e KGEntity
	var from, until, created sql.NullString
	if err := sc.Scan(&e.ID, &e.TenantID, &e.Name, &e.Kind, &e.Body, &from, &until, &created); err != nil {
		return nil, err
	}
	e.TenantID = NormalizeTenant(e.TenantID)
	if from.Valid {
		e.ValidFrom = parseTime(from.String)
	}
	if until.Valid && strings.TrimSpace(until.String) != "" {
		u := parseTime(until.String)
		if !u.IsZero() {
			e.ValidUntil = &u
		}
	}
	if created.Valid {
		e.CreatedAt = parseTime(created.String)
	}
	return &e, nil
}

func scanKGRelation(sc scanner) (*KGRelation, error) {
	var r KGRelation
	var from, until sql.NullString
	if err := sc.Scan(&r.ID, &r.TenantID, &r.FromID, &r.ToID, &r.Rel, &r.Body, &from, &until); err != nil {
		return nil, err
	}
	r.TenantID = NormalizeTenant(r.TenantID)
	if from.Valid {
		r.ValidFrom = parseTime(from.String)
	}
	if until.Valid && strings.TrimSpace(until.String) != "" {
		u := parseTime(until.String)
		if !u.IsZero() {
			r.ValidUntil = &u
		}
	}
	return &r, nil
}
