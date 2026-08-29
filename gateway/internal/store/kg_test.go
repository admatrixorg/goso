// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"path/filepath"
	"strings"
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

func TestStore_KGGraphAgentScopeCapAndSecrets(t *testing.T) {
	s := New()
	a, err := s.CreateAgent(Agent{AgentKey: "kg-explorer"})
	if err != nil || a == nil {
		t.Fatalf("agent %v", err)
	}
	other, err := s.CreateAgent(Agent{AgentKey: "other"})
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := s.PutKGEntity(KGEntity{Name: "Acme Billing", Kind: "org", Body: "invoices", AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	zeta, err := s.PutKGEntity(KGEntity{Name: "Zeta Warehouse", Kind: "place", Body: "stock", AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	secret, err := s.PutKGEntity(KGEntity{Name: "Safe Name", Kind: "org", Body: "sk-live-abcdefghijk", AgentID: a.ID, Source: KGSourceExtracted})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.PutKGEntity(KGEntity{Name: "Other Co", Kind: "org", Body: "hidden", AgentID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.PutKGRelation(KGRelation{FromID: alpha.ID, ToID: zeta.ID, Rel: "ships_to"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.PutKGRelation(KGRelation{FromID: alpha.ID, ToID: secret.ID, Rel: "mentions", Source: KGSourceExtracted})
	if err != nil {
		t.Fatal(err)
	}

	g, err := s.ListKGGraph(KGGraphQuery{AgentID: a.ID, Tenant: DefaultTenant})
	if err != nil || g == nil {
		t.Fatalf("graph %v %#v", err, g)
	}
	if !g.InferredAreNotFacts {
		t.Fatal("must flag inferred_are_not_facts")
	}
	if g.TotalNodes != 3 || len(g.Nodes) != 3 {
		t.Fatalf("nodes %#v", g)
	}
	if g.TotalEdges != 2 || len(g.Edges) != 2 {
		t.Fatalf("edges %#v", g)
	}
	var sawSecret, sawInferred bool
	for _, n := range g.Nodes {
		if strings.Contains(n.Snippet, "sk-") || strings.Contains(n.Name, "sk-") {
			t.Fatalf("snippet leaked secret %#v", n)
		}
		if n.ID == secret.ID {
			sawSecret = true
			if !n.Inferred || n.Source != KGSourceExtracted {
				t.Fatalf("extracted provenance %#v", n)
			}
		}
	}
	if !sawSecret {
		t.Fatal("missing extracted node")
	}
	for _, e := range g.Edges {
		if e.Rel == "mentions" {
			sawInferred = true
			if !e.Inferred {
				t.Fatalf("extracted edge not inferred %#v", e)
			}
		}
	}
	if !sawInferred {
		t.Fatal("missing extracted edge")
	}

	posted, err := s.ListKGGraph(KGGraphQuery{AgentID: a.ID, Scope: KGSourcePosted, Tenant: DefaultTenant})
	if err != nil || posted.TotalNodes != 2 {
		t.Fatalf("posted scope %v %#v", err, posted)
	}
	capped, err := s.ListKGGraph(KGGraphQuery{AgentID: a.ID, Tenant: DefaultTenant, Limit: 1})
	if err != nil || !capped.Truncated || len(capped.Nodes) != 1 || capped.NodeCap != 1 {
		t.Fatalf("cap %v %#v", err, capped)
	}
	empty, err := s.ListKGGraph(KGGraphQuery{AgentID: other.ID, Q: "Acme", Tenant: DefaultTenant})
	if err != nil || empty.TotalNodes != 0 {
		t.Fatalf("other agent q %v %#v", err, empty)
	}
	byBody, err := s.ListKGGraph(KGGraphQuery{AgentID: a.ID, Q: "invoices", Tenant: DefaultTenant})
	if err != nil || byBody.TotalNodes != 1 || byBody.Nodes[0].ID != alpha.ID {
		t.Fatalf("body search %v %#v", err, byBody)
	}
}

func TestSQLiteStore_KGGraph(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenSQLite(filepath.Join(dir, "kg-graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.CreateAgent(Agent{AgentKey: "kg-sql"})
	if err != nil {
		t.Fatal(err)
	}
	n1, err := s.PutKGEntity(KGEntity{Name: "Alpha", Kind: "org", AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	n2, err := s.PutKGEntity(KGEntity{Name: "Beta", Kind: "org", AgentID: a.ID, Source: KGSourceExtracted})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.PutKGRelation(KGRelation{FromID: n1.ID, ToID: n2.ID, Rel: "knows", Source: KGSourceExtracted})
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.ListKGGraph(KGGraphQuery{AgentID: a.ID, Tenant: DefaultTenant})
	if err != nil || g.TotalNodes != 2 || g.TotalEdges != 1 {
		t.Fatalf("sqlite graph %v %#v", err, g)
	}
	got, err := s.GetKGEntity(n2.ID)
	if err != nil || got.Source != KGSourceExtracted || got.AgentID != a.ID {
		t.Fatalf("roundtrip %#v %v", got, err)
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

func TestSQLiteStore_MigrateKGAgentID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old-kg.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE kg_entities (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL DEFAULT 'default',
		name TEXT NOT NULL,
		kind TEXT,
		body TEXT,
		valid_from TEXT NOT NULL,
		valid_until TEXT,
		created_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO kg_entities(id, tenant_id, name, kind, body, valid_from, created_at) VALUES('e1','default','Acme','org','x','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("migrate old kg_entities: %v", err)
	}
	defer s.Close()
	e, err := s.GetKGEntity("e1")
	if err != nil || e == nil || e.Name != "Acme" {
		t.Fatalf("get after migrate %v %+v", err, e)
	}
	if e.AgentID != "" {
		t.Fatalf("agent_id default %q", e.AgentID)
	}
}
