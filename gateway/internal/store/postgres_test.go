// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsPostgresDSN(t *testing.T) {
	if IsPostgresDSN("") || IsPostgresDSN("data/goso.db") || IsPostgresDSN(":memory:") {
		t.Fatal("non-dsn")
	}
	if !IsPostgresDSN("postgres://goso:goso@127.0.0.1:5433/goso?sslmode=disable") {
		t.Fatal("postgres://")
	}
	if !IsPostgresDSN("postgresql://localhost/goso") {
		t.Fatal("postgresql://")
	}
	if !IsPostgresDSN("  POSTGRES://localhost/goso") {
		t.Fatal("case")
	}
}

func TestPgSQLRewrite(t *testing.T) {
	got := pgSQL(`SELECT id FROM agents WHERE id=?`)
	if got != `SELECT id FROM agents WHERE id=$1` {
		t.Fatalf("placeholder: %s", got)
	}
	got = pgSQL(`INSERT OR IGNORE INTO agent_links(from_agent_id, to_agent_id) VALUES(?,?)`)
	if !strings.Contains(got, "INSERT INTO") || strings.Contains(strings.ToUpper(got), "OR IGNORE") {
		t.Fatalf("insert or ignore: %s", got)
	}
	if !strings.Contains(got, "ON CONFLICT DO NOTHING") {
		t.Fatalf("on conflict: %s", got)
	}
	if !strings.Contains(got, "$1") || !strings.Contains(got, "$2") {
		t.Fatalf("args: %s", got)
	}
	got = pgSQL(`SELECT id FROM memories WHERE instr(lower(body), lower(?)) > 0`)
	if strings.Contains(got, "instr(") || !strings.Contains(got, "strpos(") {
		t.Fatalf("instr: %s", got)
	}
	if !strings.Contains(got, "$1") {
		t.Fatalf("instr placeholder: %s", got)
	}
}

func TestOpenSQLitePathUnchanged(t *testing.T) {
	t.Setenv("GOSO_DATABASE_URL", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "goso.db")
	st, closer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closer()
	if _, ok := st.(*SQLiteStore); !ok {
		t.Fatalf("want *SQLiteStore, got %T", st)
	}
	a, err := st.CreateAgent(Agent{AgentKey: "k", DisplayName: "K"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetAgent(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentKey != "k" || got.TenantID != DefaultTenant {
		t.Fatalf("roundtrip %+v", got)
	}
}

func TestOpenPostgresDSNWithoutServerErrors(t *testing.T) {
	// Port 1 is not the compose postgres host port (5433) and not a demo port.
	dsn := "postgres://goso:goso@127.0.0.1:1/goso?sslmode=disable"
	t.Setenv("GOSO_DATABASE_URL", dsn)
	sqlitePath := filepath.Join(t.TempDir(), "must-not-create.db")
	st, closer, err := Open(sqlitePath)
	if closer != nil {
		_ = closer()
	}
	if err == nil {
		t.Fatal("expected connect error")
	}
	if st != nil {
		t.Fatal("no store on connect fail")
	}
	if errors.Is(err, ErrPostgresUnsupported) {
		t.Fatalf("Open must try Postgres, not refuse-always: %v", err)
	}
	if _, statErr := os.Stat(sqlitePath); statErr == nil {
		t.Fatal("must not fall back to sqlite file")
	}

	t.Setenv("GOSO_DATABASE_URL", "")
	st2, closer2, err := Open(dsn)
	if closer2 != nil {
		_ = closer2()
	}
	if err == nil || st2 != nil {
		t.Fatal("path DSN without server must error")
	}
	if errors.Is(err, ErrPostgresUnsupported) {
		t.Fatalf("path DSN must connect-fail, not refuse: %v", err)
	}
}

func TestOpenSQLiteStillRefusesPostgresDSN(t *testing.T) {
	t.Setenv("GOSO_DATABASE_URL", "")
	_, err := OpenSQLite("postgres://goso:goso@127.0.0.1:5433/goso?sslmode=disable")
	if !errors.Is(err, ErrPostgresUnsupported) {
		t.Fatalf("OpenSQLite path: %v", err)
	}
	t.Setenv("GOSO_DATABASE_URL", "postgres://goso:goso@127.0.0.1:1/goso?sslmode=disable")
	_, err = OpenSQLite("data/goso.db")
	if !errors.Is(err, ErrPostgresUnsupported) {
		t.Fatalf("OpenSQLite env: %v", err)
	}
}

func TestPostgresRoundTrip(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("GOSO_TEST_DATABASE_URL unset")
	}
	t.Setenv("GOSO_DATABASE_URL", "")
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	key := "pg-rt-" + newID()
	a, err := st.CreateAgent(Agent{AgentKey: key, DisplayName: "PG RT", TenantID: DefaultTenant})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetAgent(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentKey != key || got.TenantID != DefaultTenant {
		t.Fatalf("agent %+v", got)
	}
	sess, err := st.CreateSession(Session{AgentID: a.ID, Label: "s1", TenantID: DefaultTenant})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := st.GetSession(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AgentID != a.ID || loaded.TenantID != DefaultTenant {
		t.Fatalf("session %+v", loaded)
	}
	if _, err := st.AddMessage(Message{SessionID: sess.ID, Role: "user", Content: "pgvector path needle"}); err != nil {
		t.Fatal(err)
	}
	hits, err := st.SearchMemory("needle")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("lexical search empty")
	}
}
