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
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestB1_UnclearOwnerDeniedBodyAndMetadata(t *testing.T) {
	stores := map[string]store.StoreIface{}
	sq, err := store.OpenSQLite(filepath.Join(t.TempDir(), "b1.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	stores["sqlite"] = sq
	stores["memory"] = store.New()
	if pg := openVaultPostgres(t); pg != nil {
		stores["postgres"] = pg
	}
	for name, st := range stores {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
			svc := New(st, root)
			unclear, err := st.PutVaultDoc(store.VaultDoc{
				Title: "Chính sách cũ Minh An", Path: "chinh-sach.md", TenantID: "minh-an-sales",
				Body: "LEGACY-ROW-8841", SHA256: "old", OwnerExplicit: false,
			})
			if err != nil {
				t.Fatal(err)
			}
			explicit, err := svc.PutTenant("Đã rõ", "explicit-body", "minh-an-sales")
			if err != nil {
				t.Fatal(err)
			}
			if err := st.ReplaceVaultLinks(explicit.ID, []string{"Chính sách cũ Minh An"}); err != nil {
				t.Fatal(err)
			}
			if err := st.ReplaceVaultLinks(unclear.ID, []string{"Đã rõ"}); err != nil {
				t.Fatal(err)
			}
			if err := st.ReResolveVaultLinks(); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.Get("minh-an-sales", unclear.ID); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("get served legacy row: %v", err)
			}
			for _, d := range svc.List() {
				if d.ID == unclear.ID || strings.Contains(d.Title, "cũ") || strings.Contains(d.Body, "LEGACY-ROW-8841") || d.Path == unclear.Path && d.ID == unclear.ID {
					t.Fatalf("list leaked metadata %#v", d)
				}
			}
			hits, err := svc.Search("minh-an-sales", "LEGACY-ROW-8841")
			if err != nil || len(hits) != 0 {
				t.Fatalf("search leaked %v %#v", err, hits)
			}
			ob, ib, err := svc.Links(explicit.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, l := range ob {
				if l.ToID == unclear.ID {
					t.Fatalf("outbound id leaked %#v", ob)
				}
			}
			for _, l := range ib {
				if l.FromID == unclear.ID {
					t.Fatalf("inbound id leaked %#v", ib)
				}
			}
			g := svc.Graph(40, "minh-an-sales")
			if len(g.Nodes) != 1 || g.Nodes[0].ID != explicit.ID {
				t.Fatalf("graph count %#v", g.Nodes)
			}
			h, err := svc.Health("minh-an-sales")
			if err != nil || h.Docs != 1 {
				t.Fatalf("health %#v %v", h, err)
			}
			before, err := st.GetVaultDoc(unclear.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.PutTenant("Chính sách cũ Minh An", "replaced-body", "minh-an-sales"); err != nil {
				t.Fatal(err)
			}
			after, err := st.GetVaultDoc(unclear.ID)
			if err != nil || after.Body != before.Body || after.OwnerExplicit || after.Title != before.Title {
				t.Fatalf("put upgraded legacy %#v", after)
			}
			dir, err := StorageDir(root, "minh-an-sales")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, unclear.Path), []byte("# Chính sách cũ Minh An\nchanged-on-disk\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.SyncTenant("minh-an-sales"); err != nil {
				t.Fatal(err)
			}
			after, err = st.GetVaultDoc(unclear.ID)
			if err != nil || after.Body != "LEGACY-ROW-8841" || after.OwnerExplicit {
				t.Fatalf("sync upgraded legacy %#v %v", after, err)
			}
		})
	}
}

