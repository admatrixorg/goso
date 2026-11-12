// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vaultimport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// TestImportSkipsRaw is criterion 5 for the SPEC 152 importer.
func TestImportSkipsRaw(t *testing.T) {
	src := t.TempDir()
	rawPath := filepath.Join(src, "tenants", "minh-an", "raw", "x.md")
	notePath := filepath.Join(src, "tenants", "minh-an", "notes", "hop-dong.md")
	if err := os.MkdirAll(filepath.Dir(rawPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rawPath, []byte("RAWBODYMARKER Minh An"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, []byte("NOTEBODYMARKER Minh An"), 0o644); err != nil {
		t.Fatal(err)
	}
	ex := &Export{
		Documents: []Document{
			{ID: "doc-raw", TenantID: sourceTenant, Scope: "team", Path: "tenants/minh-an/raw/x.md", Title: "Raw Minh An", UpdatedAt: "2026-03-01T04:05:06Z"},
			{ID: "doc-note", TenantID: sourceTenant, Scope: "team", Path: "tenants/minh-an/notes/hop-dong.md", Title: "Hop dong Minh An", UpdatedAt: "2026-03-01T04:05:06Z"},
		},
		Versions: []Version{
			{DocID: "doc-raw", Version: 1, Content: "VERSIONRAWMARKER", CreatedAt: "2026-03-01T04:05:06Z"},
		},
	}
	vaultRoot := t.TempDir()
	st := store.New()
	rep, err := Import(st, ex, Options{
		SourceTenant: sourceTenant,
		Root:         tenantRoot,
		DestTenant:   destTenant,
		VaultRoot:    vaultRoot,
		SrcDir:       src,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Inserted != 1 || rep.Rejected != 1 || !hasReason(rep, "doc", "tenants/minh-an/raw/x.md", "raw-skipped") {
		t.Fatalf("report %+v", rep)
	}
	note, err := st.FindVaultDocByPath(destTenant, "notes/hop-dong.md")
	if err != nil || !strings.Contains(note.Body, "NOTEBODYMARKER Minh An") {
		t.Fatalf("note %#v %v", note, err)
	}
	if strings.Contains(note.Body, "RAWBODYMARKER") || strings.Contains(note.Body, "VERSIONRAWMARKER") {
		t.Fatalf("raw body leaked into note: %s", note.Body)
	}
	for _, doc := range st.ListVaultDocs() {
		if doc != nil && (strings.HasPrefix(doc.Path, "raw/") || strings.Contains(doc.Path, "/raw/")) {
			t.Fatalf("raw row %#v", doc)
		}
		if doc != nil && (strings.Contains(doc.Body, "RAWBODYMARKER") || strings.Contains(doc.Body, "VERSIONRAWMARKER")) {
			t.Fatalf("raw body stored %#v", doc)
		}
	}
	rawFile := filepath.Join(vaultRoot, "t", destTenant, "raw", "x.md")
	if _, err := os.Stat(rawFile); !os.IsNotExist(err) {
		t.Fatalf("importer wrote raw: %v", err)
	}
}
