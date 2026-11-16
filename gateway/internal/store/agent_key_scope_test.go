// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// sameKeyTwoTenants is the N9 contract every backend keeps: two tenants may
// each hold agent key tro-ly-demo, a second one in the same tenant is
// refused, and a lookup in tenant A never returns tenant B's agent.
func sameKeyTwoTenants(t *testing.T, st StoreIface) {
	t.Helper()
	a, err := st.CreateAgent(Agent{TenantID: "minh-an-a", AgentKey: "tro-ly-demo", DisplayName: "Minh An A"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateAgent(Agent{TenantID: "minh-an-b", AgentKey: "tro-ly-demo", DisplayName: "Minh An B"})
	if err != nil {
		t.Fatalf("second tenant, same key: %v", err)
	}
	if a.ID == b.ID {
		t.Fatal("same id")
	}
	if _, err := st.CreateAgent(Agent{TenantID: "minh-an-a", AgentKey: "tro-ly-demo"}); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate in one tenant: %v", err)
	}
	if got := FindAgentByKey(st, "minh-an-a", "tro-ly-demo"); got == nil || got.ID != a.ID {
		t.Fatalf("tenant A got %+v", got)
	}
	if got := FindAgentByKey(st, "minh-an-b", "tro-ly-demo"); got == nil || got.ID != b.ID {
		t.Fatalf("tenant B got %+v", got)
	}
	if got := FindAgentByKey(st, "minh-an-c", "tro-ly-demo"); got != nil {
		t.Fatalf("tenant C got %+v", got)
	}
}

func TestAgentKeyUniquePerTenantMemory(t *testing.T) {
	sameKeyTwoTenants(t, New())
}

func TestAgentKeyUniquePerTenantSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "n9.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sameKeyTwoTenants(t, st)
}

// legacySQLite writes a database the way older builds created it: a
// column-level UNIQUE on agent_key, plus one agent.
func legacySQLite(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`CREATE TABLE agents (
			id TEXT PRIMARY KEY,
			agent_key TEXT NOT NULL UNIQUE,
			display_name TEXT,
			model TEXT,
			llm_provider TEXT,
			created_at TEXT NOT NULL,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			enabled INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`ALTER TABLE agents ADD COLUMN instructions TEXT`,
		`ALTER TABLE agents ADD COLUMN orchestration_mode TEXT`,
		`INSERT INTO agents(id, agent_key, display_name, model, created_at, tenant_id, enabled, updated_at, instructions)
			VALUES('old-1', 'tro-ly-demo', 'Minh An cu', 'sample-model', '2026-01-01T00:00:00Z', 'minh-an-a', 0, '2026-01-02T00:00:00Z', 'Giu nguyen.')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAgentKeyScopeMigratesLegacySQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacySQLite(t, path)
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var ddl string
	if err := st.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='agents'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ddl, "UNIQUE") {
		t.Fatalf("agents still has a column UNIQUE: %s", ddl)
	}
	got, err := st.GetAgent("old-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentKey != "tro-ly-demo" || got.TenantID != "minh-an-a" || got.Enabled || got.Instructions != "Giu nguyen." || got.Model != "sample-model" {
		t.Fatalf("row changed %+v", got)
	}
	b, err := st.CreateAgent(Agent{TenantID: "minh-an-b", AgentKey: "tro-ly-demo"})
	if err != nil {
		t.Fatalf("other tenant, same key: %v", err)
	}
	if _, err := st.CreateAgent(Agent{TenantID: "minh-an-a", AgentKey: "tro-ly-demo"}); !errors.Is(err, ErrExists) {
		t.Fatalf("same tenant duplicate: %v", err)
	}
	if FindAgentByKey(st, "minh-an-a", "tro-ly-demo").ID != "old-1" || FindAgentByKey(st, "minh-an-b", "tro-ly-demo").ID != b.ID {
		t.Fatal("lookup crossed tenants")
	}
	_ = st.Close()
	// A second open is a no-op.
	again, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if n := len(again.ListAgents()); n != 2 {
		t.Fatalf("agents after reopen %d", n)
	}
}

