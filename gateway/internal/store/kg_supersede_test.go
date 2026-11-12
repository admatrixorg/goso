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
			rels, err := s.ListKGRelations(an.ID, KGScope{})
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
			cur, _ := s.ListKGRelations(an.ID, KGScope{})
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
			exp, err := s.ExpandKG(acme.ID, KGScope{})
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

// An exclusive write closes only that customer's current row. Customer Y's
// relation, and a shared one, stay active. Minh An fixtures only.
func TestKGExclusiveSupersedeKeepsOtherCustomers(t *testing.T) {
	for name, st := range kgCustomerStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, err := st.CreateAgent(Agent{AgentKey: "kg-ex-" + name})
			if err != nil {
				t.Fatal(err)
			}
			sx, err := st.CreateSession(Session{AgentID: ag.ID, Label: "zalo-personal:minh-an-x"})
			if err != nil {
				t.Fatal(err)
			}
			sy, err := st.CreateSession(Session{AgentID: ag.ID, Label: "zalo-personal:minh-an-y"})
			if err != nil {
				t.Fatal(err)
			}
			alpha, err := st.PutKGEntity(KGEntity{Name: "Kho Minh An", Kind: "org", AgentID: ag.ID})
			if err != nil {
				t.Fatal(err)
			}
			acme, err := st.PutKGEntity(KGEntity{Name: "Acme Minh An", Kind: "org", AgentID: ag.ID})
			if err != nil {
				t.Fatal(err)
			}
			zeta, err := st.PutKGEntity(KGEntity{Name: "Zeta Minh An", Kind: "org", AgentID: ag.ID})
			if err != nil {
				t.Fatal(err)
			}
			depot, err := st.PutKGEntity(KGEntity{Name: "Kho phu Minh An", Kind: "place", AgentID: ag.ID})
			if err != nil {
				t.Fatal(err)
			}
			yRel, err := st.PutKGRelation(KGRelation{FromID: alpha.ID, ToID: acme.ID, Rel: "works_at", SessionID: sy.ID})
			if err != nil || yRel.CustomerKey != "zalo-personal:minh-an-y" {
				t.Fatalf("Y relation: %+v %v", yRel, err)
			}
			shared, err := st.PutKGRelation(KGRelation{FromID: alpha.ID, ToID: depot.ID, Rel: "works_at"})
			if err != nil || shared.CustomerKey != "" {
				t.Fatalf("shared relation: %+v %v", shared, err)
			}
			xOld, err := st.PutKGRelation(KGRelation{FromID: alpha.ID, ToID: acme.ID, Rel: "works_at", SessionID: sx.ID})
			if err != nil {
				t.Fatal(err)
			}
			xNew, err := st.PutKGRelation(KGRelation{FromID: alpha.ID, ToID: zeta.ID, Rel: "works_at", Exclusive: true, SessionID: sx.ID})
			if err != nil || xNew.CustomerKey != "zalo-personal:minh-an-x" {
				t.Fatalf("X exclusive: %+v %v", xNew, err)
			}
			rels, err := st.ListKGRelations(alpha.ID, KGScope{})
			if err != nil {
				t.Fatal(err)
			}
			byID := map[string]*KGRelation{}
			for _, r := range rels {
				byID[r.ID] = r
			}
			if byID[yRel.ID] == nil || byID[yRel.ID].ValidUntil != nil {
				t.Fatalf("Y's relation must stay active: %+v", byID[yRel.ID])
			}
			if byID[shared.ID] == nil || byID[shared.ID].ValidUntil != nil {
				t.Fatalf("shared relation must stay active: %+v", byID[shared.ID])
			}
			if byID[xOld.ID] == nil || byID[xOld.ID].ValidUntil == nil {
				t.Fatalf("X's previous relation must close: %+v", byID[xOld.ID])
			}
			if byID[xNew.ID] == nil || byID[xNew.ID].ValidUntil != nil {
				t.Fatalf("X's new relation must stay current: %+v", byID[xNew.ID])
			}
		})
	}
}
