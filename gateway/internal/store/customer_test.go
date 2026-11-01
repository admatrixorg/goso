// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestCustomerKeyForLabel(t *testing.T) {
	cases := map[string]string{
		"zalo-personal:123":  "zalo-personal:123",
		"zalo-oa:u9":         "zalo-oa:u9",
		"telegram:-100":      "telegram:-100",
		"discord:c1":         "discord:c1",
		"slack:C1":           "slack:C1",
		"feishu:x":           "feishu:x",
		"whatsapp:+84":       "whatsapp:+84",
		"zalo-personal:":     "",
		"Zalo-Personal:123":  "",
		"omni:zalo:1":        "",
		"web chat":           "",
		"":                   "",
		" zalo-personal:123": "",
	}
	for in, want := range cases {
		if got := CustomerKeyForLabel(in); got != want {
			t.Errorf("CustomerKeyForLabel(%q)=%q want %q", in, got, want)
		}
	}
}

// Writes are stamped from the session label, never from the caller, and
// channel recall only matches the same agent + customer key (fail closed).
func TestRecallMemories_CustomerScope(t *testing.T) {
	for name, st := range recallStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, _ := st.CreateAgent(Agent{AgentKey: "zalo-" + name})
			other, _ := st.CreateAgent(Agent{AgentKey: "zalo2-" + name})
			a1, _ := st.CreateSession(Session{AgentID: ag.ID, Label: "zalo-personal:A"})
			a2, _ := st.CreateSession(Session{AgentID: ag.ID, Label: "zalo-personal:A"})
			b1, _ := st.CreateSession(Session{AgentID: ag.ID, Label: "zalo-personal:B"})
			web, _ := st.CreateSession(Session{AgentID: ag.ID, Label: "web"})
			oa, _ := st.CreateSession(Session{AgentID: other.ID, Label: "zalo-personal:A"})

			m, err := st.PutMemory(Memory{SessionID: a1.ID, Kind: KindDurable, Body: "khách A đặt sofa walnut", CustomerKey: "zalo-personal:B"})
			if err != nil {
				t.Fatal(err)
			}
			if m.CustomerKey != "zalo-personal:A" {
				t.Fatalf("stamp from session, not caller: %q", m.CustomerKey)
			}
			if got, _ := st.GetMemory(m.ID); got == nil || got.CustomerKey != "zalo-personal:A" {
				t.Fatalf("persisted stamp: %+v", got)
			}
			sum, err := st.SaveSummary(b1.ID, "khách B hỏi sofa walnut giá")
			if err != nil || sum.CustomerKey != "zalo-personal:B" {
				t.Fatalf("summary stamp: %+v %v", sum, err)
			}
			w, _ := st.SaveSummary(web.ID, "web sofa walnut ghi chú")
			if w.CustomerKey != "" {
				t.Fatalf("web note must be unscoped: %q", w.CustomerKey)
			}
			_, _ = st.PutMemory(Memory{SessionID: oa.ID, Kind: KindDurable, Body: "agent khác sofa walnut"})

			q := func(agent, key, exclude string, scoped bool) []RecallHit {
				t.Helper()
				hits, err := st.RecallMemories(RecallQuery{AgentID: agent, ExcludeSession: exclude, Query: "sofa walnut",
					CustomerScoped: scoped, CustomerKey: key})
				if err != nil {
					t.Fatal(err)
				}
				return hits
			}
			hits := q(ag.ID, "zalo-personal:A", a2.ID, true)
			if len(hits) != 1 || hits[0].Memory.SessionID != a1.ID || hits[0].Memory.CustomerKey != "zalo-personal:A" {
				t.Fatalf("A sees only A's note: %+v", hits)
			}
			for _, h := range q(ag.ID, "zalo-personal:B", "", true) {
				if h.Memory.CustomerKey != "zalo-personal:B" {
					t.Fatalf("B leaked %+v", h.Memory)
				}
			}
			if hits := q(ag.ID, "", a2.ID, true); len(hits) != 0 {
				t.Fatalf("empty key must match nothing: %+v", hits)
			}
			if hits := q("", "zalo-personal:A", a2.ID, true); len(hits) != 0 {
				t.Fatalf("scoped recall without agent must match nothing: %+v", hits)
			}
			// Web/API recall is agent-wide as before (A, B and web notes).
			if hits := q(ag.ID, "", "", false); len(hits) != 3 {
				t.Fatalf("web recall agent-wide: %d %+v", len(hits), hits)
			}
		})
	}
}

// legacyMemoriesDDL is the memories table before customer_key existed.
const legacyMemoriesDDL = `CREATE TABLE memories (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	body TEXT NOT NULL,
	created_at TEXT NOT NULL,
	tenant_id TEXT NOT NULL DEFAULT 'default'
)`

