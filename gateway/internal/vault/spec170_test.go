// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// TestPutTenantAt_WritesTogetherAndRefusesRaw is criterion 4.
func TestPutTenantAt_WritesTogetherAndRefusesRaw(t *testing.T) {
	t.Setenv("GOSO_WORKSPACE", "")
	root := t.TempDir()
	st := store.New()
	svc := New(st, root)
	body := []byte("# Hop dong Minh An\n")
	doc, err := svc.PutTenantAt("minh-an", "sources/inbox/hop-dong.md", body)
	if err != nil || doc == nil || doc.Path != "sources/inbox/hop-dong.md" || !doc.OwnerExplicit {
		t.Fatalf("put %#v %v", doc, err)
	}
	sum := sha256.Sum256(body)
	if doc.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash %s", doc.SHA256)
	}
	abs, err := svc.namespaced("minh-an", doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(abs)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("file %q %v", got, err)
	}
	row, err := st.GetVaultDoc(doc.ID)
	if err != nil || row.SHA256 != doc.SHA256 || row.Body != string(body) {
		t.Fatalf("registry %#v %v", row, err)
	}

	if _, err := svc.PutTenantAt("minh-an", "raw/x.md", []byte("# no\n")); err == nil {
		t.Fatal("raw path must be refused")
	}
	if _, err := svc.PutTenantAt("minh-an", "sources/raw/x.md", []byte("# no\n")); err == nil {
		t.Fatal("nested raw path must be refused")
	}
	if _, err := os.Stat(filepath.Join(root, "t", "minh-an", "raw", "x.md")); !os.IsNotExist(err) {
		t.Fatalf("raw file appeared: %v", err)
	}
	if _, err := svc.PutTenantAt("minh-an", "../x.md", []byte("no")); err == nil {
		t.Fatal("escape must be refused")
	}
	if _, err := svc.PutTenantAt("minh-an", "sources/../../x.md", []byte("no")); err == nil {
		t.Fatal("nested escape must be refused")
	}

	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("SENTINEL"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, err := StorageDir(root, "minh-an")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sources"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "sources", "link.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PutTenantAt("minh-an", "sources/link.md", []byte("# Minh An\n")); err == nil {
		t.Fatal("symlink must be refused")
	}
	left, err := os.ReadFile(outside)
	if err != nil || string(left) != "SENTINEL" {
		t.Fatalf("symlink target changed: %q %v", left, err)
	}

	fail := failPut{StoreIface: store.New()}
	broken := New(fail, t.TempDir())
	if _, err := broken.PutTenantAt("minh-an", "wiki/topics/minh-an.md", []byte("# Minh An\n")); err == nil {
		t.Fatal("db error must fail")
	}
	matches, _ := filepath.Glob(filepath.Join(broken.Root, "t", "minh-an", "wiki", "topics", "*"))
	if len(matches) != 0 {
		t.Fatalf("db error published %v", matches)
	}
}

type failPut struct{ store.StoreIface }

func (f failPut) PutVaultDoc(store.VaultDoc) (*store.VaultDoc, error) {
	return nil, errors.New("db down")
}

// TestSyncInventory_RawDoesNotTouchSourcesOrWiki is criterion 5 for sync and inventory.
func TestSyncInventory_RawDoesNotTouchSourcesOrWiki(t *testing.T) {
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	t.Setenv("GOSO_WORKSPACE", "")
	for _, name := range []string{"memory", "sqlite", "postgres"} {
		t.Run(name, func(t *testing.T) {
			st := openNamedVaultStore(t, name)
			if st == nil {
				t.Skip("postgres DSN unset")
			}
			root := t.TempDir()
			svc := New(st, root)
			srcBody := []byte("# Hop dong Minh An\n")
			wikiBody := []byte("# Minh An\n")
			src, err := svc.PutTenantAt("minh-an", "sources/inbox/hop-dong.md", srcBody)
			if err != nil {
				t.Fatal(err)
			}
			wiki, err := svc.PutTenantAt("minh-an", "wiki/topics/minh-an.md", wikiBody)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := st.PutVaultDoc(store.VaultDoc{
				Title: "raw row", Path: "raw/old.md", Body: "old", SHA256: "old",
				TenantID: "minh-an", OwnerExplicit: true,
			}); err != nil {
				t.Fatal(err)
			}
			dir, err := StorageDir(root, "minh-an")
			if err != nil {
				t.Fatal(err)
			}
			rawDir := filepath.Join(dir, "raw")
			if err := os.MkdirAll(rawDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(rawDir, "x.md"), []byte("# raw Minh An\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(rawDir, "extra.txt"), []byte("extra Minh An"), 0o644); err != nil {
				t.Fatal(err)
			}
			res, err := svc.SyncTenant("minh-an")
			if err != nil {
				t.Fatal(err)
			}
			if res.Deleted != 0 {
				t.Fatalf("sync deleted %d", res.Deleted)
			}
			gotSrc, err := st.GetVaultDoc(src.ID)
			if err != nil || gotSrc.Body != string(srcBody) || gotSrc.Path != "sources/inbox/hop-dong.md" {
				t.Fatalf("sources row %v %#v", err, gotSrc)
			}
			gotWiki, err := st.GetVaultDoc(wiki.ID)
			if err != nil || gotWiki.Body != string(wikiBody) || gotWiki.Path != "wiki/topics/minh-an.md" {
				t.Fatalf("wiki row %v %#v", err, gotWiki)
			}
			if _, err := st.FindVaultDocByPath("minh-an", "raw/x.md"); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("raw/x.md created a vault_docs row: %v", err)
			}
			rawRow, err := st.FindVaultDocByPath("minh-an", "raw/old.md")
			if err != nil || rawRow.Body != "old" {
				t.Fatalf("raw registry row was deleted: %v %#v", err, rawRow)
			}
			os.Remove(filepath.Join(rawDir, "extra.txt"))
			res, err = svc.SyncTenant("minh-an")
			if err != nil || res.Deleted != 0 {
				t.Fatalf("missing raw file deleted rows: %v %#v", err, res)
			}
			if _, err := st.GetVaultDoc(src.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := st.GetVaultDoc(wiki.ID); err != nil {
				t.Fatal(err)
			}
			inv, err := svc.Inventory("minh-an")
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range inv.Entries {
				if e.Path == "raw/x.md" || e.Path == "raw/old.md" || strings.HasPrefix(e.Path, "raw/") || strings.Contains(e.Path, "/raw/") {
					t.Fatalf("inventory listed raw: %+v", e)
				}
			}
		})
	}
}
