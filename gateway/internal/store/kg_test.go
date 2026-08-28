// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"path/filepath"
	"testing"
)

func TestStore_KGSearchAndExpand(t *testing.T) {
	s := New()
	a, _ := s.CreateAgent(Agent{AgentKey: "kg"})
	sess, _ := s.CreateSession(Session{AgentID: a.ID})
	_, err := s.PutMemory(Memory{SessionID: sess.ID, Body: "episodic billing note"})
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := s.PutKGEntity(KGEntity{Name: "Acme Billing", Kind: "org", Body: "invoices and billing"})
	if err != nil || alpha == nil {
		t.Fatalf("entity %v %#v", err, alpha)
	}
	other, err := s.PutKGEntity(KGEntity{Name: "Zeta Warehouse", Kind: "place", Body: "stock room"})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := s.PutKGRelation(KGRelation{FromID: alpha.ID, ToID: other.ID, Rel: "ships_to", Body: "weekly"})
	if err != nil || rel == nil {
		t.Fatalf("rel %v", err)
	}

	hits, err := s.SearchProgressive("Acme", DefaultTenant)
	if err != nil {
		t.Fatal(err)
	}
	if !hasKGHit(hits, alpha.ID, TierL2) {
		t.Fatalf("named entity missing %#v", hits)
	}
	if hasKGHit(hits, other.ID, TierL2) {
		t.Fatalf("unrelated entity hit %#v", hits)
	}
	l1, err := s.SearchProgressive("billing", DefaultTenant)
	if err != nil || !hasKGHit(l1, alpha.ID, TierL2) {
		t.Fatalf("l2 billing %v %#v", err, l1)
	}
	none, err := s.SearchProgressive("zzzz-absent", DefaultTenant)
	if err != nil || len(none) != 0 {
		t.Fatalf("empty %v %#v", err, none)
	}

	exp, err := s.ExpandKG(alpha.ID)
	if err != nil || exp == nil || exp.Entity == nil || exp.Entity.ID != alpha.ID {
		t.Fatalf("expand %v %#v", err, exp)
	}
	if len(exp.Relations) != 1 || exp.Relations[0].Rel != "ships_to" || exp.Relations[0].ToName != "Zeta Warehouse" {
		t.Fatalf("relations %#v", exp.Relations)
	}
}

func TestSQLiteStore_KGFTSAndExpand(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenSQLite(filepath.Join(dir, "kg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.kgFTS {
		t.Fatal("kg fts not enabled")
	}
	a, _ := s.CreateAgent(Agent{AgentKey: "kg"})
	sess, _ := s.CreateSession(Session{AgentID: a.ID})
	_, _ = s.PutMemory(Memory{SessionID: sess.ID, Body: "episodic pineapple note"})
	named, err := s.PutKGEntity(KGEntity{Name: "Pineapple Co", Kind: "org", Body: "fruit vendor"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.PutKGEntity(KGEntity{Name: "Mango LLC", Kind: "org", Body: "unrelated"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.PutKGRelation(KGRelation{FromID: named.ID, ToID: other.ID, Rel: "supplies"})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := s.SearchProgressive("Pineapple", DefaultTenant)
	if err != nil || !hasKGHit(hits, named.ID, TierL2) {
		t.Fatalf("named %v %#v fts=%v", err, hits, s.kgFTS)
	}
	if hasKGHit(hits, other.ID, TierL2) {
		t.Fatalf("unrelated %#v", hits)
	}
	none, err := s.SearchProgressive("zzzz-absent", DefaultTenant)
	if err != nil || len(none) != 0 {
		t.Fatalf("empty %v %#v", err, none)
	}
	exp, err := s.ExpandKG(named.ID)
	if err != nil || len(exp.Relations) != 1 || exp.Relations[0].ToName != "Mango LLC" {
		t.Fatalf("expand %v %#v", err, exp)
	}
}

func TestStore_ProgressiveKeepsL2WhenL1Full(t *testing.T) {
	s := New()
	a, _ := s.CreateAgent(Agent{AgentKey: "cap"})
	sess, _ := s.CreateSession(Session{AgentID: a.ID})
	for i := 0; i < 40; i++ {
		_, err := s.PutMemory(Memory{SessionID: sess.ID, Body: "shared token note"})
		if err != nil {
			t.Fatal(err)
		}
	}
	ent, err := s.PutKGEntity(KGEntity{Name: "shared token org", Kind: "org"})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := s.SearchProgressive("shared", DefaultTenant)
	if err != nil || !hasKGHit(hits, ent.ID, TierL2) {
		t.Fatalf("l2 dropped under l1 load %v %#v", err, hits)
	}
}

func TestSQLiteStore_ProgressiveKeepsL2AndTenant(t *testing.T) {
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "kg-cap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, _ := s.CreateAgent(Agent{AgentKey: "cap"})
	sess, _ := s.CreateSession(Session{AgentID: a.ID})
	for i := 0; i < 40; i++ {
		if _, err := s.PutMemory(Memory{SessionID: sess.ID, Body: "shared token note"}); err != nil {
			t.Fatal(err)
		}
	}
	ent, err := s.PutKGEntity(KGEntity{Name: "shared token org", Kind: "org"})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := s.PutKGEntity(KGEntity{TenantID: "acme", Name: "shared token other", Kind: "org"})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := s.SearchProgressive("shared", DefaultTenant)
	if err != nil || !hasKGHit(hits, ent.ID, TierL2) {
		t.Fatalf("l2 dropped %v %#v", err, hits)
	}
	if hasKGHit(hits, hidden.ID, TierL2) {
		t.Fatalf("cross-tenant l2 %#v", hits)
	}
	other, err := s.SearchProgressive("shared", "acme")
	if err != nil || !hasKGHit(other, hidden.ID, TierL2) || hasKGHit(other, ent.ID, TierL2) {
		t.Fatalf("acme search %v %#v", err, other)
	}
}

func hasKGHit(hits []KGSearchHit, id, tier string) bool {
	for _, h := range hits {
		if h.ID == id && h.Tier == tier {
			return true
		}
	}
	return false
}