func TestB4_TwoServicesSameTenantDoNotDropPut(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	st := store.New()
	scan := New(st, root)
	writer := New(st, root)
	if _, err := scan.PutTenant("Giữ", "stay", "minh-an-sales"); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	SetTestBarriers(func() { close(entered); <-release }, nil)
	t.Cleanup(func() { SetTestBarriers(nil, nil) })
	syncErr := make(chan error, 1)
	go func() {
		_, err := scan.SyncTenant("minh-an-sales")
		syncErr <- err
	}()
	<-entered
	putDone := make(chan error, 1)
	go func() {
		_, err := writer.PutTenant("Mới trong lúc sync", "fresh-body", "minh-an-sales")
		putDone <- err
	}()
	select {
	case err := <-putDone:
		t.Fatalf("put finished during scan barrier: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	close(release)
	if err := <-syncErr; err != nil {
		t.Fatal(err)
	}
	if err := <-putDone; err != nil {
		t.Fatal(err)
	}
	hits, err := writer.Search("minh-an-sales", "fresh-body")
	if err != nil || len(hits) != 1 {
		t.Fatalf("put lost across services %v %#v", err, hits)
	}
}

func TestB4_DBFailureDoesNotPublishFile(t *testing.T) {
	root := t.TempDir()
	inner := store.New()
	svc := New(inner, root)
	doc, err := svc.PutTenant("Giữ", "original-bytes", "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	abs, err := svc.namespaced("minh-an-sales", doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	broken := New(putFail{inner}, root)
	if _, err := broken.PutTenant("Giữ", "changed-bytes", "minh-an-sales"); err == nil {
		t.Fatal("db failure reported success")
	}
	after, err := os.ReadFile(abs)
	if err != nil || string(after) != string(before) {
		t.Fatalf("file published despite db error %q", after)
	}
	raw, err := inner.GetVaultDoc(doc.ID)
	if err != nil || raw.Body != "original-bytes" {
		t.Fatalf("row changed %#v %v", raw, err)
	}
}

func TestB5_NamespaceSymlinkRejected(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	legal, err := StorageDir(root, "minh-an-legal")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legal, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legal, "bi-mat.md"), []byte("LEGAL-ONLY-8841"), 0o644); err != nil {
		t.Fatal(err)
	}
	sales, err := StorageDir(root, "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(legal, sales); err != nil {
		t.Fatal(err)
	}
	svc := New(store.New(), root)
	if _, err := svc.PutTenant("Bán", "sales-body", "minh-an-sales"); err == nil {
		t.Fatal("put followed namespace symlink")
	}
	entries, _ := os.ReadDir(legal)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(legal, e.Name()))
		if strings.Contains(string(b), "sales-body") {
			t.Fatal("put wrote into the other namespace")
		}
	}
	if _, err := svc.SyncTenant("minh-an-sales"); err == nil {
		t.Fatal("sync followed namespace symlink")
	}
	for _, d := range svc.Store.ListVaultDocs() {
		if strings.Contains(d.Body, "LEGAL-ONLY-8841") {
			t.Fatalf("sync imported other namespace %#v", d)
		}
	}
	if _, err := svc.Health("minh-an-sales"); err == nil {
		t.Fatal("health followed namespace symlink")
	}
	if _, err := svc.Inventory("minh-an-sales"); err == nil {
		t.Fatal("inventory followed namespace symlink")
	}
}

