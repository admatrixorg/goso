// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"path/filepath"
	"testing"
)

func TestReResolveVaultLinks(t *testing.T) {
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "goso.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	put := func(tenant, title, path string) *VaultDoc {
		t.Helper()
		d, err := s.PutVaultDoc(VaultDoc{TenantID: tenant, Title: title, Path: path, SHA256: path, Body: "x", OwnerExplicit: true})
		if err != nil {
			t.Fatalf("put %s: %v", path, err)
		}
		return d
	}
	from := put("minh-an", "Ghi chú", "notes/ghi-chu.md")
	later := put("minh-an", "Kế Hoạch", "z/ke-hoach.md")
	first := put("minh-an", "kế hoạch", "a/ke-hoach.md")
	other := put("demo", "Báo cáo", "bao-cao.md")
	if err := s.ReplaceVaultLinks(from.ID, []string{"KẾ HOẠCH", "Báo cáo", later.ID, "Không có"}); err != nil {
		t.Fatalf("links: %v", err)
	}
	if err := s.ReResolveVaultLinks(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got := map[string]string{}
	out, _, err := s.ListVaultDocLinks(from.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, l := range out {
		got[vaultInner(l.Raw)] = l.ToID
	}
	want := map[string]string{
		"KẾ HOẠCH": first.ID, // case-insensitive title, first by path
		"Báo cáo":  "",       // a title in another tenant never resolves
		later.ID:   later.ID, // a doc id in the same tenant
		"Không có": "",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("link %q -> %q, want %q (all %v; other %s)", k, got[k], v, got, other.ID)
		}
	}
	if err := s.DeleteVaultDoc(first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.ReResolveVaultLinks(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	out, _, _ = s.ListVaultDocLinks(from.ID)
	for _, l := range out {
		if vaultInner(l.Raw) == "KẾ HOẠCH" && l.ToID != later.ID {
			t.Fatalf("after delete -> %q, want %q", l.ToID, later.ID)
		}
	}
}
