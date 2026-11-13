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

const kgEntityCols = `id, tenant_id, name, kind, body, valid_from, valid_until, created_at, agent_id, provenance, customer_key`
const kgRelationCols = `id, tenant_id, from_id, to_id, rel, body, valid_from, valid_until, provenance, customer_key`

func (s *SQLiteStore) PutKGEntity(e KGEntity) (*KGEntity, error) {
	if err := prepareKGEntity(&e); err != nil {
		return nil, err
	}
	var sess *Session
	if sid := strings.TrimSpace(e.SessionID); sid != "" {
		got, err := s.GetSession(sid)
		if err != nil {
			return nil, errKGSessionNotFound
		}
		sess = got
	}
	key, err := stampKGCustomer(sess, e.SessionID, e.TenantID)
	if err != nil {
		return nil, err
	}
	e.CustomerKey = key
	e.SessionID = ""
	e.ID = newID()
	e.CreatedAt = time.Now().UTC()
	_, err = s.db.Exec(`INSERT INTO kg_entities(id, tenant_id, name, kind, body, valid_from, valid_until, created_at, agent_id, provenance, customer_key) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.TenantID, e.Name, e.Kind, e.Body, formatTime(e.ValidFrom), formatNullTime(e.ValidUntil), formatTime(e.CreatedAt), e.AgentID, e.Source, e.CustomerKey)
	if err != nil {
		return nil, err
	}
	cp := e
	return &cp, nil
}

func (s *SQLiteStore) getKGEntity(id string) (*KGEntity, error) {
	id = strings.TrimSpace(id)
	row := s.db.QueryRow(`SELECT `+kgEntityCols+` FROM kg_entities WHERE id=?`, id)
	e, err := scanKGEntity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return e, err
}

func (s *SQLiteStore) GetKGEntity(id string, sc KGScope) (*KGEntity, error) {
	e, err := s.getKGEntity(id)
	if err != nil || e == nil {
		return nil, err
	}
	if !sc.Allows(e.CustomerKey) {
		return nil, ErrNotFound
	}
	return e, nil
}

func (s *SQLiteStore) PutKGRelation(rel KGRelation) (*KGRelation, error) {
	if err := prepareKGRelation(&rel); err != nil {
		return nil, err
	}
	var sess *Session
	if sid := strings.TrimSpace(rel.SessionID); sid != "" {
		got, err := s.GetSession(sid)
		if err != nil {
			return nil, errKGSessionNotFound
		}
		sess = got
	}
	key, err := stampKGCustomer(sess, rel.SessionID, rel.TenantID)
	if err != nil {
		return nil, err
	}
	rel.CustomerKey = key
	rel.SessionID = ""
	sc := KGScope{}
	if key != "" {
		sc = KGScope{Scoped: true, CustomerKey: key}
	}
	from, err := s.getKGEntity(rel.FromID)
	if err != nil || from == nil || !SameTenant(from.TenantID, rel.TenantID) || !sc.Allows(from.CustomerKey) {
		return nil, errors.New("from entity not found")
	}
	to, err := s.getKGEntity(rel.ToID)
	if err != nil || to == nil || !SameTenant(to.TenantID, rel.TenantID) || !sc.Allows(to.CustomerKey) {
		return nil, errors.New("to entity not found")
	}
	rel.ID = newID()
	_, err = s.db.Exec(`INSERT INTO kg_relations(id, tenant_id, from_id, to_id, rel, body, valid_from, valid_until, provenance, customer_key) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		rel.ID, rel.TenantID, rel.FromID, rel.ToID, rel.Rel, rel.Body, formatTime(rel.ValidFrom), formatNullTime(rel.ValidUntil), rel.Source, rel.CustomerKey)
	if err != nil {
		return nil, err
	}
	if rel.Exclusive {
		s.supersedeExclusive(rel)
	}
	cp := rel
	return &cp, nil
}

