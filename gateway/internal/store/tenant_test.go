// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNormalizeTenant(t *testing.T) {
	if NormalizeTenant("") != DefaultTenant {
		t.Fatal("empty")
	}
	if NormalizeTenant("  ") != DefaultTenant {
		t.Fatal("ws")
	}
	if NormalizeTenant("alpha") != "alpha" {
		t.Fatal("alpha")
	}
	if NormalizeTenant("bad id") != DefaultTenant {
		t.Fatal("space invalid")
	}
	if !SameTenant("", "default") {
		t.Fatal("empty==default")
	}
}

func TestRefusePostgres(t *testing.T) {
	t.Setenv("GOSO_DATABASE_URL", "")
	if err := RefusePostgres("data/goso.db"); err != nil {
		t.Fatalf("sqlite path: %v", err)
	}
	if err := RefusePostgres("postgres://user:pass@localhost:5432/goso"); !errors.Is(err, ErrPostgresUnsupported) {
		t.Fatalf("dsn: %v", err)
	}
	if err := RefusePostgres("postgresql://localhost/goso"); !errors.Is(err, ErrPostgresUnsupported) {
		t.Fatalf("postgresql: %v", err)
	}
	t.Setenv("GOSO_DATABASE_URL", "postgres://localhost/goso")
	if err := RefusePostgres("data/goso.db"); !errors.Is(err, ErrPostgresUnsupported) {
		t.Fatalf("env wins over sqlite path: %v", err)
	}
}

func TestOpenPostgresFailClosed(t *testing.T) {
	t.Setenv("GOSO_DATABASE_URL", "postgres://localhost/goso")
	_, _, err := Open("data/goso.db")
	if !errors.Is(err, ErrPostgresUnsupported) {
		t.Fatalf("Open: %v", err)
	}
	_, err = OpenSQLite("postgresql://localhost/goso")
	if !errors.Is(err, ErrPostgresUnsupported) {
		t.Fatalf("OpenSQLite: %v", err)
	}
}

func TestSQLiteTenantBackfill(t *testing.T) {
	t.Setenv("GOSO_DATABASE_URL", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE agents (
		id TEXT PRIMARY KEY,
		agent_key TEXT NOT NULL UNIQUE,
		display_name TEXT,
		model TEXT,
		llm_provider TEXT,
		created_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agents(id, agent_key, display_name, model, llm_provider, created_at) VALUES('a1','k1','A','','','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.GetAgent("a1")
	if err != nil {
		t.Fatal(err)
	}
	if got.TenantID != DefaultTenant {
		t.Fatalf("backfill tenant %q", got.TenantID)
	}
}

func TestCreateAgentStampsDefaultTenant(t *testing.T) {
	s := New()
	a, err := s.CreateAgent(Agent{AgentKey: "k", DisplayName: "K"})
	if err != nil {
		t.Fatal(err)
	}
	if a.TenantID != DefaultTenant {
		t.Fatalf("tenant %q", a.TenantID)
	}
}
