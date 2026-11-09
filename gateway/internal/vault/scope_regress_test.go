// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestSync_TenantADoesNotDeleteB(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	st := store.New()
	legal, err := st.PutVaultDoc(store.VaultDoc{
		Title:    "Chính sách chiết khấu Minh An",
		Path:     "chinh-sach.md",
		TenantID: "minh-an-legal",
		Body:     "legal-only-8-percent",
		SHA256:   "legal",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := StorageDir(root, "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sales.md"), []byte("# Bán hàng\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := New(st, root)
	res, err := svc.SyncTenant("minh-an-sales")
	if err != nil || res.Upserted != 1 || res.Deleted != 0 {
		t.Fatalf("sync %v %#v", err, res)
	}
	got, err := st.GetVaultDoc(legal.ID)
	if err != nil || got == nil || got.Body != "legal-only-8-percent" || got.TenantID != "minh-an-legal" {
		t.Fatalf("sync changed legal tenant doc: %v %#v", err, got)
	}
	if _, err := svc.Sync(); !errors.Is(err, ErrSyncDisabled) {
		t.Fatalf("unscoped Sync ran: %v", err)
	}
}

func TestSync_TenantADoesNotDeleteB_SQLiteAndPostgres(t *testing.T) {
	stores := map[string]store.StoreIface{}
	sq, err := store.OpenSQLite(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	stores["sqlite"] = sq
	if pg := openVaultPostgres(t); pg != nil {
		stores["postgres"] = pg
	}
	for name, st := range stores {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
			legal, err := st.PutVaultDoc(store.VaultDoc{
				Title: "Chính sách chiết khấu Minh An", Path: "chinh-sach.md", TenantID: "minh-an-legal",
				Body: "legal-only-8-percent", SHA256: "legal",
			})
			if err != nil {
				t.Fatal(err)
			}
			dir, err := StorageDir(root, "minh-an-sales")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "sales.md"), []byte("# Bán hàng\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			svc := New(st, root)
			res, err := svc.SyncTenant("minh-an-sales")
			if err != nil || res.Deleted != 0 {
				t.Fatalf("sync %v %#v", err, res)
			}
			got, err := st.GetVaultDoc(legal.ID)
			if err != nil || got.Body != "legal-only-8-percent" || got.TenantID != "minh-an-legal" {
				t.Fatalf("sql sync changed legal: %v %#v", err, got)
			}
		})
	}
}

func openVaultPostgres(t *testing.T) store.StoreIface {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if dsn == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		return nil
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		t.Fatal(err)
	}
	const schema = "vault_sync_fs"
	if _, err := admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	st, err := store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSync_TwoTenantsSameTitlePath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	svc := New(store.New(), root)
	sales, err := svc.PutTenant("Chính sách chiết khấu Minh An", "sales-9-percent", "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	legal, err := svc.PutTenant("Chính sách chiết khấu Minh An", "legal-8-percent", "minh-an-legal")
	if err != nil {
		t.Fatal(err)
	}
	wantPath := Slug("Chính sách chiết khấu Minh An") + ".md"
	if sales.ID == legal.ID || sales.Path != wantPath || legal.Path != sales.Path {
		t.Fatalf("paths want %s %#v %#v", wantPath, sales, legal)
	}
	got, err := svc.Get("minh-an-sales", legal.ID)
	if !errors.Is(err, store.ErrNotFound) || got != nil {
		t.Fatalf("sales read legal: %v %#v", err, got)
	}
	got, err = svc.Get("minh-an-sales", sales.ID)
	if err != nil || got.Body != "sales-9-percent" {
		t.Fatalf("sales get %v %#v", err, got)
	}
	hits, err := svc.Search("minh-an-sales", "8-percent")
	if err != nil || len(hits) != 0 {
		t.Fatalf("search leaked %v %#v", err, hits)
	}
	hits, err = svc.Search("minh-an-legal", "legal-8-percent")
	if err != nil || len(hits) != 1 || hits[0].ID != legal.ID || strings.Contains(hits[0].Snippet, "sales-9") {
		t.Fatalf("legal search %v %#v", err, hits)
	}
	if _, err := svc.PutTenant("Chính sách chiết khấu Minh An", "overwrite", "minh-an-sales"); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Get("minh-an-legal", legal.ID)
	if err != nil || !strings.Contains(got.Body, "legal-8-percent") {
		t.Fatalf("sales write changed legal: %v %#v", err, got)
	}
	res, err := svc.SyncTenant("minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 0 {
		t.Fatalf("sales sync deleted %d", res.Deleted)
	}
	got, err = svc.Get("minh-an-legal", legal.ID)
	if err != nil || !strings.Contains(got.Body, "legal-8-percent") {
		t.Fatalf("sync changed legal: %v %#v", err, got)
	}
	h, err := svc.Health("minh-an-sales")
	if err != nil || h.Docs != 1 {
		t.Fatalf("health count %v %#v", err, h)
	}
	g := svc.Graph(40, "minh-an-sales")
	if len(g.Nodes) != 1 || g.Nodes[0].ID != sales.ID {
		t.Fatalf("graph %#v", g)
	}
	for _, n := range g.Nodes {
		if strings.Contains(n.Title, "legal") {
			t.Fatalf("graph leaked %#v", g)
		}
	}
}

func TestSync_EmptyErrorPartialScanDoesNotMassDelete(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	svc := New(store.New(), root)
	doc, err := svc.PutTenant("Giữ lại", "keep-me", "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	svc.collectHook = func(string) ([]diskFile, error) { return nil, nil }
	res, err := svc.SyncTenant("minh-an-sales")
	if err != nil || res.Deleted != 0 {
		t.Fatalf("empty scan %v %#v", err, res)
	}
	if _, err := svc.Get("minh-an-sales", doc.ID); err != nil {
		t.Fatal(err)
	}
	svc.collectHook = func(string) ([]diskFile, error) { return nil, ErrScan }
	if _, err := svc.SyncTenant("minh-an-sales"); err == nil {
		t.Fatal("partial scan reported success")
	}
	if _, err := svc.Get("minh-an-sales", doc.ID); err != nil {
		t.Fatal(err)
	}
	svc.collectHook = nil
	dir, err := StorageDir(root, "minh-an-legal")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("not-a-directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	legal, err := svc.Store.PutVaultDoc(store.VaultDoc{
		Title: "Pháp chế", Path: "note.md", TenantID: "minh-an-legal", Body: "stay", SHA256: "l", OwnerExplicit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SyncTenant("minh-an-legal"); err == nil {
		t.Fatal("bad directory scan reported success")
	}
	if got, err := svc.Store.GetVaultDoc(legal.ID); err != nil || got.Body != "stay" {
		t.Fatalf("bad scan deleted legal: %v %#v", err, got)
	}
}

func TestSync_FlagOffDoesNotGlobalSync(t *testing.T) {
	root := t.TempDir()
	svc := New(store.New(), root)
	doc, err := svc.PutTenant("Giữ", "original", "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := StorageDir(root, "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	dropped := filepath.Join(dir, "new.md")
	if err := os.WriteFile(dropped, []byte("# Mới\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SyncTenant("minh-an-sales"); !errors.Is(err, ErrSyncDisabled) {
		t.Fatalf("flag off sync: %v", err)
	}
	if _, err := svc.Sync(); !errors.Is(err, ErrSyncDisabled) {
		t.Fatalf("unscoped sync: %v", err)
	}
	got, err := svc.Get("minh-an-sales", doc.ID)
	if err != nil || got.Body != "original" {
		t.Fatalf("flag off mutated: %v %#v", err, got)
	}
	if _, err := svc.Store.FindVaultDocByTitle("minh-an-sales", "Mới"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("flag off imported a file: %v", err)
	}
}

type putFail struct{ store.StoreIface }

func (p putFail) PutVaultDoc(store.VaultDoc) (*store.VaultDoc, error) {
	return nil, errors.New("db write failed")
}

type delFail struct{ store.StoreIface }

func (d delFail) DeleteVaultDoc(string) error { return errors.New("db delete failed") }

func TestSync_DBFailureIsNotSuccess(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	inner := store.New()
	legal, err := inner.PutVaultDoc(store.VaultDoc{
		Title: "Pháp chế", Path: "chinh-sach.md", TenantID: "minh-an-legal", Body: "legal-stay", SHA256: "l",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := StorageDir(root, "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := New(putFail{inner}, root)
	if _, err := svc.SyncTenant("minh-an-sales"); err == nil || !strings.Contains(err.Error(), "db write failed") {
		t.Fatalf("fake success: %v", err)
	}
	if got, err := inner.GetVaultDoc(legal.ID); err != nil || got.Body != "legal-stay" {
		t.Fatalf("failed sync touched legal: %v %#v", err, got)
	}

	ok := New(inner, root)
	first, err := ok.PutTenant("Một", "one", "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ok.PutTenant("Hai", "two", "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	secondFile, err := ok.namespaced("minh-an-sales", second.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(secondFile); err != nil {
		t.Fatal(err)
	}
	failDel := New(delFail{inner}, root)
	if _, err := failDel.SyncTenant("minh-an-sales"); err == nil {
		t.Fatal("delete failure reported success")
	}
	if _, err := inner.GetVaultDoc(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := inner.GetVaultDoc(second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := inner.GetVaultDoc(legal.ID); err != nil {
		t.Fatal(err)
	}
}

func TestVault_PathEscapeSymlinkAbsolute(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	svc := New(store.New(), root)
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secret, []byte("# Secret\nlegal-only-8841\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, err := StorageDir(root, "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "leak.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ok.md"), []byte("# Ổn\ninside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "agents", "cfo")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "note.md"), []byte("# Nội bộ\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := svc.SyncTenant("minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	if res.Upserted != 2 {
		t.Fatalf("imported symlink or skipped real files: %#v", res)
	}
	for _, d := range svc.List() {
		if strings.Contains(d.Body, "legal-only-8841") || d.TenantID != "minh-an-sales" {
			t.Fatalf("scope from symlink or path: %#v", d)
		}
		if d.Path == "agents/cfo/note.md" && d.TenantID != "minh-an-sales" {
			t.Fatal("path became a tenant")
		}
	}
	if _, err := svc.namespaced("minh-an-sales", "../chinh-sach.md"); err == nil {
		t.Fatal("traversal accepted")
	}
	if _, err := svc.namespaced("minh-an-sales", "/etc/passwd"); err == nil {
		t.Fatal("absolute path accepted")
	}
	inv, err := svc.Inventory("minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range inv.Entries {
		if e.Path == "agents/cfo/note.md" && !strings.Contains(e.Reason, "path_not_authority") {
			t.Fatalf("path treated as authority %#v", e)
		}
		if e.Tenant != "" && e.Tenant != "minh-an-sales" {
			t.Fatalf("inventory assigned another tenant %#v", e)
		}
	}
}

func TestInventory_DryRunDoesNotMutate(t *testing.T) {
	root := t.TempDir()
	svc := New(store.New(), root)
	explicit, err := svc.PutTenant("Đã rõ", "sales-body", "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	unclear, err := svc.Store.PutVaultDoc(store.VaultDoc{
		Title: "Chưa rõ", Path: "unclear.md", TenantID: "minh-an-sales", Body: "legacy-body", SHA256: "u",
	})
	if err != nil {
		t.Fatal(err)
	}
	legal, err := svc.PutTenant("Pháp chế", "legal-body", "minh-an-legal")
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root, "legacy-unclear.md")
	if err := os.WriteFile(legacy, []byte("# Legacy\nkeep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.Inventory("minh-an-sales")
	if err != nil || !first.DryRun || first.Applied {
		t.Fatalf("inventory %#v %v", first, err)
	}
	second, err := svc.Inventory("minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != len(second.Entries) || len(first.Entries) < 2 {
		t.Fatalf("replay changed the manifest %d %d", len(first.Entries), len(second.Entries))
	}
	var sawUnclear, sawMapped bool
	for _, e := range second.Entries {
		if strings.Contains(e.Path, "legacy-unclear") || e.DocID == legal.ID || strings.Contains(e.Reason, "legal-body") {
			t.Fatalf("manifest leaked %#v", e)
		}
		if e.DocID == unclear.ID {
			sawUnclear = e.Action == "quarantine" && strings.Contains(e.Reason, "unclear_owner")
		}
		if e.DocID == explicit.ID {
			sawMapped = e.Action == "mapped" && strings.Contains(e.Reason, "registry_explicit")
		}
	}
	if !sawUnclear || !sawMapped {
		t.Fatalf("manifest %#v", second.Entries)
	}
	after, err := os.ReadFile(legacy)
	if err != nil || string(after) != string(before) {
		t.Fatalf("dry-run changed legacy file: %v %q", err, after)
	}
	got, err := svc.Store.GetVaultDoc(unclear.ID)
	if err != nil || got.Body != "legacy-body" || got.OwnerExplicit || got.TenantID != "minh-an-sales" {
		t.Fatalf("dry-run assigned or rewrote unclear: %v %#v", err, got)
	}
	if _, err := os.Stat(filepath.Join(root, "chinh-sach.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run moved a file: %v", err)
	}
}

func TestPut_ConcurrentTenants(t *testing.T) {
	root := t.TempDir()
	svc := New(store.New(), root)
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := svc.PutTenant("Chính sách chiết khấu Minh An", "sales-9-percent", "minh-an-sales")
		errCh <- err
	}()
	go func() {
		defer wg.Done()
		_, err := svc.PutTenant("Chính sách chiết khấu Minh An", "legal-8-percent", "minh-an-legal")
		errCh <- err
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	hits, err := svc.Search("minh-an-sales", "legal-8-percent")
	if err != nil || len(hits) != 0 {
		t.Fatalf("concurrent write crossed: %v %#v", err, hits)
	}
	hits, err = svc.Search("minh-an-legal", "legal-8-percent")
	if err != nil || len(hits) != 1 {
		t.Fatalf("legal missing after concurrent put: %v %#v", err, hits)
	}
}