func (s *SQLiteStore) ListKGRelations(entityID string, sc KGScope) ([]*KGRelation, error) {
	entityID = strings.TrimSpace(entityID)
	if _, err := s.GetKGEntity(entityID, sc); err != nil {
		return nil, err
	}
	pred, pargs := kgCustomerPredicate("customer_key", sc)
	args := append([]any{entityID, entityID}, pargs...)
	rows, err := s.db.Query(`SELECT `+kgRelationCols+` FROM kg_relations WHERE (from_id=? OR to_id=?)`+pred, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*KGRelation
	for rows.Next() {
		r, err := scanKGRelation(rows)
		if err != nil || r == nil || !relationEndpointsVisible(s, r, sc) {
			continue
		}
		out = append(out, r)
	}
	if out == nil {
		out = []*KGRelation{}
	}
	return out, nil
}

// relationEndpointsVisible drops a neighbour whose endpoint belongs to
// another customer. A missing endpoint stays, matching the unscoped expand.
func relationEndpointsVisible(s *SQLiteStore, r *KGRelation, sc KGScope) bool {
	if r == nil || !sc.Allows(r.CustomerKey) {
		return false
	}
	if from, err := s.getKGEntity(r.FromID); err == nil && from != nil && !sc.Allows(from.CustomerKey) {
		return false
	}
	if to, err := s.getKGEntity(r.ToID); err == nil && to != nil && !sc.Allows(to.CustomerKey) {
		return false
	}
	return true
}

func (s *SQLiteStore) ListKGGraph(q KGGraphQuery) (*KGGraph, error) {
	tenant := NormalizeTenant(q.Tenant)
	sc := KGScope{Scoped: q.CustomerScoped, CustomerKey: q.CustomerKey}
	pred, pargs := kgCustomerPredicate("customer_key", sc)
	args := append([]any{tenant}, pargs...)
	erows, err := s.db.Query(`SELECT `+kgEntityCols+` FROM kg_entities WHERE tenant_id=?`+pred, args...)
	if err != nil {
		return nil, err
	}
	var ents []*KGEntity
	for erows.Next() {
		e, err := scanKGEntity(erows)
		if err != nil || e == nil {
			continue
		}
		ents = append(ents, e)
	}
	_ = erows.Close()
	rargs := append([]any{tenant}, pargs...)
	rrows, err := s.db.Query(`SELECT `+kgRelationCols+` FROM kg_relations WHERE tenant_id=?`+pred, rargs...)
	if err != nil {
		return nil, err
	}
	defer rrows.Close()
	var rels []*KGRelation
	for rrows.Next() {
		r, err := scanKGRelation(rrows)
		if err != nil || r == nil {
			continue
		}
		rels = append(rels, r)
	}
	return BuildKGGraph(ents, rels, q), nil
}

func (s *SQLiteStore) ExpandKG(id string, sc KGScope) (*KGExpand, error) {
	ent, err := s.GetKGEntity(id, sc)
	if err != nil {
		return nil, err
	}
	rels, err := s.ListKGRelations(id, sc)
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
		if from, err := s.getKGEntity(r.FromID); err == nil && from != nil && sc.Allows(from.CustomerKey) {
			view.FromName = from.Name
		}
		if to, err := s.getKGEntity(r.ToID); err == nil && to != nil && sc.Allows(to.CustomerKey) {
			view.ToName = to.Name
		}
		out.Relations = append(out.Relations, view)
	}
	return out, nil
}

func (s *SQLiteStore) SearchProgressive(q, tenant string, sc KGScope) ([]KGSearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []KGSearchHit{}, nil
	}
	tenant = NormalizeTenant(tenant)
	l1, err := s.searchProgressiveL1(q, tenant)
	if err != nil {
		return nil, err
	}
	l2, err := s.searchProgressiveL2(q, tenant, sc)
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
	if s.bodyTSV {
		return s.searchProgressiveL1TSV(q, tenant)
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

func (s *SQLiteStore) searchProgressiveL2(q, tenant string, sc KGScope) ([]KGSearchHit, error) {
	if s.kgFTS {
		if hits, err := s.searchProgressiveL2FTS(q, tenant, sc); err == nil {
			return hits, nil
		}
	}
	if s.bodyTSV {
		return s.searchProgressiveL2TSV(q, tenant, sc)
	}
	pred, pargs := kgCustomerPredicate("customer_key", sc)
	args := append([]any{tenant, q, q}, pargs...)
	args = append(args, progressivePerTier)
	rows, err := s.db.Query(`SELECT `+kgEntityCols+` FROM kg_entities
		WHERE tenant_id=? AND (instr(lower(name), lower(?)) > 0 OR instr(lower(body), lower(?)) > 0)`+pred+` LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KGSearchHit
	for rows.Next() {
		e, err := scanKGEntity(rows)
		if err != nil || e == nil || !kgCurrent(e.ValidFrom, e.ValidUntil) || !sc.Allows(e.CustomerKey) {
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

func (s *SQLiteStore) searchProgressiveL2FTS(q, tenant string, sc KGScope) ([]KGSearchHit, error) {
	phrase := strings.ReplaceAll(q, `"`, `""`)
	pred, pargs := kgCustomerPredicate("e.customer_key", sc)
	args := append([]any{`"` + phrase + `"`, tenant}, pargs...)
	args = append(args, progressivePerTier)
	rows, err := s.db.Query(`SELECT f.id
		FROM kg_fts f
		JOIN kg_entities e ON e.id = f.id
		WHERE kg_fts MATCH ? AND f.source = 'entity' AND e.tenant_id = ?`+pred+`
		ORDER BY rank LIMIT ?`, args...)
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
		e, err := s.GetKGEntity(id, sc)
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
	if err := sc.Scan(&e.ID, &e.TenantID, &e.Name, &e.Kind, &e.Body, &from, &until, &created, &e.AgentID, &e.Source, &e.CustomerKey); err != nil {
		return nil, err
	}
	e.TenantID = NormalizeTenant(e.TenantID)
	e.AgentID = strings.TrimSpace(e.AgentID)
	e.Source = resolveKGSource(e.Source, e.Kind)
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
	if err := sc.Scan(&r.ID, &r.TenantID, &r.FromID, &r.ToID, &r.Rel, &r.Body, &from, &until, &r.Source, &r.CustomerKey); err != nil {
		return nil, err
	}
	r.TenantID = NormalizeTenant(r.TenantID)
	r.Source = resolveKGSource(r.Source, "")
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
