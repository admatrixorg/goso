// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/contactsimport"
	"github.com/mqglobal/goso/gateway/internal/store"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestCLIDryRunWritesNothingThenApplyIsIdempotent(t *testing.T) {
	exportPath := filepath.Join("..", "..", "internal", "contactsimport", "testdata", "minh-an-contacts.json")
	dbPath := filepath.Join(t.TempDir(), "goso.db")
	args := []string{"--export", exportPath, "--tenant", "minh-an", "--db", dbPath}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr %s", stderr.String())
	}
	rep := decodeReport(t, stdout.Bytes())
	if !rep.DryRun || rep.New != contactsimport.FixtureCount || rep.Updated != 0 || rep.Skipped != 0 || rep.Invalid != 0 {
		t.Fatalf("dry-run %+v", rep)
	}
	assertReportCountsOnly(t, stdout.String())
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	n, err := st.CountContacts("minh-an")
	if err != nil || n != 0 {
		t.Fatalf("dry-run count %d %v", n, err)
	}
	_ = st.Close()

	stdout.Reset()
	stderr.Reset()
	if code := run(append(append([]string{}, args...), "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("apply exit %d %s", code, stderr.String())
	}
	rep = decodeReport(t, stdout.Bytes())
	if rep.DryRun || rep.New != contactsimport.FixtureCount || rep.Skipped != 0 {
		t.Fatalf("apply %+v", rep)
	}
	assertReportCountsOnly(t, stdout.String())
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	n, err = st.CountContacts("minh-an")
	if err != nil || n != contactsimport.FixtureCount {
		t.Fatalf("apply count %d %v", n, err)
	}
	other, err := st.CountContacts("tenant-bee")
	if err != nil || other != 0 {
		t.Fatalf("other count %d %v", other, err)
	}
	_ = st.Close()

	stdout.Reset()
	stderr.Reset()
	if code := run(append(append([]string{}, args...), "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("rerun exit %d %s", code, stderr.String())
	}
	rep = decodeReport(t, stdout.Bytes())
	if rep.New != 0 || rep.Updated != 0 || rep.Skipped != contactsimport.FixtureCount || rep.Invalid != 0 || rep.DryRun {
		t.Fatalf("rerun %+v", rep)
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n, err = st.CountContacts("minh-an")
	if err != nil || n != contactsimport.FixtureCount {
		t.Fatalf("rerun count %d %v", n, err)
	}
}

func TestCLITenantIsolation(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "export.json")
	body := `{
	  "contacts": [
	    {"tenant_id":"minh-an","channel":"telegram","dest":"ext-minh-an-cli","display":"Minh An CLI","kind":"user","agent_id":"minh-an-sales","seen_count":1,"first_seen":"2026-01-15T08:00:00Z","last_seen":"2026-01-15T09:00:00Z"},
	    {"tenant_id":"tenant-bee","channel":"telegram","dest":"ext-bee-cli","display":"Bee CLI","kind":"user","agent_id":"bee-agent","seen_count":4,"first_seen":"2026-01-15T08:00:00Z","last_seen":"2026-01-15T09:00:00Z"}
	  ]
	}`
	if err := os.WriteFile(exportPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "goso.db")
	args := []string{"--export", exportPath, "--tenant", "minh-an", "--db", dbPath, "--apply"}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("exit %d %s", code, stderr.String())
	}
	assertReportCountsOnly(t, stdout.String())
	rep := decodeReport(t, stdout.Bytes())
	if rep.New != 1 || rep.Invalid != 1 || rep.DryRun {
		t.Fatalf("%+v", rep)
	}
	if len(rep.Reasons) != 1 || rep.Reasons[0].Code != "foreign-tenant" || rep.Reasons[0].Count != 1 {
		t.Fatalf("reasons %+v", rep.Reasons)
	}
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if n, err := st.CountContacts("minh-an"); err != nil || n != 1 {
		t.Fatalf("minh-an %d %v", n, err)
	}
	if n, err := st.CountContacts("tenant-bee"); err != nil || n != 0 {
		t.Fatalf("bee %d %v", n, err)
	}
	rows, err := st.ListContacts("minh-an", "", "", "")
	if err != nil || len(rows) != 1 || rows[0].Dest != "ext-minh-an-cli" {
		t.Fatalf("rows %#v %v", rows, err)
	}
	if _, err := st.GetContact(rows[0].ID, "tenant-bee"); err == nil {
		t.Fatal("bee saw minh-an")
	}
}

