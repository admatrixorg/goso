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

func (s *Store) PutKGEntity(e KGEntity) (*KGEntity, error) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	e.ID = s.nextID()
	e.CreatedAt = time.Now().UTC()
	cp := e
	s.kgEntities[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (s *Store) GetKGEntity(id string) (*KGEntity, error) {
	id = strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.kgEntities[id]
	if !ok || e == nil {
		return nil, ErrNotFound
	}
	cp := *e
	return &cp, nil
}

func (s *Store) PutKGRelation(rel KGRelation) (*KGRelation, error) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	from, ok := s.kgEntities[rel.FromID]
	if !ok || from == nil {
		return nil, errors.New("from entity not found")
	}
	to, ok := s.kgEntities[rel.ToID]
	if !ok || to == nil {
		return nil, errors.New("to entity not found")
	}
	if !SameTenant(from.TenantID, rel.TenantID) || !SameTenant(to.TenantID, rel.TenantID) {
		return nil, errors.New("from entity not found")
	}
	rel.ID = s.nextID()
	cp := rel
	s.kgRelations[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (s *Store) ListKGRelations(entityID string) ([]*KGRelation, error) {
	entityID = strings.TrimSpace(entityID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.kgEntities[entityID]; !ok {
		return nil, ErrNotFound
	}
	var out []*KGRelation
	for _, r := range s.kgRelations {
		if r == nil {
			continue
		}
		if r.FromID == entityID || r.ToID == entityID {
			cp := *r
			out = append(out, &cp)
		}
	}
	if out == nil {
		out = []*KGRelation{}
	}
	return out, nil
}

func (s *Store) ExpandKG(id string) (*KGExpand, error) {
	ent, err := s.GetKGEntity(id)
	if err != nil {
		return nil, err
	}
	rels, err := s.ListKGRelations(id)
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

func (s *Store) SearchProgressive(q, tenant string) ([]KGSearchHit, error) {
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
