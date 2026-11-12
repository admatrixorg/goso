// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func kgCustomerStores(t *testing.T) map[string]StoreIface {
	t.Helper()
	sq, err := OpenSQLite(filepath.Join(t.TempDir(), "kg-cust.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	out := map[string]StoreIface{"memory": New(), "sqlite": sq}
	addPostgresStore(t, out, "goso_kg182")
	return out
}

// Customer X cannot see customer Y's entities or relations. Shared rows
// (customer_key ”) stay visible. Web reads stay unscoped. The key is stamped
// from the session, never from the caller. Minh An fixtures only.
func TestKGCustomerScope_ThreeBackends(t *testing.T) {
	for name, st := range kgCustomerStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, err := st.CreateAgent(Agent{AgentKey: "kg-cust-" + name})
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
			web, err := st.CreateSession(Session{AgentID: ag.ID, Label: "web"})
			if err != nil {
				t.Fatal(err)
			}

			shared, err := st.PutKGEntity(KGEntity{
				Name: "Kho Minh An", Kind: "place", Body: "catalog", AgentID: ag.ID,
				CustomerKey: "zalo-personal:minh-an-y",
			})
			if err != nil || shared.CustomerKey != "" {
				t.Fatalf("unscoped write must ignore caller key: %+v %v", shared, err)
			}
			ownX, err := st.PutKGEntity(KGEntity{
				Name: "Don MA-WALNUT-X", Kind: "order", Body: "walnut X", AgentID: ag.ID,
				SessionID: sx.ID, CustomerKey: "zalo-personal:minh-an-y",
			})
			if err != nil || ownX.CustomerKey != "zalo-personal:minh-an-x" {
				t.Fatalf("stamp from X's session: %+v %v", ownX, err)
			}
			if got, err := st.GetKGEntity(ownX.ID, KGScope{}); err != nil || got.CustomerKey != ownX.CustomerKey {
				t.Fatalf("persisted stamp: %+v %v", got, err)
			}
			ownY, err := st.PutKGEntity(KGEntity{
				Name: "Don MA-WALNUT-Y", Kind: "order", Body: "walnut Y", AgentID: ag.ID, SessionID: sy.ID,
			})
			if err != nil || ownY.CustomerKey != "zalo-personal:minh-an-y" {
				t.Fatalf("stamp Y: %+v %v", ownY, err)
			}
			webEnt, err := st.PutKGEntity(KGEntity{
				Name: "Don MA-WALNUT-WEB", Kind: "order", Body: "walnut web", AgentID: ag.ID, SessionID: web.ID,
			})
			if err != nil || webEnt.CustomerKey != "" {
				t.Fatalf("web write stays shared: %+v %v", webEnt, err)
			}

			relX, err := st.PutKGRelation(KGRelation{FromID: ownX.ID, ToID: shared.ID, Rel: "orders", SessionID: sx.ID})
			if err != nil || relX.CustomerKey != "zalo-personal:minh-an-x" {
				t.Fatalf("relation stamp X: %+v %v", relX, err)
			}
			relY, err := st.PutKGRelation(KGRelation{FromID: ownY.ID, ToID: shared.ID, Rel: "orders", SessionID: sy.ID})
			if err != nil || relY.CustomerKey != "zalo-personal:minh-an-y" {
				t.Fatalf("relation stamp Y: %+v %v", relY, err)
			}
			relShared, err := st.PutKGRelation(KGRelation{FromID: shared.ID, ToID: webEnt.ID, Rel: "lists"})
			if err != nil || relShared.CustomerKey != "" {
				t.Fatalf("shared relation: %+v %v", relShared, err)
			}
			if _, err := st.PutKGRelation(KGRelation{FromID: ownX.ID, ToID: ownY.ID, Rel: "sees", SessionID: sx.ID}); err == nil {
				t.Fatal("X must not attach a relation to Y's entity")
			}

			scopeX := KGScope{Scoped: true, CustomerKey: "zalo-personal:minh-an-x"}
			scopeY := KGScope{Scoped: true, CustomerKey: "zalo-personal:minh-an-y"}

			assertKGSearch := func(sc KGScope, want, deny []string) {
				t.Helper()
				hits, err := st.SearchProgressive("MA-WALNUT", DefaultTenant, sc)
				if err != nil {
					t.Fatal(err)
				}
				for _, id := range want {
					if !hasKGHit(hits, id, TierL2) {
						t.Fatalf("scope %+v missing %s in %#v", sc, id, hits)
					}
				}
				for _, id := range deny {
					if hasKGHit(hits, id, TierL2) {
						t.Fatalf("scope %+v leaked %s in %#v", sc, id, hits)
					}
				}
			}
			assertKGSearch(scopeX, []string{ownX.ID, webEnt.ID}, []string{ownY.ID})
			assertKGSearch(scopeY, []string{ownY.ID, webEnt.ID}, []string{ownX.ID})
			assertKGSearch(KGScope{}, []string{ownX.ID, ownY.ID, webEnt.ID}, nil)

			sharedHits, err := st.SearchProgressive("Kho Minh An", DefaultTenant, scopeX)
			if err != nil || !hasKGHit(sharedHits, shared.ID, TierL2) {
				t.Fatalf("shared entity must stay visible to X: %v %#v", err, sharedHits)
			}
			if _, err := st.GetKGEntity(ownY.ID, scopeX); !errors.Is(err, ErrNotFound) {
				t.Fatalf("X get Y: %v", err)
			}
			if _, err := st.GetKGEntity(shared.ID, scopeX); err != nil {
				t.Fatal(err)
			}
			if _, err := st.ExpandKG(ownY.ID, scopeX); !errors.Is(err, ErrNotFound) {
				t.Fatalf("X expand Y: %v", err)
			}
			exp, err := st.ExpandKG(ownX.ID, scopeX)
			if err != nil || exp == nil || len(exp.Relations) != 1 || exp.Relations[0].ID != relX.ID || exp.Relations[0].ToName != shared.Name {
				t.Fatalf("X neighbours: %v %#v", err, exp)
			}
			rels, err := st.ListKGRelations(shared.ID, scopeX)
			if err != nil {
				t.Fatal(err)
			}
			gotRel := map[string]bool{}
			for _, r := range rels {
				gotRel[r.ID] = true
			}
			if !gotRel[relX.ID] || !gotRel[relShared.ID] || gotRel[relY.ID] {
				t.Fatalf("X relation neighbours: %#v", rels)
			}
			relsY, err := st.ListKGRelations(shared.ID, scopeY)
			if err != nil {
				t.Fatal(err)
			}
			gotRel = map[string]bool{}
			for _, r := range relsY {
				gotRel[r.ID] = true
			}
			if !gotRel[relY.ID] || !gotRel[relShared.ID] || gotRel[relX.ID] {
				t.Fatalf("Y relation neighbours: %#v", relsY)
			}

			g, err := st.ListKGGraph(KGGraphQuery{
				AgentID: ag.ID, Tenant: DefaultTenant, CustomerScoped: true, CustomerKey: scopeX.CustomerKey,
			})
			if err != nil {
				t.Fatal(err)
			}
			nodes := map[string]bool{}
			for _, n := range g.Nodes {
				nodes[n.ID] = true
			}
			if !nodes[shared.ID] || !nodes[ownX.ID] || !nodes[webEnt.ID] || nodes[ownY.ID] {
				t.Fatalf("X graph nodes: %#v", g.Nodes)
			}
			for _, e := range g.Edges {
				if e.ID == relY.ID || e.ToID == ownY.ID || e.FromID == ownY.ID {
					t.Fatalf("X graph leaked Y: %#v", g.Edges)
				}
			}
			gAll, err := st.ListKGGraph(KGGraphQuery{AgentID: ag.ID, Tenant: DefaultTenant})
			if err != nil {
				t.Fatal(err)
			}
			nodes = map[string]bool{}
			for _, n := range gAll.Nodes {
				nodes[n.ID] = true
			}
			if !nodes[ownY.ID] || !nodes[ownX.ID] {
				t.Fatalf("web graph must stay unscoped: %#v", gAll.Nodes)
			}
		})
	}
}

