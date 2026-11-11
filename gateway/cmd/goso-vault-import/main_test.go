// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCLIDryRunWritesNothingThenImportIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "export.json")
	body := `{
	  "vault_documents": [
	    {"id":"a","tenant_id":"11111111-1111-4111-8111-111111111111","path":"tenants/minh-an/notes/a.md","title":"A","updated_at":"2026-03-01T04:05:06Z"},
	    {"id":"c","tenant_id":"11111111-1111-4111-8111-111111111111","path":"tenants/mq-capital/secret.md","title":"Nope","embedding":"EMBEDMARKERvector","updated_at":"2026-03-01T04:05:06Z"}
	  ],
	  "vault_versions": [
	    {"doc_id":"a","version":1,"content":"BODYMARKERminhan","created_at":"2026-03-01T04:05:06Z"}
	  ],
	  "vault_links": []
	}`
	if err := os.WriteFile(exportPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "goso.db")
	vaultDir := filepath.Join(dir, "vault")
	args := []string{
		"--export", exportPath,
		"--source-tenant", "11111111-1111-4111-8111-111111111111",
		"--root", "tenants/minh-an",
		"--dest-tenant", "minh-an",
		"--db", dbPath,
		"--vault-dir", vaultDir,
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(append(args, "--dry-run"), stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"inserted": 1`) || !strings.Contains(stdout.String(), `"rejected": 1`) {
		t.Fatalf("stdout %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "BODYMARKERminhan") || strings.Contains(stderr.String(), "BODYMARKERminhan") || strings.Contains(stderr.String(), "EMBEDMARKERvector") {
		t.Fatal("cli leaked a body or embedding")
	}
	if !strings.Contains(stderr.String(), "tenants/mq-capital/secret.md") {
		t.Fatalf("stderr %s", stderr.String())
	}
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.FindVaultDocByPath("minh-an", "notes/a.md"); err == nil {
		t.Fatal("dry-run wrote a document")
	}
	_ = st.Close()
	if _, err := os.Stat(filepath.Join(vaultDir, "t")); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote a file")
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("import exit %d %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("second exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"inserted": 0`) || !strings.Contains(stdout.String(), `"updated": 0`) || !strings.Contains(stdout.String(), `"skipped": 1`) {
		t.Fatalf("second stdout %s", stdout.String())
	}
}

func TestCLIRefusesRemoteDatabase(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "export.json")
	if err := os.WriteFile(exportPath, []byte(`{"vault_documents":[],"vault_links":[],"vault_versions":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{
		"--export", exportPath,
		"--source-tenant", "minh-an",
		"--root", "tenants/minh-an",
		"--dest-tenant", "minh-an",
		"--db", "postgres://importer:super-secret-dsn-pass@example.com:5432/goso?sslmode=disable",
		"--dry-run",
	}, stdout, stderr)
	if code == 0 {
		t.Fatal("remote database was accepted")
	}
	if strings.Contains(stderr.String(), "super-secret-dsn-pass") || strings.Contains(stdout.String(), "super-secret-dsn-pass") {
		t.Fatal("dsn secret leaked")
	}
	if !strings.Contains(stderr.String(), "non-local") {
		t.Fatalf("stderr %s", stderr.String())
	}
}
