// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"path/filepath"
	"testing"
	"time"
)

func recallStores(t *testing.T) map[string]StoreIface {
	t.Helper()
	sq, err := OpenSQLite(filepath.Join(t.TempDir(), "recall.db"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]StoreIface{"memory": New(), "sqlite": sq}
	addPostgresStore(t, out, "recall132")
	return out
}

// addPostgresStore adds a fresh-schema Postgres store when
// GOSO_TEST_POSTGRES_DSN (or GOSO_TEST_DATABASE_URL) is set, so the SPEC 132
// recall/prune/supersede paths also run against Postgres (prod VPS).
func addPostgresStore(t *testing.T, stores map[string]StoreIface, schema string) {
	t.Helper()
	dsn := postgresTestDSN()
	if dsn == "" {
		return
	}
	pg, err := OpenPostgres(prepareSchema(t, dsn, schema, false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Close() })
	stores["postgres"] = pg
}

func TestRecallTokens(t *testing.T) {
	toks := RecallTokens("Khách Mẫu Thử Alpha, thanh toán BankDemo! a b")
	want := []string{"khách", "mẫu", "thử", "alpha", "thanh", "toán", "bankdemo"}
	if len(toks) != len(want) {
		t.Fatalf("tokens %v", toks)
	}
	for i := range want {
		if toks[i] != want[i] {
			t.Fatalf("token %d = %q want %q", i, toks[i], want[i])
		}
	}
	if len(RecallTokens("")) != 0 {
		t.Fatal("empty query must give no tokens")
	}
}

func TestRecallMemories_CrossSessionTenantAgent(t *testing.T) {
	for name, s := range recallStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, err := s.CreateAgent(Agent{AgentKey: "sales"})
			if err != nil {
				t.Fatal(err)
			}
			other, _ := s.CreateAgent(Agent{AgentKey: "support"})
			acmeAg, err := s.CreateAgent(Agent{AgentKey: "sales-acme", TenantID: "acme"})
			if err != nil {
				t.Fatal(err)
			}
			s1, _ := s.CreateSession(Session{AgentID: ag.ID})
			s2, _ := s.CreateSession(Session{AgentID: ag.ID})
			s3, _ := s.CreateSession(Session{AgentID: other.ID})
			sa, err := s.CreateSession(Session{AgentID: acmeAg.ID, TenantID: "acme"})
			if err != nil {
				t.Fatal(err)
			}
			put := func(sess *Session, kind, body string) {
				t.Helper()
				if _, err := s.PutMemory(Memory{SessionID: sess.ID, TenantID: sess.TenantID, Kind: kind, Body: body}); err != nil {
					t.Fatal(err)
				}
			}
			put(s1, KindEpisodic, "Khách Mẫu Thử Alpha thích thanh toán chuyển khoản BankDemo")
			put(s1, KindDurable, "Alpha luôn muốn hoá đơn VAT")
			put(s1, KindMessage, "Alpha hỏi về BankDemo trong tin nhắn")
			put(s2, KindEpisodic, "Phiên hiện tại cũng nhắc Alpha và BankDemo")
			put(s3, KindEpisodic, "Agent support cũng biết Alpha BankDemo")
			put(sa, KindEpisodic, "Acme: Alpha BankDemo bí mật")

			hits, err := s.RecallMemories(RecallQuery{TenantID: DefaultTenant, AgentID: ag.ID, ExcludeSession: s2.ID, Query: "Alpha thanh toán BankDemo thế nào?"})
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for _, h := range hits {
				seen[h.Memory.Body] = true
				if h.Score <= 0 {
					t.Fatalf("score must be positive: %+v", h)
				}
				if h.Memory.SessionID == s2.ID {
					t.Fatalf("current session leaked: %+v", h.Memory)
				}
				if h.Memory.SessionID == s3.ID {
					t.Fatalf("other agent leaked: %+v", h.Memory)
				}
				if h.Memory.TenantID != DefaultTenant {
					t.Fatalf("other tenant leaked: %+v", h.Memory)
				}
				if h.Memory.Kind == KindMessage {
					t.Fatalf("raw message leaked: %+v", h.Memory)
				}
			}
			if !seen["Khách Mẫu Thử Alpha thích thanh toán chuyển khoản BankDemo"] {
				t.Fatalf("episodic note of session 1 missing: %+v", hits)
			}
			if !seen["Alpha luôn muốn hoá đơn VAT"] {
				t.Fatalf("durable note missing: %+v", hits)
			}
			if hits[0].Memory.Body != "Khách Mẫu Thử Alpha thích thanh toán chuyển khoản BankDemo" {
				t.Fatalf("best hit should match most tokens, got %q", hits[0].Memory.Body)
			}

			// Without the agent filter the support agent's note is visible, the other tenant never is.
			hits, _ = s.RecallMemories(RecallQuery{TenantID: DefaultTenant, ExcludeSession: s2.ID, Query: "Alpha BankDemo"})
			foundSupport := false
			for _, h := range hits {
				if h.Memory.SessionID == s3.ID {
					foundSupport = true
				}
				if h.Memory.TenantID != DefaultTenant {
					t.Fatalf("tenant leak: %+v", h.Memory)
				}
			}
			if !foundSupport {
				t.Fatal("agent-less recall should include the support agent's note")
			}

			// Empty / no-token query → nothing, no error.
			if hits, err := s.RecallMemories(RecallQuery{TenantID: DefaultTenant, Query: "?!"}); err != nil || len(hits) != 0 {
				t.Fatalf("empty query: %v %v", err, hits)
			}
		})
	}
}

func TestPruneEpisodic_KeepsDurableAndSyncsFTS(t *testing.T) {
	for name, s := range recallStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, _ := s.CreateAgent(Agent{AgentKey: "p"})
			sess, _ := s.CreateSession(Session{AgentID: ag.ID})
			if _, err := s.PutMemory(Memory{SessionID: sess.ID, Kind: KindEpisodic, Body: "episodic zebra note"}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.PutMemory(Memory{SessionID: sess.ID, Kind: KindDurable, Body: "durable zebra fact"}); err != nil {
				t.Fatal(err)
			}
			if n, err := s.PruneEpisodic(time.Now().Add(-time.Hour)); err != nil || n != 0 {
				t.Fatalf("nothing old yet: n=%d err=%v", n, err)
			}
			n, err := s.PruneEpisodic(time.Now().Add(time.Hour))
			if err != nil || n != 1 {
				t.Fatalf("prune n=%d err=%v", n, err)
			}
			list, _ := s.ListMemories(sess.ID)
			if len(list) != 1 || list[0].Kind != KindDurable {
				t.Fatalf("durable must survive: %+v", list)
			}
			hits, err := s.SearchMemory("zebra")
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range hits {
				if h.Kind == KindEpisodic {
					t.Fatalf("pruned note still searchable (FTS out of sync): %+v", h)
				}
			}
		})
	}
}