func TestAgentKeyScopeSQLiteIsTransactional(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fault.db")
	legacySQLite(t, path)
	agentKeyMigrateFault = func(step string) error {
		if step == "copied" {
			return errors.New("fault")
		}
		return nil
	}
	if st, err := OpenSQLite(path); err == nil {
		_ = st.Close()
		agentKeyMigrateFault = nil
		t.Fatal("expected fault")
	}
	agentKeyMigrateFault = nil
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if got, err := st.GetAgent("old-1"); err != nil || got.AgentKey != "tro-ly-demo" {
		t.Fatalf("row lost %+v %v", got, err)
	}
	if _, err := st.CreateAgent(Agent{TenantID: "minh-an-b", AgentKey: "tro-ly-demo"}); err != nil {
		t.Fatalf("not migrated after retry: %v", err)
	}
}

func TestAgentKeyUniquePerTenantPostgres(t *testing.T) {
	dsn := postgresTestDSN()
	if dsn == "" {
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	t.Setenv("GOSO_DATABASE_URL", "")
	st, err := OpenPostgres(prepareSchema(t, dsn, "n9_fresh_"+strings.ReplaceAll(newID(), "-", ""), false))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sameKeyTwoTenants(t, st)
}

// An older Postgres database has agents.agent_key UNIQUE. Here its
// constraint carries a non-default name and a stray standalone unique index
// sits on the same column; the migration finds both by catalog lookup.
func TestAgentKeyScopeMigratesLegacyPostgres(t *testing.T) {
	dsn := postgresTestDSN()
	if dsn == "" {
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	t.Setenv("GOSO_DATABASE_URL", "")
	schema := "n9_legacy_" + strings.ReplaceAll(newID(), "-", "")
	scoped := prepareSchema(t, dsn, schema, true)
	raw, err := sql.Open("pgx", scoped)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, q := range []string{
		`ALTER TABLE agents RENAME CONSTRAINT agents_agent_key_key TO legacy_agent_key_uniq`,
		`CREATE UNIQUE INDEX legacy_agent_key_idx ON agents(agent_key)`,
		`INSERT INTO agents(id, agent_key, display_name, model, llm_provider, instructions, orchestration_mode, created_at, tenant_id) VALUES('old-1', 'tro-ly-demo', 'Minh An cu', 'sample-model', '', '', '', '2026-01-01T00:00:00Z', 'minh-an-a')`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	// A failure after the drop rolls the whole step back.
	agentKeyMigrateFault = func(step string) error {
		if step == "postgres-dropped" {
			return errors.New("fault")
		}
		return nil
	}
	if bad, err := OpenPostgres(scoped); err == nil {
		_ = bad.Close()
		agentKeyMigrateFault = nil
		t.Fatal("expected fault")
	}
	agentKeyMigrateFault = nil
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM pg_constraint WHERE conname = 'legacy_agent_key_uniq'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("constraint dropped by a failed migration: %d %v", n, err)
	}
	st, err := OpenPostgres(scoped)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := raw.QueryRow(`SELECT COUNT(*) FROM pg_constraint WHERE conname = 'legacy_agent_key_uniq'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("legacy constraint left %d %v", n, err)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'legacy_agent_key_idx'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("legacy index left %d %v", n, err)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'idx_agents_tenant_key'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("composite index %d %v", n, err)
	}
	b, err := st.CreateAgent(Agent{TenantID: "minh-an-b", AgentKey: "tro-ly-demo"})
	if err != nil {
		t.Fatalf("other tenant, same key: %v", err)
	}
	if _, err := st.CreateAgent(Agent{TenantID: "minh-an-a", AgentKey: "tro-ly-demo"}); !errors.Is(err, ErrExists) {
		t.Fatalf("same tenant duplicate: %v", err)
	}
	if FindAgentByKey(st, "minh-an-a", "tro-ly-demo").ID != "old-1" || FindAgentByKey(st, "minh-an-b", "tro-ly-demo").ID != b.ID {
		t.Fatal("lookup crossed tenants")
	}
	_ = st.Close()
	again, err := OpenPostgres(scoped)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	_ = again.Close()
}
