// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"errors"
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
				Body: "LEGACY-SECRET-8841", SHA256: "old", OwnerExplicit: false,
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
				if d.ID == unclear.ID || strings.Contains(d.Title, "cũ") || strings.Contains(d.Body, "LEGACY-SECRET-8841") || d.Path == unclear.Path && d.ID == unclear.ID {
					t.Fatalf("list leaked metadata %#v", d)
				}
			}
			hits, err := svc.Search("minh-an-sales", "LEGACY-SECRET-8841")
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
			if err != nil || after.Body != "LEGACY-SECRET-8841" || after.OwnerExplicit {
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

var _ = sync.Mutex{}