// seedLegacy writes sessions + unscoped notes the way an old binary did.
func seedLegacy(t *testing.T, exec func(string, ...any) error, ph func(int) string) {
	t.Helper()
	now := formatTime(time.Now().UTC())
	ins := func(q string, args ...any) {
		t.Helper()
		if err := exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	ins(`INSERT INTO agents(id, agent_key, created_at) VALUES(`+ph(1)+`,`+ph(2)+`,`+ph(3)+`)`, "ag1", "zalo-legacy", now)
	for _, s := range [][2]string{{"sA", "zalo-personal:A"}, {"sB", "zalo-oa:B"}, {"sW", "web chat"}, {"sE", "zalo-personal:"}, {"sU", "ZALO-PERSONAL:X"}} {
		ins(`INSERT INTO sessions(id, agent_id, label, created_at) VALUES(`+ph(1)+`,`+ph(2)+`,`+ph(3)+`,`+ph(4)+`)`, s[0], "ag1", s[1], now)
	}
	for _, m := range [][2]string{{"mA", "sA"}, {"mB", "sB"}, {"mW", "sW"}, {"mE", "sE"}, {"mU", "sU"}} {
		ins(`INSERT INTO memories(id, session_id, kind, body, created_at) VALUES(`+ph(1)+`,`+ph(2)+`,`+ph(3)+`,`+ph(4)+`,`+ph(5)+`)`,
			m[0], m[1], KindEpisodic, "legacy sofa walnut "+m[0], now)
	}
}

// assertBackfill checks the deterministic backfill and that unscoped legacy
// notes never reach channel recall while web recall still sees them.
func assertBackfill(t *testing.T, st StoreIface) {
	t.Helper()
	want := map[string]string{"mA": "zalo-personal:A", "mB": "zalo-oa:B", "mW": "", "mE": "", "mU": ""}
	for id, key := range want {
		m, err := st.GetMemory(id)
		if err != nil || m.CustomerKey != key {
			t.Fatalf("%s customer_key=%q want %q (%v)", id, m.CustomerKey, key, err)
		}
	}
	hits, err := st.RecallMemories(RecallQuery{AgentID: "ag1", Query: "sofa walnut", CustomerScoped: true, CustomerKey: "zalo-personal:A"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Memory.ID != "mA" {
		t.Fatalf("channel A recall after upgrade: %+v", hits)
	}
	hits, err = st.RecallMemories(RecallQuery{AgentID: "ag1", Query: "sofa walnut"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 5 {
		t.Fatalf("web recall must keep all legacy notes, got %d", len(hits))
	}
}

func TestMemoryCustomerKey_SQLiteUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE agents (id TEXT PRIMARY KEY, agent_key TEXT NOT NULL UNIQUE, display_name TEXT, model TEXT, llm_provider TEXT, instructions TEXT, created_at TEXT NOT NULL)`,
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, label TEXT, created_at TEXT NOT NULL)`,
		`CREATE TABLE messages (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, role TEXT NOT NULL, content TEXT NOT NULL, created_at TEXT NOT NULL)`,
		legacyMemoriesDDL,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	seedLegacy(t, func(q string, a ...any) error { _, err := db.Exec(q, a...); return err }, func(int) string { return "?" })
	_ = db.Close()

	for i := 0; i < 2; i++ { // second open: idempotent
		st, err := OpenSQLite(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		assertBackfill(t, st)
		var n int
		if err := st.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_memories_customer'`).Scan(&n); err != nil || n != 1 {
			t.Fatalf("index: %d %v", n, err)
		}
		_ = st.Close()
	}
}

func TestMemoryCustomerKey_SQLiteFresh(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if ok, err := st.hasColumn("memories", "customer_key"); err != nil || !ok {
		t.Fatalf("fresh column: %v %v", ok, err)
	}
}

func TestMemoryCustomerKey_PostgresFreshAndUpgrade(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	t.Run("fresh", func(t *testing.T) {
		dsn := prepareSchema(t, base, "goso_custkey_fresh", false)
		t.Cleanup(func() { dropSchema(t, base, "goso_custkey_fresh") })
		st, err := OpenPostgres(dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		var n int
		if err := st.db.db.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_schema='goso_custkey_fresh' AND table_name='memories' AND column_name='customer_key'`).Scan(&n); err != nil || n != 1 {
			t.Fatalf("fresh column: %d %v", n, err)
		}
	})
	t.Run("upgrade", func(t *testing.T) {
		dsn := prepareSchema(t, base, "goso_custkey_old", true)
		t.Cleanup(func() { dropSchema(t, base, "goso_custkey_old") })
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(legacyMemoriesDDL); err != nil {
			t.Fatal(err)
		}
		seedLegacy(t, func(q string, a ...any) error { _, err := db.Exec(q, a...); return err },
			func(i int) string { return "$" + string(rune('0'+i)) })
		_ = db.Close()
		for i := 0; i < 2; i++ {
			st, err := OpenPostgres(dsn)
			if err != nil {
				t.Fatalf("open %d: %v", i, err)
			}
			assertBackfill(t, st)
			var n int
			if err := st.db.db.QueryRow(`SELECT count(*) FROM pg_indexes WHERE schemaname='goso_custkey_old' AND indexname='idx_memories_customer'`).Scan(&n); err != nil || n != 1 {
				t.Fatalf("index: %d %v", n, err)
			}
			_ = st.Close()
		}
	})
}

func dropSchema(t *testing.T, dsn, schema string) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return
	}
	defer db.Close()
	_, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
}
