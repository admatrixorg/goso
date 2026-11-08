// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestB3_SQLiteMigrationFaultIsReplaySafe(t *testing.T) {
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
	if _, err := raw.Exec(`CREATE TABLE vault_links (
		from_id TEXT NOT NULL,
		to_id TEXT NOT NULL DEFAULT '',
		raw TEXT NOT NULL,
		PRIMARY KEY(from_id, raw)
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO vault_docs(id, title, path, sha256, mtime, body, tenant_id) VALUES('a','Chính sách','chinh-sach.md','aa','2026-01-01T00:00:00Z','keep-sales','minh-an-sales')`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO vault_links(from_id, to_id, raw) VALUES('a','','[[Giữ]]')`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	vaultMigrateFault = func(step string) error {
		if step == "copied" {
			return errors.New("injected")
		}
		return nil
	}
	if _, err := OpenSQLite(path); err == nil {
		t.Fatal("faulted migration reported success")
	}
	vaultMigrateFault = nil
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen after fault: %v", err)
	}
	defer s.Close()
	got, err := s.GetVaultDoc("a")
	if err != nil || got.Body != "keep-sales" || got.OwnerExplicit || got.TenantID != "minh-an-sales" {
		t.Fatalf("row after replay %#v %v", got, err)
	}
	var links int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM vault_links WHERE from_id=?`, "a").Scan(&links); err != nil || links != 1 {
		t.Fatalf("links %d %v", links, err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM vault_docs WHERE id=?`, "a").Scan(&n); err != nil || n != 1 {
		t.Fatalf("duplicate rows %d %v", n, err)
	}
	again, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if err := again.db.QueryRow(`SELECT COUNT(*) FROM vault_docs WHERE id=?`, "a").Scan(&n); err != nil || n != 1 {
		t.Fatalf("second reopen %d %v", n, err)
	}
	hits, err := again.SearchVault("minh-an-sales", "keep-sales")
	if err != nil {
		t.Fatal(err)
	}
	// Serving hides owner_explicit=0. The index must still contain the row.
	var indexed int
	if again.vaultFTS {
		if err := again.db.QueryRow(`SELECT COUNT(*) FROM vault_fts WHERE id=?`, "a").Scan(&indexed); err != nil || indexed != 1 {
			t.Fatalf("fts lost the row %d %v", indexed, err)
		}
	}
	_ = hits
}

func TestB3_PostgresMigrationFaultRollsBack(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	dsn := prepareSchema(t, base, "vault_b3_fault", false)
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
	if _, err := raw.Exec(`INSERT INTO vault_docs(id, title, path, sha256, mtime, body, tenant_id) VALUES('a','Chính sách','chinh-sach.md','aa','2026-01-01T00:00:00Z','keep-sales','minh-an-sales')`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	vaultMigrateFault = func(step string) error {
		if step == "postgres-altered" {
			return errors.New("injected")
		}
		return nil
	}
	if _, err := OpenPostgres(dsn); err == nil {
		vaultMigrateFault = nil
		t.Fatal("postgres fault reported success")
	}
	vaultMigrateFault = nil
	check, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var present int
	if err := check.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='vault_docs' AND column_name='owner_explicit'`).Scan(&present); err != nil {
		t.Fatal(err)
	}
	if present != 0 {
		t.Fatalf("fault committed owner_explicit")
	}
	check.Close()
	s, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	got, err := s.GetVaultDoc("a")
	if err != nil || got.Body != "keep-sales" || got.OwnerExplicit {
		t.Fatalf("postgres row %#v %v", got, err)
	}
	if _, err := s.PutVaultDoc(VaultDoc{
		Title: "Khác", Path: "chinh-sach.md", TenantID: "minh-an-legal", Body: "legal", SHA256: "l", OwnerExplicit: true,
	}); err != nil {
		t.Fatal(err)
	}
	again, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	got, err = again.GetVaultDoc("a")
	if err != nil || got.Body != "keep-sales" {
		t.Fatalf("replay %#v %v", got, err)
	}
}
