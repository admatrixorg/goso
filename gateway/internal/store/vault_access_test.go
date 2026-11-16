// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"sort"
	"testing"
)

func TestNormalizeVaultAccess(t *testing.T) {
	cases := []struct {
		scope, agent, team string
		want               [3]string
		bad                bool
	}{
		{"", "", "", [3]string{"tenant", "", ""}, false},
		{"tenant", "a1", "t1", [3]string{"tenant", "", ""}, false},
		{"", "a1", "", [3]string{"agent", "a1", ""}, false},
		{"", "", "t1", [3]string{"team", "", "t1"}, false},
		{"", "a1", "t1", [3]string{}, true},
		{"agent", "", "", [3]string{}, true},
		{"team", "a1", "", [3]string{}, true},
		{"personal", "a1", "t1", [3]string{"personal", "a1", ""}, false},
		{"world", "", "", [3]string{}, true},
	}
	for _, c := range cases {
		sc, a, tm, err := NormalizeVaultAccess(c.scope, c.agent, c.team)
		if c.bad {
			if !errors.Is(err, ErrVaultScope) {
				t.Fatalf("%+v: want ErrVaultScope, got %v", c, err)
			}
			continue
		}
		if err != nil || [3]string{sc, a, tm} != c.want {
			t.Fatalf("%+v: got %q %q %q %v", c, sc, a, tm, err)
		}
	}
}

// vaultAccessFixture stores one doc per scope in tenant minh-an and returns
// their ids by title.
func vaultAccessFixture(t *testing.T, s StoreIface) map[string]string {
	t.Helper()
	ids := map[string]string{}
	docs := []VaultDoc{
		{Title: "chung", Scope: VaultScopeTenant},
		{Title: "rieng-x", Scope: VaultScopeAgent, AgentID: "agent-x"},
		{Title: "rieng-y", Scope: VaultScopeAgent, AgentID: "agent-y"},
		{Title: "nhom-ban-hang", Scope: VaultScopeTeam, TeamID: "team-sales"},
		{Title: "ca-nhan-x", Scope: VaultScopePersonal, AgentID: "agent-x"},
		{Title: "khach-minh-an", Scope: VaultScopeTenant, CustomerKey: "zalo:minh-an"},
		{Title: "khach-khac", Scope: VaultScopeTenant, CustomerKey: "zalo:khach-khac"},
	}
	for _, d := range docs {
		d.Path = d.Title + ".md"
		d.Body = "bang gia thang muoi " + d.Title
		d.SHA256 = d.Title
		d.TenantID = "minh-an"
		d.OwnerExplicit = true
		saved, err := s.PutVaultDoc(d)
		if err != nil {
			t.Fatal(err)
		}
		ids[d.Title] = saved.ID
	}
	return ids
}

func hitTitles(t *testing.T, s StoreIface, v VaultViewer) []string {
	t.Helper()
	hits, err := s.SearchVaultFor("minh-an", "bang gia", v)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, h := range hits {
		d, err := s.GetVaultDoc(h.ID)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, d.Title)
	}
	sort.Strings(out)
	return out
}

func TestSearchVaultFor_Scopes(t *testing.T) {
	for name, s := range vaultStores(t, "vault_access_scopes") {
		t.Run(name, func(t *testing.T) {
			vaultAccessFixture(t, s)
			eq := func(got []string, want ...string) {
				t.Helper()
				sort.Strings(want)
				if len(got) != len(want) {
					t.Fatalf("got %v want %v", got, want)
				}
				for i := range got {
					if got[i] != want[i] {
						t.Fatalf("got %v want %v", got, want)
					}
				}
			}
			eq(hitTitles(t, s, OperatorView), "chung", "rieng-x", "rieng-y", "nhom-ban-hang", "ca-nhan-x", "khach-minh-an", "khach-khac")
			// Agent X: tenant docs, its own, its personal; never agent Y's.
			eq(hitTitles(t, s, VaultViewer{AgentID: "agent-x"}), "chung", "rieng-x", "ca-nhan-x", "khach-minh-an", "khach-khac")
			eq(hitTitles(t, s, VaultViewer{AgentID: "agent-y", TeamIDs: []string{"team-sales"}}), "chung", "rieng-y", "nhom-ban-hang", "khach-minh-an", "khach-khac")
			// Channel: no personal, only shared or this customer's docs.
			eq(hitTitles(t, s, VaultViewer{AgentID: "agent-x", Channel: true, CustomerKey: "zalo:minh-an"}), "chung", "rieng-x", "khach-minh-an")
			eq(hitTitles(t, s, VaultViewer{AgentID: "agent-x", Channel: true}), "chung", "rieng-x")
			// An unknown agent sees tenant docs only.
			eq(hitTitles(t, s, VaultViewer{AgentID: "agent-z"}), "chung", "khach-minh-an", "khach-khac")
		})
	}
}

func TestPutVaultDoc_UpdateKeepsScope(t *testing.T) {
	for name, s := range vaultStores(t, "vault_access_keep") {
		t.Run(name, func(t *testing.T) {
			ids := vaultAccessFixture(t, s)
			got, err := s.GetVaultDoc(ids["rieng-x"])
			if err != nil {
				t.Fatal(err)
			}
			// A body edit without scope fields keeps the agent owner.
			if _, err := s.PutVaultDoc(VaultDoc{ID: got.ID, Title: got.Title, Path: got.Path, Body: "sua", SHA256: "sua", TenantID: "minh-an", OwnerExplicit: true}); err != nil {
				t.Fatal(err)
			}
			again, err := s.GetVaultDoc(got.ID)
			if err != nil || again.Scope != VaultScopeAgent || again.AgentID != "agent-x" {
				t.Fatalf("scope lost: %#v %v", again, err)
			}
			if _, err := s.PutVaultDoc(VaultDoc{Title: "sai", Path: "sai.md", Body: "x", SHA256: "x", TenantID: "minh-an", OwnerExplicit: true, Scope: "agent"}); !errors.Is(err, ErrVaultScope) {
				t.Fatalf("agent scope without agent_id: %v", err)
			}
		})
	}
}

func TestMigrateVaultAccess_OldRowsBecomeTenant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE vault_docs (id TEXT PRIMARY KEY, title TEXT NOT NULL, path TEXT NOT NULL, sha256 TEXT NOT NULL, mtime TEXT NOT NULL, body TEXT NOT NULL, tenant_id TEXT NOT NULL DEFAULT 'default', owner_explicit INTEGER NOT NULL DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO vault_docs(id,title,path,sha256,mtime,body,tenant_id,owner_explicit) VALUES('v1','Quy trinh Minh An','quy-trinh.md','aa','2026-01-01T00:00:00Z','bang gia cu','minh-an',1)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d, err := s.GetVaultDoc("v1")
	if err != nil || d.Scope != VaultScopeTenant || d.AgentID != "" || d.TeamID != "" {
		t.Fatalf("migrated %#v %v", d, err)
	}
	if !(VaultViewer{AgentID: "agent-x"}).Allows(d) || !(VaultViewer{AgentID: "agent-x", Channel: true}).Allows(d) {
		t.Fatal("old doc not visible to an agent")
	}
}