func TestCLIRefusesWriteWithoutApply(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "export.json")
	if err := os.WriteFile(exportPath, []byte(`{"contacts":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "nested", "goso.db")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run([]string{"--export", exportPath, "--db", dbPath}, stdout, stderr); code != 2 {
		t.Fatalf("missing tenant exit %d %s", code, stderr.String())
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("db created without a tenant")
	}
	if code := run([]string{"--export", exportPath, "--tenant", "minh-an", "--db", dbPath, "--dry-run=false"}, stdout, stderr); code != 2 {
		t.Fatalf("dry-run=false exit %d", code)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("db created without --apply")
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--export", exportPath, "--tenant", "minh-an", "--db", dbPath, "--apply", "--dry-run"}, stdout, stderr); code != 2 {
		t.Fatalf("both flags exit %d %s", code, stderr.String())
	}
	if code := run([]string{"--export", exportPath, "--tenant", "minh-an", "--db", "postgres://goso@db.example/goso", "--apply"}, stdout, stderr); code != 1 {
		t.Fatalf("remote exit %d", code)
	}
	if !strings.Contains(stderr.String(), "refusing non-local database host") {
		t.Fatalf("stderr %s", stderr.String())
	}
	t.Setenv("GOSO_DATABASE_URL", "postgres://goso@127.0.0.1/goso")
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--export", exportPath, "--tenant", "minh-an", "--db", dbPath, "--apply"}, stdout, stderr); code != 1 {
		t.Fatalf("env dsn exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "refusing GOSO_DATABASE_URL") {
		t.Fatalf("stderr %s", stderr.String())
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("sqlite file created while GOSO_DATABASE_URL is postgres")
	}
}

func TestCLIPostgresApply(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if dsn == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	switch strings.ToLower(u.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
	default:
		t.Skip("CLI refuses a non-local database host")
	}
	const schema = "contactsimport172cli"
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`) })
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	exportPath := filepath.Join("..", "..", "internal", "contactsimport", "testdata", "minh-an-contacts.json")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	args := []string{"--export", exportPath, "--tenant", "minh-an", "--db", u.String()}
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	rep := decodeReport(t, stdout.Bytes())
	if !rep.DryRun || rep.New != contactsimport.FixtureCount {
		t.Fatalf("dry-run %+v", rep)
	}
	pg, err := store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	if n, err := pg.CountContacts("minh-an"); err != nil || n != 0 {
		t.Fatalf("dry-run count %d %v", n, err)
	}
	_ = pg.Close()
	stdout.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("apply exit %d %s", code, stderr.String())
	}
	rep = decodeReport(t, stdout.Bytes())
	if rep.New != contactsimport.FixtureCount || rep.DryRun {
		t.Fatalf("apply %+v", rep)
	}
	pg, err = store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	if n, err := pg.CountContacts("minh-an"); err != nil || n != contactsimport.FixtureCount {
		t.Fatalf("count %d %v", n, err)
	}
	stdout.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("rerun exit %d %s", code, stderr.String())
	}
	rep = decodeReport(t, stdout.Bytes())
	if rep.Skipped != contactsimport.FixtureCount || rep.New != 0 || rep.Updated != 0 {
		t.Fatalf("rerun %+v", rep)
	}
}

func decodeReport(t *testing.T, b []byte) contactsimport.Report {
	t.Helper()
	var rep contactsimport.Report
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatal(err)
	}
	return rep
}

func assertReportCountsOnly(t *testing.T, s string) {
	t.Helper()
	for _, m := range []string{"Minh An", "ext-", "grp-", "telegram", "zalo", "minh-an", "Bee", "DROPME"} {
		if strings.Contains(s, m) {
			t.Fatalf("report contains %q", m)
		}
	}
}
