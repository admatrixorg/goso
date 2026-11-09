// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func vaultStores(t *testing.T, schema string) map[string]StoreIface {
	t.Helper()
	sq, err := OpenSQLite(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	out := map[string]StoreIface{"memory": New(), "sqlite": sq}
	addPostgresStore(t, out, schema)
	if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
		if _, ok := out["postgres"]; !ok {
			t.Fatal("CI requires GOSO_TEST_POSTGRES_DSN for the vault scope contract")
		}
	}
	return out
}

func TestPutVaultDoc_SameLogicalPathTwoTenants(t *testing.T) {
	for name, s := range vaultStores(t, "vault140_path") {
		t.Run(name, func(t *testing.T) {
			sales, err := s.PutVaultDoc(VaultDoc{
				Title: "Chính sách chiết khấu Minh An", Path: "chinh-sach.md", TenantID: "minh-an-sales",
				Body: "sales-9-percent", SHA256: "s", OwnerExplicit: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			legal, err := s.PutVaultDoc(VaultDoc{
				Title: "Chính sách chiết khấu Minh An", Path: "chinh-sach.md", TenantID: "minh-an-legal",
				Body: "legal-8-percent", SHA256: "l", OwnerExplicit: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if sales.ID == legal.ID || sales.Path != legal.Path {
				t.Fatalf("path collapse sales=%+v legal=%+v", sales, legal)
			}
			got, err := s.GetVaultDoc(sales.ID)
			if err != nil || got.Body != "sales-9-percent" || got.TenantID != "minh-an-sales" {
				t.Fatalf("sales overwritten: %v %#v", err, got)
			}
			got, err = s.GetVaultDoc(legal.ID)
			if err != nil || got.Body != "legal-8-percent" || got.TenantID != "minh-an-legal" {
				t.Fatalf("legal missing: %v %#v", err, got)
			}
			if _, err := s.PutVaultDoc(VaultDoc{
				ID: sales.ID, Title: "forged", Path: "other.md", TenantID: "minh-an-legal",
				Body: "nope", SHA256: "x",
			}); !errors.Is(err, ErrConflict) {
				t.Fatalf("forged id: %v", err)
			}
			got, err = s.GetVaultDoc(sales.ID)
			if err != nil || got.Body != "sales-9-percent" {
				t.Fatalf("forged id wrote sales: %v %#v", err, got)
			}
		})
	}
}

func TestSearchVault_TenantBeforeLimit(t *testing.T) {
	for name, s := range vaultStores(t, "vault140_noise") {
		t.Run(name, func(t *testing.T) {
			for i := 0; i < 55; i++ {
				if _, err := s.PutVaultDoc(VaultDoc{
					Title:         fmt.Sprintf("000-noise-%02d", i),
					Path:          fmt.Sprintf("noise-%02d.md", i),
					TenantID:      "minh-an-legal",
					Body:          "MATOKEN441 legal-only-8841",
					SHA256:        "n",
					OwnerExplicit: true,
				}); err != nil {
					t.Fatal(err)
				}
			}
			sales, err := s.PutVaultDoc(VaultDoc{
				Title: "zzz-chinh-sach", Path: "chinh-sach.md", TenantID: "minh-an-sales",
				Body: "MATOKEN441 sales-9-percent", SHA256: "s", OwnerExplicit: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			hits, err := s.SearchVault("minh-an-sales", "MATOKEN441")
			if err != nil || len(hits) != 1 || hits[0].ID != sales.ID {
				t.Fatalf("sales search %v %#v", err, hits)
			}
			if strings.Contains(hits[0].Snippet, "legal-only-8841") || strings.Contains(hits[0].Title, "noise") {
				t.Fatalf("snippet leaked %q", hits[0].Snippet)
			}
			legalHits, err := s.SearchVault("minh-an-legal", "MATOKEN441")
			if err != nil || len(legalHits) != VaultSearchLimit {
				t.Fatalf("legal limit %v %d", err, len(legalHits))
			}
			for _, h := range legalHits {
				if h.ID == sales.ID || strings.Contains(h.Snippet, "sales-9-percent") {
					t.Fatalf("legal search returned sales %#v", h)
				}
			}
			byTitle, err := s.FindVaultDocByTitle("minh-an-legal", "zzz-chinh-sach")
			if !errors.Is(err, ErrNotFound) || byTitle != nil {
				t.Fatalf("title lookup crossed tenants: %v %#v", err, byTitle)
			}
		})
	}
}

func TestVaultLinks_DoNotCrossTenants(t *testing.T) {
	for name, s := range vaultStores(t, "vault140_links") {
		t.Run(name, func(t *testing.T) {
			sales, err := s.PutVaultDoc(VaultDoc{
				Title: "Chính sách chiết khấu Minh An", Path: "chinh-sach.md", TenantID: "minh-an-sales",
				Body: "see [[Chính sách chiết khấu Minh An]]", SHA256: "s", OwnerExplicit: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			legal, err := s.PutVaultDoc(VaultDoc{
				Title: "Chính sách chiết khấu Minh An", Path: "chinh-sach.md", TenantID: "minh-an-legal",
				Body: "see [[Chính sách chiết khấu Minh An]]", SHA256: "l", OwnerExplicit: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.ReplaceVaultLinks(sales.ID, []string{"Chính sách chiết khấu Minh An"}); err != nil {
				t.Fatal(err)
			}
			if err := s.ReplaceVaultLinks(legal.ID, []string{"Chính sách chiết khấu Minh An"}); err != nil {
				t.Fatal(err)
			}
			if err := s.ReResolveVaultLinks(); err != nil {
				t.Fatal(err)
			}
			ob, ib, err := s.ListVaultDocLinks(sales.ID)
			if err != nil || len(ob) != 1 || ob[0].ToID != sales.ID {
				t.Fatalf("sales outbound %v %#v", err, ob)
			}
			for _, l := range ib {
				if l.FromID == legal.ID {
					t.Fatalf("legal backlink visible on sales %#v", ib)
				}
			}
			ob, _, err = s.ListVaultDocLinks(legal.ID)
			if err != nil || len(ob) != 1 || ob[0].ToID != legal.ID {
				t.Fatalf("legal outbound %v %#v", err, ob)
			}
		})
	}
}

func TestNormalizeVaultPath_RejectsEscape(t *testing.T) {
	s := New()
	for _, p := range []string{"/etc/passwd", "../chinh-sach.md", `..\note.md`, "C:/Windows/note.md", "foo/../../etc/passwd"} {
		if _, err := s.PutVaultDoc(VaultDoc{Title: "x", Path: p, TenantID: "minh-an-sales", Body: "no", SHA256: "n"}); err == nil {
			t.Fatalf("accepted %q", p)
		}
	}
}

func TestSQLiteVault_UpgradeDropsGlobalPathUnique(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE vault_docs (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		path TEXT NOT NULL UNIQUE,
		sha256 TEXT NOT NULL,
		mtime TEXT NOT NULL,
		body TEXT,
		tenant_id TEXT NOT NULL DEFAULT 'default'
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO vault_docs(id, title, path, sha256, mtime, body, tenant_id) VALUES('a','Old','chinh-sach.md','aa','2026-01-01T00:00:00Z','keep-sales','minh-an-sales')`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	legal, err := s.PutVaultDoc(VaultDoc{
		Title: "Chính sách chiết khấu Minh An", Path: "chinh-sach.md", TenantID: "minh-an-legal",
		Body: "legal-8-percent", SHA256: "l", OwnerExplicit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sales, err := s.GetVaultDoc("a")
	if err != nil || sales.Body != "keep-sales" || sales.TenantID != "minh-an-sales" {
		t.Fatalf("upgrade rewrote sales: %v %#v", err, sales)
	}
	if legal.ID == sales.ID {
		t.Fatal("upgrade collapsed path")
	}
	if sales.OwnerExplicit {
		t.Fatal("upgrade assigned an owner to the legacy row")
	}
}

func TestPostgresVault_UpgradeDropsGlobalPathUnique(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	dsn := prepareSchema(t, base, "vault140_upgrade", false)
	raw, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE vault_docs (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		path TEXT NOT NULL UNIQUE,
		sha256 TEXT NOT NULL,
		mtime TEXT NOT NULL,
		body TEXT,
		tenant_id TEXT NOT NULL DEFAULT 'default'
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO vault_docs(id, title, path, sha256, mtime, body, tenant_id) VALUES('a','Old','chinh-sach.md','aa','2026-01-01T00:00:00Z','keep-sales','minh-an-sales')`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.PutVaultDoc(VaultDoc{
		Title: "Chính sách chiết khấu Minh An", Path: "chinh-sach.md", TenantID: "minh-an-legal",
		Body: "legal-8-percent", SHA256: "l", OwnerExplicit: true,
	}); err != nil {
		t.Fatal(err)
	}
	sales, err := s.GetVaultDoc("a")
	if err != nil || sales.Body != "keep-sales" || sales.OwnerExplicit {
		t.Fatalf("postgres upgrade: %v %#v", err, sales)
	}
}
