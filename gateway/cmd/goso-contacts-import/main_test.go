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
	args := importArgs(t, exportPath, dbPath)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr %s", stderr.String())
	}
	rep := decodeReport(t, stdout.Bytes())
	if !rep.DryRun || rep.New != contactsimport.FixtureLive || rep.Updated != 0 || rep.Skipped != contactsimport.FixtureMerged || rep.Invalid != 0 {
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
	if rep.DryRun || rep.New != contactsimport.FixtureLive || rep.Skipped != contactsimport.FixtureMerged {
		t.Fatalf("apply %+v", rep)
	}
	assertReportCountsOnly(t, stdout.String())
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	n, err = st.CountContacts("minh-an")
	if err != nil || n != contactsimport.FixtureStoredCount() {
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
	if rep.New != 0 || rep.Updated != 0 || rep.Skipped != contactsimport.FixtureRows || rep.Invalid != 0 || rep.DryRun {
		t.Fatalf("rerun %+v", rep)
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n, err = st.CountContacts("minh-an")
	if err != nil || n != contactsimport.FixtureStoredCount() {
		t.Fatalf("rerun count %d %v", n, err)
	}
}

func TestCLITenantIsolation(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "export.json")
	body := `{
	  "channel_contacts": [
	    {"tenant_id":"minh-an","channel_type":"telegram","sender_id":"ext-minh-an-cli","display_name":"Minh An CLI","contact_type":"user","peer_kind":"user","first_seen_at":"2026-01-15T08:00:00Z","last_seen_at":"2026-01-15T09:00:00Z"},
	    {"tenant_id":"tenant-bee","channel_type":"telegram","sender_id":"ext-bee-cli","display_name":"Bee CLI","contact_type":"user","peer_kind":"user","first_seen_at":"2026-01-15T08:00:00Z","last_seen_at":"2026-01-15T09:00:00Z"}
	  ]
	}`
	if err := os.WriteFile(exportPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "goso.db")
	args := append(importArgs(t, exportPath, dbPath), "--apply")
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
	if err != nil || len(rows) != 1 || rows[0].Dest != "ext-minh-an-cli" || rows[0].Channel != "telegram" {
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
	if code := run(append(importArgs(t, exportPath, "postgres://goso@db.example/goso"), "--apply"), stdout, stderr); code != 1 {
		t.Fatalf("remote exit %d", code)
	}
	if !strings.Contains(stderr.String(), "refusing non-local database host") {
		t.Fatalf("stderr %s", stderr.String())
	}
	t.Setenv("GOSO_DATABASE_URL", "postgres://goso@127.0.0.1/goso")
	stdout.Reset()
	stderr.Reset()
	if code := run(append(importArgs(t, exportPath, dbPath), "--apply"), stdout, stderr); code != 1 {
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
	args := importArgs(t, exportPath, u.String())
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	rep := decodeReport(t, stdout.Bytes())
	if !rep.DryRun || rep.New != contactsimport.FixtureLive || rep.Skipped != contactsimport.FixtureMerged {
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
	if rep.New != contactsimport.FixtureLive || rep.Skipped != contactsimport.FixtureMerged || rep.DryRun {
		t.Fatalf("apply %+v", rep)
	}
	pg, err = store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	if n, err := pg.CountContacts("minh-an"); err != nil || n != contactsimport.FixtureStoredCount() {
		t.Fatalf("count %d %v", n, err)
	}
	stdout.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("rerun exit %d %s", code, stderr.String())
	}
	rep = decodeReport(t, stdout.Bytes())
	if rep.Skipped != contactsimport.FixtureRows || rep.New != 0 || rep.Updated != 0 {
		t.Fatalf("rerun %+v", rep)
	}
}

func importArgs(t *testing.T, exportPath, dbPath string) []string {
	t.Helper()
	t.Setenv("GOSO_CONTACTS_DENY_FILE", "")
	deny := filepath.Join(t.TempDir(), "deny.txt")
	if err := os.WriteFile(deny, []byte("blocked-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return []string{"--export", exportPath, "--tenant", "minh-an", "--source-tenant", "minh-an", "--db", dbPath, "--deny-file", deny, "--customer-map", filepath.Join(t.TempDir(), "customers.tsv")}
}

func TestCLIDenyFile(t *testing.T) {
	t.Setenv("GOSO_CONTACTS_DENY_FILE", "")
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "export.json")
	body := `{"channel_contacts":[{"tenant_id":"minh-an","channel_type":"telegram","sender_id":"ext-minh-an-deny","display_name":"Minh An Deny","contact_type":"user","first_seen_at":"2026-01-15T08:00:00Z","last_seen_at":"2026-01-15T09:00:00Z"}]}`
	if err := os.WriteFile(exportPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "goso.db")
	base := []string{"--export", exportPath, "--tenant", "minh-an", "--source-tenant", "minh-an", "--db", dbPath, "--apply"}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(base, stdout, stderr); code == 0 {
		t.Fatal("missing deny file succeeded")
	}
	if strings.Contains(stdout.String(), "minh-an") || strings.Contains(stderr.String(), "minh-an") {
		t.Fatal("missing deny file printed a tenant")
	}
	assertNoRows(t, dbPath)

	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte("\n# comment only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(base, "--deny-file", empty), stdout, stderr); code == 0 || !strings.Contains(stderr.String(), "deny file is empty") {
		t.Fatalf("empty exit stderr %s", stderr.String())
	}
	assertNoRows(t, dbPath)

	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte("not a tenant!\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(base, "--deny-file", bad), stdout, stderr); code == 0 || !strings.Contains(stderr.String(), "deny file is malformed") {
		t.Fatalf("malformed exit stderr %s", stderr.String())
	}
	assertNoRows(t, dbPath)

	deny := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(deny, []byte("blocked-tenant\nminh-an\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(base, "--deny-file", deny), stdout, stderr); code == 0 {
		t.Fatal("denied tenant succeeded")
	}
	if strings.Contains(stdout.String(), "minh-an") || strings.Contains(stdout.String(), "blocked-tenant") || strings.Contains(stderr.String(), "minh-an") {
		t.Fatalf("denied report leaked a name stdout %s stderr %s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"denied": 1`) {
		t.Fatalf("stdout %s", stdout.String())
	}
	assertNoRows(t, dbPath)

	other := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(other, []byte("11111111-1111-4111-8111-111111111111\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	srcArgs := []string{"--export", exportPath, "--tenant", "minh-an", "--source-tenant", "11111111-1111-4111-8111-111111111111", "--db", dbPath, "--deny-file", other, "--apply"}
	if code := run(srcArgs, stdout, stderr); code == 0 || !strings.Contains(stdout.String(), `"denied": 1`) {
		t.Fatalf("source deny stdout %s stderr %s", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "11111111") {
		t.Fatal("report printed a denied id")
	}
	assertNoRows(t, dbPath)
}

func assertNoRows(t *testing.T, dbPath string) {
	t.Helper()
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return
	}
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if n, err := st.CountContacts("minh-an"); err != nil || n != 0 {
		t.Fatalf("rows %d %v", n, err)
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

func TestCLIApplyMapModeAndRollbackConfirm(t *testing.T) {
	t.Setenv("GOSO_CONTACTS_DENY_FILE", "")
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "export.json")
	body := `{"channel_contacts":[{"tenant_id":"minh-an","channel_type":"telegram","sender_id":"ext-minh-an-map","display_name":"Minh An Map","contact_type":"user","user_id":"user-map","first_seen_at":"2026-01-15T08:00:00Z","last_seen_at":"2026-01-15T09:00:00Z"}]}`
	if err := os.WriteFile(exportPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	deny := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(deny, []byte("blocked-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "goso.db")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	missing := []string{"--export", exportPath, "--tenant", "minh-an", "--source-tenant", "minh-an", "--db", dbPath, "--deny-file", deny, "--apply"}
	if code := run(missing, stdout, stderr); code == 0 || !strings.Contains(stderr.String(), "customer map is required") {
		t.Fatalf("missing map %d %s", code, stderr.String())
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("apply without a customer map created a database")
	}
	mapPath := filepath.Join(dir, "customers.tsv")
	args := append(missing, "--customer-map", mapPath)
	stdout.Reset()
	stderr.Reset()
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("apply %d %s", code, stderr.String())
	}
	info, err := os.Stat(mapPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", info, err)
	}
	raw, err := os.ReadFile(mapPath)
	if err != nil || string(raw) != "telegram\text-minh-an-map\tuser-map\ttelegram:ext-minh-an-map\n" {
		t.Fatalf("map %q %v", raw, err)
	}
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ObserveContact(store.ContactSighting{Channel: "telegram", Dest: "ext-minh-an-map", Kind: "user", TenantID: "minh-an"}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	roll := []string{"--tenant", "minh-an", "--source-tenant", "minh-an", "--db", dbPath, "--deny-file", deny, "--rollback"}
	stdout.Reset()
	stderr.Reset()
	if code := run(roll, stdout, stderr); code != 0 {
		t.Fatalf("rollback dry-run %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "dry_run=true") || !strings.Contains(stdout.String(), "delete=0") || !strings.Contains(stdout.String(), "active=1") {
		t.Fatalf("stdout %s", stdout.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "ext-minh-an-map") || strings.Contains(stdout.String()+stderr.String(), "Minh An") {
		t.Fatal("rollback printed a contact")
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := st.CountContacts("minh-an"); err != nil || n != 1 {
		t.Fatalf("dry-run rollback count %d %v", n, err)
	}
	_ = st.Close()
	stdout.Reset()
	stderr.Reset()
	if code := run(append(roll, "--apply"), stdout, stderr); code == 0 || !strings.Contains(stderr.String(), "rollback needs confirm") {
		t.Fatalf("confirm missing %d %s %s", code, stdout.String(), stderr.String())
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := st.CountContacts("minh-an"); err != nil || n != 1 {
		t.Fatalf("refused rollback count %d %v", n, err)
	}
	_ = st.Close()
	stdout.Reset()
	stderr.Reset()
	if code := run(append(roll, "--apply", "--confirm-rollback"), stdout, stderr); code != 0 {
		t.Fatalf("confirm %d %s %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "dry_run=false") || !strings.Contains(stdout.String(), "delete=1") {
		t.Fatalf("stdout %s", stdout.String())
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if n, err := st.CountContacts("minh-an"); err != nil || n != 0 {
		t.Fatalf("after confirm %d %v", n, err)
	}
}
