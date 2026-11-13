// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"strings"
	"time"
)

func kgCurrent(from time.Time, until *time.Time) bool {
	now := time.Now().UTC()
	if !from.IsZero() && from.After(now) {
		return false
	}
	if until != nil && !until.IsZero() && !until.After(now) {
		return false
	}
	return true
}

func stampKGTimes(from time.Time, until *time.Time) (time.Time, *time.Time) {
	if from.IsZero() {
		from = time.Now().UTC()
	} else {
		from = from.UTC()
	}
	if until != nil {
		u := until.UTC()
		if u.IsZero() {
			until = nil
		} else {
			until = &u
		}
	}
	return from, until
}

func prepareKGEntity(e *KGEntity) error {
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" {
		return errors.New("name is required")
	}
	e.Kind = strings.TrimSpace(e.Kind)
	if e.Kind == "" {
		e.Kind = "entity"
	}
	e.Body = strings.TrimSpace(e.Body)
	e.TenantID = NormalizeTenant(e.TenantID)
	e.AgentID = strings.TrimSpace(e.AgentID)
	e.Source = resolveKGSource(e.Source, e.Kind)
	e.ValidFrom, e.ValidUntil = stampKGTimes(e.ValidFrom, e.ValidUntil)
	return nil
}

func prepareKGRelation(rel *KGRelation) error {
	rel.FromID = strings.TrimSpace(rel.FromID)
	rel.ToID = strings.TrimSpace(rel.ToID)
	rel.Rel = strings.TrimSpace(rel.Rel)
	if rel.FromID == "" || rel.ToID == "" {
		return errors.New("from_id and to_id are required")
	}
	if rel.Rel == "" {
		return errors.New("rel is required")
	}
	rel.Body = strings.TrimSpace(rel.Body)
	rel.TenantID = NormalizeTenant(rel.TenantID)
	rel.Source = resolveKGSource(rel.Source, "")
	rel.ValidFrom, rel.ValidUntil = stampKGTimes(rel.ValidFrom, rel.ValidUntil)
	return nil
}