func TestKGCustomerIndexSQL_IsConcurrentAndNotStartup(t *testing.T) {
	if !strings.Contains(kgCustomerIndexSQL, "CONCURRENTLY") {
		t.Fatal("postgres index must be created CONCURRENTLY")
	}
	if strings.Contains(kgCustomerIndexSQL, "CREATE INDEX IF NOT EXISTS") {
		t.Fatal("startup form of CREATE INDEX must not be the documented step")
	}
	if !strings.Contains(kgCustomerIndexSQL, "idx_kg_entities_customer") || !strings.Contains(kgCustomerIndexSQL, "tenant_id, agent_id, customer_key") {
		t.Fatal("entity index must be (tenant_id, agent_id, customer_key)")
	}
	if !strings.Contains(kgCustomerIndexSQL, "idx_kg_relations_customer") || !strings.Contains(kgCustomerIndexSQL, "kg_relations (tenant_id, customer_key)") {
		t.Fatal("relation index must be (tenant_id, customer_key)")
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "qa", "182-kg-customer-scope", "migration.sql"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	for _, needle := range []string{
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_kg_entities_customer",
		"ON kg_entities (tenant_id, agent_id, customer_key)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_kg_relations_customer",
		"ON kg_relations (tenant_id, customer_key)",
		"Dat",
	} {
		if !strings.Contains(doc, needle) {
			t.Fatalf("migration.sql missing %q", needle)
		}
	}
}

func TestKGCustomerKey_PostgresColumnWithoutIndex(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	dsn := prepareSchema(t, base, "goso_kg182_old", false)
	t.Cleanup(func() { dropSchema(t, base, "goso_kg182_old") })
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE kg_entities (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL DEFAULT 'default',
		name TEXT NOT NULL,
		kind TEXT,
		body TEXT,
		valid_from TEXT NOT NULL,
		valid_until TEXT,
		created_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE kg_relations (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL DEFAULT 'default',
		from_id TEXT NOT NULL,
		to_id TEXT NOT NULL,
		rel TEXT NOT NULL,
		body TEXT,
		valid_from TEXT NOT NULL,
		valid_until TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO kg_entities(id, tenant_id, name, kind, body, valid_from, created_at)
		VALUES('e-minh','default','Kho Minh An','place','giu-nguyen','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO kg_relations(id, tenant_id, from_id, to_id, rel, body, valid_from)
		VALUES('r-minh','default','e-minh','e-minh','same','giu-nguyen','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	e, err := st.GetKGEntity("e-minh", KGScope{})
	if err != nil || e.Name != "Kho Minh An" || e.Body != "giu-nguyen" || e.CustomerKey != "" {
		t.Fatalf("entity after add-only migrate: %+v %v", e, err)
	}
	var n int
	if err := st.db.db.QueryRow(`SELECT count(*) FROM kg_entities`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("entity count %d %v", n, err)
	}
	var body, key string
	if err := st.db.db.QueryRow(`SELECT body, customer_key FROM kg_relations WHERE id='r-minh'`).Scan(&body, &key); err != nil || body != "giu-nguyen" || key != "" {
		t.Fatalf("relation after add-only migrate: body %q key %q %v", body, key, err)
	}
	for _, idx := range []string{"idx_kg_entities_customer", "idx_kg_relations_customer"} {
		var found int
		if err := st.db.db.QueryRow(`SELECT count(*) FROM pg_indexes WHERE schemaname=current_schema() AND indexname=?`, idx).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if found != 0 {
			t.Fatalf("index %s must not be created at startup", idx)
		}
	}
	var cols int
	if err := st.db.db.QueryRow(`SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='kg_entities' AND column_name='customer_key'`).Scan(&cols); err != nil || cols != 1 {
		t.Fatalf("column %d %v", cols, err)
	}
}