func TestB5_ParentAndNestedSymlink(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("OUTSIDE-SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(root, "t")
	if err := os.Symlink(outside, parent); err != nil {
		t.Fatal(err)
	}
	svc := New(store.New(), root)
	if _, err := svc.PutTenant("X", "nope", "minh-an-sales"); err == nil {
		t.Fatal("put followed parent symlink")
	}

	root2 := t.TempDir()
	svc2 := New(store.New(), root2)
	keep, err := svc2.PutTenant("Giữ", "keep-body", "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := StorageDir(root2, "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "nhom")
	if err := os.Symlink(outside, nested); err != nil {
		t.Fatal(err)
	}
	if _, err := svc2.SyncTenant("minh-an-sales"); err != nil {
		t.Fatal(err)
	}
	for _, d := range svc2.Store.ListVaultDocs() {
		if strings.Contains(d.Body, "OUTSIDE-SECRET") {
			t.Fatalf("nested symlink imported %#v", d)
		}
	}
	if _, err := svc2.Store.GetVaultDoc(keep.ID); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyScan_DoesNotPresentCachedBody(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	st := store.New()
	svc := New(st, root)
	doc, err := svc.PutTenant("Một", "CACHED-BODY-7712", "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	abs, err := svc.namespaced("minh-an-sales", doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(abs); err != nil {
		t.Fatal(err)
	}
	res, err := svc.SyncTenant("minh-an-sales")
	if err != nil || res.Deleted != 0 {
		t.Fatalf("empty scan deleted %v %#v", err, res)
	}
	got, err := svc.Get("minh-an-sales", doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Body, "CACHED-BODY-7712") || got.BodyStatus != "unconfirmed" {
		t.Fatalf("cached body presented as current %#v", got)
	}
	raw, err := st.GetVaultDoc(doc.ID)
	if err != nil || raw.Body != "CACHED-BODY-7712" {
		t.Fatalf("row mutated %#v %v", raw, err)
	}
	hits, err := svc.Search("minh-an-sales", "CACHED-BODY-7712")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if strings.Contains(h.Snippet, "CACHED-BODY-7712") {
			t.Fatalf("search snippet presented cache %#v", h)
		}
	}
}

func TestPartialSkip_DoesNotAuthoritativelyDelete(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	st := store.New()
	svc := New(st, root)
	keep, err := svc.PutTenant("Giữ", "keep-body", "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	odd, err := svc.PutTenant("Bỏ qua", "odd-body", "minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	oddAbs, err := svc.namespaced("minh-an-sales", odd.Path)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	ext := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(ext, []byte("OUTSIDE-SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(oddAbs); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, oddAbs); err != nil {
		t.Fatal(err)
	}
	res, err := svc.SyncTenant("minh-an-sales")
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 0 {
		t.Fatalf("skip caused delete %#v", res)
	}
	if _, err := st.GetVaultDoc(keep.ID); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetVaultDoc(odd.ID)
	if err != nil || got.Body != "odd-body" {
		t.Fatalf("skipped path deleted or rewritten %#v %v", got, err)
	}
	for _, d := range st.ListVaultDocs() {
		if strings.Contains(d.Body, "OUTSIDE-SECRET") {
			t.Fatalf("symlink body imported %#v", d)
		}
	}
}

func openNamedVaultStore(t *testing.T, name string) store.StoreIface {
	t.Helper()
	switch name {
	case "memory":
		return store.New()
	case "sqlite":
		sq, err := store.OpenSQLite(filepath.Join(t.TempDir(), "vault.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sq.Close() })
		return sq
	case "postgres":
		return openVaultPostgres(t)
	default:
		t.Fatalf("unknown store %s", name)
		return nil
	}
}

func TestB4_AfterCommitReadersFailClosed(t *testing.T) {
	for _, name := range []string{"memory", "sqlite", "postgres"} {
		t.Run(name, func(t *testing.T) {
			st := openNamedVaultStore(t, name)
			if st == nil {
				t.Skip("postgres DSN unset")
			}
			root := t.TempDir()
			svc := New(st, root)
			first, err := svc.PutTenant("Giữ", "original-body", "minh-an-sales")
			if err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{})
			release := make(chan struct{})
			done := make(chan error, 1)
			var once sync.Once
			SetPublishHooks(nil, nil, func() {
				once.Do(func() { close(entered) })
				<-release
			})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
				SetPublishHooks(nil, nil, nil)
			})
			go func() {
				_, err := svc.PutTenant("Giữ", "NEW-BODY-8841", "minh-an-sales")
				done <- err
			}()
			select {
			case <-entered:
			case err := <-done:
				t.Fatalf("put finished before the post-commit barrier: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("post-commit barrier was not reached")
			}
			got, err := svc.Get("minh-an-sales", first.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.BodyStatus != "unconfirmed" || got.Body != "" || got.SHA256 != "" || strings.Contains(got.Body, "NEW-BODY-8841") || strings.Contains(got.Body, "original-body") {
				t.Fatalf("get returned a body during publish %#v", got)
			}
			hits, err := svc.Search("minh-an-sales", "NEW-BODY-8841")
			if err != nil {
				t.Fatal(err)
			}
			if len(hits) == 0 {
				t.Fatal("search missed the committed row")
			}
			for _, h := range hits {
				if h.BodyStatus != "unconfirmed" || h.Snippet != "" || strings.Contains(h.Snippet, "NEW-BODY-8841") {
					t.Fatalf("search returned a snippet during publish %#v", h)
				}
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			got, err = svc.Get("minh-an-sales", first.ID)
			if err != nil || got.BodyStatus != "current" || !strings.Contains(got.Body, "NEW-BODY-8841") || got.SHA256 == "" {
				t.Fatalf("after rename %#v %v", got, err)
			}
		})
	}
}

func TestB4_RenameFailureUnconfirmed(t *testing.T) {
	for _, name := range []string{"memory", "sqlite", "postgres"} {
		t.Run(name, func(t *testing.T) {
			st := openNamedVaultStore(t, name)
			if st == nil {
				t.Skip("postgres DSN unset")
			}
			root := t.TempDir()
			svc := New(st, root)
			first, err := svc.PutTenant("Giữ", "original-body", "minh-an-sales")
			if err != nil {
				t.Fatal(err)
			}
			abs, err := svc.namespaced("minh-an-sales", first.Path)
			if err != nil {
				t.Fatal(err)
			}
			SetPublishHooks(func(string, string) error { return errors.New("rename failed") }, nil, nil)
			t.Cleanup(func() { SetPublishHooks(nil, nil, nil) })
			if _, err := svc.PutTenant("Giữ", "NEW-RENAME-8841", "minh-an-sales"); err == nil {
				t.Fatal("rename failure reported success")
			}
			disk, err := os.ReadFile(abs)
			if err != nil || string(disk) != "original-body" {
				t.Fatalf("file bytes %q %v", disk, err)
			}
			got, err := svc.Get("minh-an-sales", first.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.BodyStatus != "unconfirmed" || got.Body != "" || got.SHA256 != "" || strings.Contains(got.Body, "NEW-RENAME-8841") || strings.Contains(got.Body, "original-body") {
				t.Fatalf("rename failure still current %#v", got)
			}
			hits, err := svc.Search("minh-an-sales", "NEW-RENAME-8841")
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range hits {
				if h.BodyStatus != "unconfirmed" || h.Snippet != "" {
					t.Fatalf("search snippet during rename failure %#v", h)
				}
			}
		})
	}
}

func TestB4_ReadFileErrorFailClosed(t *testing.T) {
	for _, name := range []string{"memory", "sqlite", "postgres"} {
		t.Run(name, func(t *testing.T) {
			st := openNamedVaultStore(t, name)
			if st == nil {
				t.Skip("postgres DSN unset")
			}
			svc := New(st, t.TempDir())
			doc, err := svc.PutTenant("Giữ", "cached-body-8841", "minh-an-sales")
			if err != nil {
				t.Fatal(err)
			}
			SetPublishHooks(nil, func(string) ([]byte, error) { return nil, errors.New("read failed") }, nil)
			t.Cleanup(func() { SetPublishHooks(nil, nil, nil) })
			got, err := svc.Get("minh-an-sales", doc.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.BodyStatus != "unconfirmed" || got.Body != "" || got.SHA256 != "" || strings.Contains(got.Body, "cached-body-8841") {
				t.Fatalf("read error kept cached body %#v", got)
			}
			hits, err := svc.Search("minh-an-sales", "cached-body-8841")
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range hits {
				if h.BodyStatus != "unconfirmed" || h.Snippet != "" || strings.Contains(h.Snippet, "cached-body-8841") {
					t.Fatalf("search kept snippet on read error %#v", h)
				}
			}
		})
	}
}

func TestB4_RestartAcrossPersistentStore(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "vault.db")
		root := t.TempDir()
		sq, err := store.OpenSQLite(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		svc := New(sq, root)
		first, err := svc.PutTenant("Giữ", "original-body", "minh-an-sales")
		if err != nil {
			t.Fatal(err)
		}
		SetPublishHooks(func(string, string) error { return errors.New("rename failed") }, nil, nil)
		if _, err := svc.PutTenant("Giữ", "NEW-RESTART-8841", "minh-an-sales"); err == nil {
			t.Fatal("rename failure reported success")
		}
		if err := sq.Close(); err != nil {
			t.Fatal(err)
		}
		SetPublishHooks(nil, nil, nil)
		sq2, err := store.OpenSQLite(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sq2.Close() })
		got, err := New(sq2, root).Get("minh-an-sales", first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.BodyStatus != "unconfirmed" || got.Body != "" || got.SHA256 != "" || strings.Contains(got.Body, "NEW-RESTART-8841") || strings.Contains(got.Body, "original-body") {
			t.Fatalf("restart presented a body %#v", got)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		root := t.TempDir()
		st := openPostgresSchema(t, "vault_b4_restart", true)
		if st == nil {
			t.Skip("postgres DSN unset")
		}
		svc := New(st, root)
		first, err := svc.PutTenant("Giữ", "original-body", "minh-an-sales")
		if err != nil {
			t.Fatal(err)
		}
		SetPublishHooks(func(string, string) error { return errors.New("rename failed") }, nil, nil)
		if _, err := svc.PutTenant("Giữ", "NEW-RESTART-8841", "minh-an-sales"); err == nil {
			t.Fatal("rename failure reported success")
		}
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
		SetPublishHooks(nil, nil, nil)
		st2 := openPostgresSchema(t, "vault_b4_restart", false)
		if st2 == nil {
			t.Fatal("postgres reopen")
		}
		got, err := New(st2, root).Get("minh-an-sales", first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.BodyStatus != "unconfirmed" || got.Body != "" || got.SHA256 != "" || strings.Contains(got.Body, "NEW-RESTART-8841") || strings.Contains(got.Body, "original-body") {
			t.Fatalf("restart presented a body %#v", got)
		}
	})
}

func openPostgresSchema(t *testing.T, schema string, reset bool) *store.PostgresStore {
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
	if reset {
		if _, err := admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
			t.Fatal(err)
		}
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

func TestB5a_PartialSymlinkLeavesOtherTenantUnchanged(t *testing.T) {
	const legalBytes = "LEGAL-BYTES-8841"
	for _, name := range []string{"memory", "sqlite", "postgres"} {
		t.Run(name, func(t *testing.T) {
			inner := openNamedVaultStore(t, name)
			if inner == nil {
				t.Skip("postgres DSN unset")
			}
			for _, label := range []string{"db-ok", "db-fail"} {
				t.Run(label, func(t *testing.T) {
					var st store.StoreIface = inner
					if label == "db-fail" {
						st = putFail{inner}
					}
					root := t.TempDir()
					legalDir, err := StorageDir(root, "minh-an-legal")
					if err != nil {
						t.Fatal(err)
					}
					salesDir, err := StorageDir(root, "minh-an-sales")
					if err != nil {
						t.Fatal(err)
					}
					if err := os.MkdirAll(legalDir, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.MkdirAll(salesDir, 0o755); err != nil {
						t.Fatal(err)
					}
					target := filepath.Join(legalDir, "bi-mat.md")
					if err := os.WriteFile(target, []byte(legalBytes), 0o644); err != nil {
						t.Fatal(err)
					}
					partial := filepath.Join(salesDir, "bien-ban.md.partial")
					if err := os.Symlink(target, partial); err != nil {
						t.Fatal(err)
					}
					svc := New(st, root)
					if _, err := svc.PutTenant("Bien ban", "SALES-OVERWRITE-8841", "minh-an-sales"); err == nil {
						t.Fatal("put followed an existing partial symlink")
					}
					got, err := os.ReadFile(target)
					if err != nil || string(got) != legalBytes {
						t.Fatalf("other tenant bytes changed %q %v", got, err)
					}
					info, err := os.Lstat(partial)
					if err != nil || info.Mode()&os.ModeSymlink == 0 {
						t.Fatalf("partial symlink replaced %v", err)
					}
				})
			}
		})
	}
}

func TestB5b_DirSymlinkDoesNotDeleteChild(t *testing.T) {
	for _, name := range []string{"memory", "sqlite", "postgres"} {
		t.Run(name, func(t *testing.T) {
			st := openNamedVaultStore(t, name)
			if st == nil {
				t.Skip("postgres DSN unset")
			}
			root := t.TempDir()
			t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
			svc := New(st, root)
			dir, err := StorageDir(root, "minh-an-sales")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(dir, "nhom"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "keep.md"), []byte("# Giữ\nkeep-root\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "nhom", "note.md"), []byte("# Ghi chú\nchild-body\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.SyncTenant("minh-an-sales"); err != nil {
				t.Fatal(err)
			}
			child, err := st.FindVaultDocByPath("minh-an-sales", "nhom/note.md")
			if err != nil || child == nil || !child.OwnerExplicit {
				t.Fatalf("child missing after first sync %#v %v", child, err)
			}
			if err := os.RemoveAll(filepath.Join(dir, "nhom")); err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "note.md"), []byte("# Khác\nforeign-body\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(dir, "nhom")); err != nil {
				t.Fatal(err)
			}
			res, err := svc.SyncTenant("minh-an-sales")
			if err != nil {
				t.Fatal(err)
			}
			again, err := st.FindVaultDocByPath("minh-an-sales", "nhom/note.md")
			if err != nil || again == nil {
				t.Fatalf("child deleted when folder became a symlink %#v %v", res, err)
			}
			if again.Body != "child-body" && !strings.Contains(again.Body, "child-body") {
				t.Fatalf("child row rewritten %#v", again)
			}
			got, err := svc.Get("minh-an-sales", again.ID)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got.Body, "foreign-body") || got.BodyStatus == "current" {
				t.Fatalf("symlink dir presented as current %#v", got)
			}
		})
	}
}

func TestB4_SearchDoesNotPairOldSnippetWithNewVersion(t *testing.T) {
	for _, name := range []string{"memory", "sqlite", "postgres"} {
		t.Run(name, func(t *testing.T) {
			st := openNamedVaultStore(t, name)
			if st == nil {
				t.Skip("postgres DSN unset")
			}
			svc := New(st, t.TempDir())
			if _, err := svc.PutTenant("Giữ", "V1-ONLY-8841 shared", "minh-an-sales"); err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			SetSearchBarrier(func() {
				once.Do(func() { close(entered) })
				<-release
			})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
				SetSearchBarrier(nil)
			})
			type searchResult struct {
				hits []store.VaultSearchHit
				err  error
			}
			done := make(chan searchResult, 1)
			go func() {
				hits, err := svc.Search("minh-an-sales", "V1-ONLY-8841")
				done <- searchResult{hits, err}
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("search did not pause after the query")
			}
			if _, err := svc.PutTenant("Giữ", "V2-ONLY-8841 shared", "minh-an-sales"); err != nil {
				t.Fatal(err)
			}
			close(release)
			res := <-done
			if res.err != nil {
				t.Fatal(res.err)
			}
			for _, h := range res.hits {
				if h.BodyStatus == "current" && strings.Contains(h.Snippet, "V1-ONLY-8841") {
					t.Fatalf("snippet v1 paired with current v2 %#v", h)
				}
			}
			if len(res.hits) != 1 || res.hits[0].BodyStatus != "unconfirmed" || res.hits[0].Snippet != "" || strings.Contains(res.hits[0].Snippet, "V1-ONLY-8841") {
				t.Fatalf("want the v1 hit kept unconfirmed with an empty snippet %#v", res.hits)
			}
		})
	}
}