func (s *Store) PutKGEntity(e KGEntity) (*KGEntity, error) {
	if err := prepareKGEntity(&e); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[strings.TrimSpace(e.SessionID)]
	key, err := stampKGCustomer(sess, e.SessionID, e.TenantID)
	if err != nil {
		return nil, err
	}
	e.CustomerKey = key
	e.SessionID = ""
	e.ID = s.nextID()
	e.CreatedAt = time.Now().UTC()
	cp := e
	s.kgEntities[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (s *Store) GetKGEntity(id string, sc KGScope) (*KGEntity, error) {
	id = strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.kgEntityLocked(id, sc)
}

func (s *Store) kgEntityLocked(id string, sc KGScope) (*KGEntity, error) {
	e, ok := s.kgEntities[id]
	if !ok || e == nil || !sc.Allows(e.CustomerKey) {
		return nil, ErrNotFound
	}
	cp := *e
	return &cp, nil
}

func (s *Store) PutKGRelation(rel KGRelation) (*KGRelation, error) {
	if err := prepareKGRelation(&rel); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[strings.TrimSpace(rel.SessionID)]
	key, err := stampKGCustomer(sess, rel.SessionID, rel.TenantID)
	if err != nil {
		return nil, err
	}
	rel.CustomerKey = key
	rel.SessionID = ""
	// A channel write may only attach to rows that customer can read.
	// An unscoped write (web/API) may attach to any row in the tenant.
	sc := KGScope{}
	if key != "" {
		sc = KGScope{Scoped: true, CustomerKey: key}
	}
	from, ok := s.kgEntities[rel.FromID]
	if !ok || from == nil || !SameTenant(from.TenantID, rel.TenantID) || !sc.Allows(from.CustomerKey) {
		return nil, errors.New("from entity not found")
	}
	to, ok := s.kgEntities[rel.ToID]
	if !ok || to == nil || !SameTenant(to.TenantID, rel.TenantID) || !sc.Allows(to.CustomerKey) {
		return nil, errors.New("to entity not found")
	}
	if rel.Exclusive {
		s.supersedeExclusiveLocked(rel)
	}
	rel.ID = s.nextID()
	cp := rel
	s.kgRelations[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (s *Store) ListKGRelations(entityID string, sc KGScope) ([]*KGRelation, error) {
	entityID = strings.TrimSpace(entityID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.kgEntityLocked(entityID, sc); err != nil {
		return nil, err
	}
	var out []*KGRelation
	for _, r := range s.kgRelations {
		if r == nil || (r.FromID != entityID && r.ToID != entityID) {
			continue
		}
		if !relationVisibleLocked(s, r, sc) {
			continue
		}
		cp := *r
		out = append(out, &cp)
	}
	if out == nil {
		out = []*KGRelation{}
	}
	return out, nil
}

// relationVisibleLocked reports a neighbour edge the caller may see.
// The edge's own customer_key and both endpoints must be visible, so a
// shared edge cannot name another customer's entity. Caller holds s.mu.
func relationVisibleLocked(s *Store, r *KGRelation, sc KGScope) bool {
	if r == nil || !sc.Allows(r.CustomerKey) {
		return false
	}
	if from := s.kgEntities[r.FromID]; from != nil && !sc.Allows(from.CustomerKey) {
		return false
	}
	if to := s.kgEntities[r.ToID]; to != nil && !sc.Allows(to.CustomerKey) {
		return false
	}
	return true
}

func (s *Store) ListKGGraph(q KGGraphQuery) (*KGGraph, error) {
	s.mu.RLock()
	ents := make([]*KGEntity, 0, len(s.kgEntities))
	for _, e := range s.kgEntities {
		if e == nil {
			continue
		}
		cp := *e
		ents = append(ents, &cp)
	}
	rels := make([]*KGRelation, 0, len(s.kgRelations))
	for _, r := range s.kgRelations {
		if r == nil {
			continue
		}
		cp := *r
		rels = append(rels, &cp)
	}
	s.mu.RUnlock()
	return BuildKGGraph(ents, rels, q), nil
}

func (s *Store) ExpandKG(id string, sc KGScope) (*KGExpand, error) {
	ent, err := s.GetKGEntity(id, sc)
	if err != nil {
		return nil, err
	}
	rels, err := s.ListKGRelations(id, sc)
	if err != nil {
		return nil, err
	}
	out := &KGExpand{Entity: ent, Relations: []KGRelationExpand{}}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range rels {
		if r == nil || !kgCurrent(r.ValidFrom, r.ValidUntil) {
			continue
		}
		if !SameTenant(r.TenantID, ent.TenantID) {
			continue
		}
		view := KGRelationExpand{KGRelation: *r}
		if from := s.kgEntities[r.FromID]; from != nil {
			view.FromName = from.Name
		}
		if to := s.kgEntities[r.ToID]; to != nil {
			view.ToName = to.Name
		}
		out.Relations = append(out.Relations, view)
	}
	return out, nil
}

const progressivePerTier = 25

func (s *Store) SearchProgressive(q, tenant string, sc KGScope) ([]KGSearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []KGSearchHit{}, nil
	}
	tenant = NormalizeTenant(tenant)
	needle := strings.ToLower(q)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var l1, l2 []KGSearchHit
	for _, list := range s.memories {
		for _, m := range list {
			if m == nil || !SameTenant(m.TenantID, tenant) {
				continue
			}
			if strings.Contains(strings.ToLower(m.Body), needle) {
				l1 = append(l1, KGSearchHit{
					ID: m.ID, TenantID: m.TenantID, SessionID: m.SessionID, Kind: m.Kind,
					Snippet: SnippetAround(m.Body, q, 80), Tier: TierL1,
				})
				if len(l1) >= progressivePerTier {
					break
				}
			}
		}
		if len(l1) >= progressivePerTier {
			break
		}
	}
	if len(l1) < progressivePerTier {
		for sid, msgs := range s.messages {
			sess := s.sessions[sid]
			tid := DefaultTenant
			if sess != nil {
				tid = sess.TenantID
			}
			if !SameTenant(tid, tenant) {
				continue
			}
			for _, m := range msgs {
				if m == nil {
					continue
				}
				if strings.Contains(strings.ToLower(m.Content), needle) {
					l1 = append(l1, KGSearchHit{
						ID: m.ID, TenantID: tid, SessionID: sid, Kind: KindMessage,
						Snippet: SnippetAround(m.Content, q, 80), Tier: TierL1,
					})
					if len(l1) >= progressivePerTier {
						break
					}
				}
			}
			if len(l1) >= progressivePerTier {
				break
			}
		}
	}
	for _, e := range s.kgEntities {
		if e == nil || !kgCurrent(e.ValidFrom, e.ValidUntil) || !SameTenant(e.TenantID, tenant) {
			continue
		}
		if !sc.Allows(e.CustomerKey) {
			continue
		}
		blob := strings.ToLower(e.Name + " " + e.Body)
		if strings.Contains(blob, needle) {
			l2 = append(l2, KGSearchHit{
				ID: e.ID, TenantID: e.TenantID, Kind: e.Kind, Name: e.Name,
				Snippet: SnippetAround(e.Name+" "+e.Body, q, 80), Tier: TierL2,
			})
			if len(l2) >= progressivePerTier {
				break
			}
		}
	}
	out := append(l1, l2...)
	if out == nil {
		out = []KGSearchHit{}
	}
	return out, nil
}
