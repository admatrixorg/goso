// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func linkTenantRule(t *testing.T, st StoreIface) {
	t.Helper()
	a, _ := st.CreateAgent(Agent{TenantID: "minh-an", AgentKey: "sale"})
	b, _ := st.CreateAgent(Agent{TenantID: "minh-an", AgentKey: "kho"})
	c, _ := st.CreateAgent(Agent{TenantID: "demo-shop", AgentKey: "kho"})
	if err := st.AddAgentLink(a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.AddAgentLink(a.ID, c.ID); !errors.Is(err, ErrCrossTenantLink) {
		t.Fatalf("cross-tenant link: %v", err)
	}
	if st.HasAgentLink(a.ID, c.ID) || !st.HasAgentLink(a.ID, b.ID) {
		t.Fatal("links wrong")
	}
}

func TestAgentLinkTenantMemory(t *testing.T) { linkTenantRule(t, New()) }

func TestAgentLinkTenantSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "links.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	linkTenantRule(t, st)
	var tid string
	if err := st.db.QueryRow(`SELECT tenant_id FROM agent_links LIMIT 1`).Scan(&tid); err != nil || tid != "minh-an" {
		t.Fatalf("stored tenant %q %v", tid, err)
	}
}

// A database written before agent_links.tenant_id existed gets the column
// and a backfill from the from agent on open.
func TestAgentLinkTenantBackfillSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-links.db")
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := st.CreateAgent(Agent{TenantID: "minh-an", AgentKey: "sale"})
	b, _ := st.CreateAgent(Agent{TenantID: "minh-an", AgentKey: "kho"})
	for _, q := range []string{
		`DROP INDEX IF EXISTS idx_agent_links_tenant`,
		`DROP TABLE agent_links`,
		`CREATE TABLE agent_links (from_agent_id TEXT NOT NULL, to_agent_id TEXT NOT NULL, PRIMARY KEY(from_agent_id, to_agent_id))`,
		`INSERT INTO agent_links VALUES('` + a.ID + `', '` + b.ID + `')`,
	} {
		if _, err := st.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Close()
	st, err = OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var tid string
	if err := st.db.QueryRow(`SELECT tenant_id FROM agent_links WHERE from_agent_id=?`, a.ID).Scan(&tid); err != nil || tid != "minh-an" {
		t.Fatalf("backfill %q %v", tid, err)
	}
	if !st.HasAgentLink(a.ID, b.ID) {
		t.Fatal("link lost")
	}
}

func hookRule(t *testing.T, st StoreIface) {
	t.Helper()
	hs := Hooks(st)
	if hs == nil {
		t.Fatal("no hook store")
	}
	if _, err := hs.CreateHook(Hook{TenantID: "minh-an"}); err == nil {
		t.Fatal("hook without event accepted")
	}
	h, err := hs.CreateHook(Hook{TenantID: "minh-an", Event: "PreToolUse", Matcher: "zalo__*", HandlerType: "block", Config: `{"message":"tạm khoá"}`, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if h.ID == "" || h.AgentID != "" {
		t.Fatalf("created %+v", h)
	}
	if _, err := hs.GetHook("demo-shop", h.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other tenant get: %v", err)
	}
	if list, _ := hs.ListHooks("demo-shop"); len(list) != 0 {
		t.Fatalf("other tenant list %d", len(list))
	}
	if _, err := hs.UpdateHook(Hook{ID: h.ID, TenantID: "demo-shop", Event: "Stop", HandlerType: "http"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other tenant update: %v", err)
	}
	if err := hs.DeleteHook("demo-shop", h.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other tenant delete: %v", err)
	}
	h.Enabled = false
	h.AgentID = "agent-1"
	h.SecretEnc = "sealed"
	up, err := hs.UpdateHook(*h)
	if err != nil || up.Enabled || up.AgentID != "agent-1" {
		t.Fatalf("update %+v %v", up, err)
	}
	got, err := hs.GetHook("minh-an", h.ID)
	if err != nil || got.Enabled || got.AgentID != "agent-1" || got.SecretEnc != "sealed" || !strings.Contains(got.Config, "tạm khoá") || got.Matcher != "zalo__*" {
		t.Fatalf("get %+v %v", got, err)
	}
	if list, _ := hs.ListHooks("minh-an"); len(list) != 1 {
		t.Fatalf("list %d", len(list))
	}
	if err := hs.DeleteHook("minh-an", h.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := hs.GetHook("minh-an", h.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestHooksMemory(t *testing.T) { hookRule(t, New()) }

func TestHooksSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "hooks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	hookRule(t, st)
}

func TestOrchestrationPostgres(t *testing.T) {
	dsn := postgresTestDSN()
	if dsn == "" {
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	t.Setenv("GOSO_DATABASE_URL", "")
	open := func(name string) *PostgresStore {
		st, err := OpenPostgres(prepareSchema(t, dsn, name+"_"+strings.ReplaceAll(newID(), "-", ""), false))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		return st
	}
	t.Run("links", func(t *testing.T) { linkTenantRule(t, open("orch_links")) })
	t.Run("hooks", func(t *testing.T) { hookRule(t, open("orch_hooks")) })
}
