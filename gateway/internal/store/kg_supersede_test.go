// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestKGExclusiveRelationAndSupersede(t *testing.T) {
	sq, err := OpenSQLite(filepath.Join(t.TempDir(), "kg.db"))
	if err != nil {
		t.Fatal(err)
	}
	stores := map[string]StoreIface{"memory": New(), "sqlite": sq}
	addPostgresStore(t, stores, "kg132")
	for name, s := range stores {
		t.Run(name, func(t *testing.T) {
			an, _ := s.PutKGEntity(KGEntity{Name: "Alpha", Kind: "person"})
			acme, _ := s.PutKGEntity(KGEntity{Name: "Acme", Kind: "org"})
			zeta, _ := s.PutKGEntity(KGEntity{Name: "Zeta", Kind: "org"})
			r1, err := s.PutKGRelation(KGRelation{FromID: an.ID, ToID: acme.ID, Rel: "works_at"})
			if err != nil {
				t.Fatal(err)
			}
			r2, err := s.PutKGRelation(KGRelation{FromID: an.ID, ToID: zeta.ID, Rel: "works_at", Exclusive: true})
			if err != nil {
				t.Fatal(err)
			}
			rels, err := s.ListKGRelations(an.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rels {
				switch r.ID {
				case r1.ID:
					if r.ValidUntil == nil {
						t.Fatalf("old works_at must be closed by exclusive insert: %+v", r)
					}
				case r2.ID:
					if r.ValidUntil != nil {
						t.Fatalf("new works_at must stay current: %+v", r)
					}
				}
			}
			cur, _ := s.ListKGRelations(an.ID)
			foundNew := false
			for _, r := range cur {
				if r.ID == r2.ID {
					foundNew = true
				}
			}
			if !foundNew {
				t.Fatal("new relation missing from list")
			}

			// Explicit supersede: wrong tenant → not found; right tenant → closed.
			if err := s.SupersedeKGRelation("other", r2.ID, time.Time{}); !errors.Is(err, ErrNotFound) {
				t.Fatalf("other tenant must get ErrNotFound, got %v", err)
			}
			if err := s.SupersedeKGRelation(DefaultTenant, r2.ID, time.Time{}); err != nil {
				t.Fatal(err)
			}
			if err := s.SupersedeKGEntity(DefaultTenant, "nope", time.Time{}); !errors.Is(err, ErrNotFound) {
				t.Fatalf("unknown entity: %v", err)
			}
			until := time.Now().UTC().Add(-time.Minute)
			if err := s.SupersedeKGEntity(DefaultTenant, acme.ID, until); err != nil {
				t.Fatal(err)
			}
			// History stays inspectable by id, but the window is closed and no
			// current relation is attached (relations to Acme were superseded).
			exp, err := s.ExpandKG(acme.ID)
			if err != nil || exp == nil || exp.Entity == nil || exp.Entity.ValidUntil == nil {
				t.Fatalf("superseded entity must carry valid_until: %v %+v", err, exp)
			}
			if len(exp.Relations) != 0 {
				t.Fatalf("superseded relations must not be listed as current: %+v", exp.Relations)
			}
			if !KGIsCurrent(exp.Entity.ValidFrom, exp.Entity.ValidUntil) {
				// expected: closed window
			} else {
				t.Fatal("entity must not be current after supersede")
			}
		})
	}
}
