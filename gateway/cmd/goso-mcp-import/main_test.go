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

	"github.com/mqglobal/goso/gateway/internal/mcpimport"
	"github.com/mqglobal/goso/gateway/internal/store"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestCLIDryRunWritesNothingThenApplyIsIdempotent(t *testing.T) {
	exportPath := filepath.Join("..", "..", "internal", "mcpimport", "testdata", "minh-an.json")
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "goso.db")
	seed(t, dbPath)
	args := []string{"--export", exportPath, "--tenant", "minh-an", "--source-tenant", "minh-an", "--deny-file", writeDeny(t, dir), "--db", dbPath}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr %s", stderr.String())
	}
	rep := decodeReport(t, stdout.Bytes())
	if !rep.DryRun || len(rep.Tenants) < 2 || len(rep.NeedsCredential) != 2 {
		t.Fatalf("dry-run tenants %d needs %d", len(rep.Tenants), len(rep.NeedsCredential))
	}
	assertNoSecret(t, stdout.String(), stderr.String())
	st := open(t, dbPath)
	if n := len(st.ListConnectors()); n != 0 {
		t.Fatalf("dry-run connectors %d", n)
	}
	_ = st.Close()

	stdout.Reset()
	stderr.Reset()
	if code := run(append(append([]string{}, args...), "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("apply exit %d %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr %s", stderr.String())
	}
	rep = decodeReport(t, stdout.Bytes())
	if rep.DryRun || rep.NeedsCredential[0].SourceName != "notes" || rep.NeedsCredential[1].SourceName != "search" {
		t.Fatalf("apply needs %+v dry %v", rep.NeedsCredential, rep.DryRun)
	}
	assertNoSecret(t, stdout.String())
	st = open(t, dbPath)
	notes := find(st, "goclaw-minh-an-notes")
	if notes == nil || notes.Enabled || notes.CredentialRef != "" || notes.Endpoint != "http://127.0.0.1:9/minh-an-notes" {
		t.Fatalf("notes %+v", notes)
	}
	if find(st, "goclaw-other-shop-ledger") != nil {
		t.Fatal("foreign connector")
	}
	sales := agentByKey(t, st, "tu-van-sales")
	names, err := st.ListAgentConnectors(sales.ID)
	if err != nil || len(names) != 1 || names[0] != notes.Name {
		t.Fatalf("links %v %v", names, err)
	}
	other := agentByKey(t, st, "other-shop-ke-toan")
	if onames, err := st.ListAgentConnectors(other.ID); err != nil || len(onames) != 0 {
		t.Fatalf("other links %v %v", onames, err)
	}
	if _, err := st.GetSecret("connector/" + notes.Name + "/token"); err == nil {
		t.Fatal("secret stored")
	}
	_ = st.Close()
	assertDB(t, dbPath)

	stdout.Reset()
	if code := run(append(append([]string{}, args...), "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("rerun exit %d %s", code, stderr.String())
	}
	rep = decodeReport(t, stdout.Bytes())
	minh := tenant(t, rep, "minh-an")
	if minh.MCPServers.Inserted != 0 || minh.MCPServers.Skipped != 3 {
		t.Fatalf("rerun %+v", minh.MCPServers)
	}
	st = open(t, dbPath)
	defer st.Close()
	if len(st.ListConnectors()) != 3 {
		t.Fatalf("count %d", len(st.ListConnectors()))
	}
}

func TestCLIDenyFileStopsBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "export.json")
	if err := os.WriteFile(exportPath, []byte(`{"mcp_servers":[{"id":"00000000-0000-4000-8000-0000000000a1","name":"notes","transport":"stdio","command":"minh-an-files","tenant_id":"minh-an"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_IMPORT_DENY_FILE", "")
	dbPath := filepath.Join(dir, "missing.db")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	args := []string{"--export", exportPath, "--tenant", "minh-an", "--source-tenant", "minh-an", "--db", dbPath, "--apply"}
	if code := run(args, stdout, stderr); code == 0 {
		t.Fatal("missing deny file exited 0")
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("db created without a deny file")
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte("\n# comment only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run(append(args, "--deny-file", empty), stdout, stderr); code == 0 {
		t.Fatal("empty deny file exited 0")
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("db created for an empty deny file")
	}
	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte("not a tenant !!!\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--deny-file", bad), stdout, stderr); code == 0 {
		t.Fatal("malformed deny file exited 0")
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("db created for a malformed deny file")
	}
	listed := filepath.Join(dir, "list.txt")
	if err := os.WriteFile(listed, []byte("blocked-sample\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	deniedArgs := []string{"--export", exportPath, "--tenant", "blocked-sample", "--source-tenant", "blocked-sample", "--deny-file", listed, "--db", dbPath, "--apply"}
	if code := run(deniedArgs, stdout, stderr); code == 0 {
		t.Fatal("denied tenant exited 0")
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("denied tenant wrote a database")
	}
	if strings.Contains(stdout.String(), "blocked-sample") || strings.Contains(stderr.String(), "blocked-sample") {
		t.Fatal("output names a denied tenant")
	}
	var rep mcpimport.Report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.DeniedTenants != 1 {
		t.Fatalf("denied count %d", rep.DeniedTenants)
	}
}

func TestCLIRefusesWriteWithoutApply(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "export.json")
	if err := os.WriteFile(exportPath, []byte(`{"mcp_servers":[]}`), 0o644); err != nil {
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
	const dsn = "postgres://importer:super-secret-dsn-pass@db.example:5432/goso?sslmode=disable"
	deny := writeDeny(t, dir)
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--export", exportPath, "--tenant", "minh-an", "--source-tenant", "minh-an", "--deny-file", deny, "--db", dsn, "--apply"}, stdout, stderr); code != 1 {
		t.Fatalf("remote exit %d", code)
	}
	if !strings.Contains(stderr.String(), "refusing non-local database host") {
		t.Fatalf("stderr %s", stderr.String())
	}
	if strings.Contains(stderr.String(), "super-secret-dsn-pass") || strings.Contains(stdout.String(), "super-secret-dsn-pass") {
		t.Fatal("dsn secret printed")
	}
	t.Setenv("GOSO_DATABASE_URL", "postgres://goso@127.0.0.1/goso")
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--export", exportPath, "--tenant", "minh-an", "--source-tenant", "minh-an", "--deny-file", deny, "--db", dbPath, "--apply"}, stdout, stderr); code != 1 {
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
	const schema = "mcpimport176"
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
	pg, err := store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pg.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "tu-van-sales"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.CreateAgent(store.Agent{TenantID: "other-shop", AgentKey: "other-shop-ke-toan"}); err != nil {
		t.Fatal(err)
	}
	_ = pg.Close()

	exportPath := filepath.Join("..", "..", "internal", "mcpimport", "testdata", "minh-an.json")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	args := []string{"--export", exportPath, "--tenant", "minh-an", "--source-tenant", "minh-an", "--deny-file", writeDeny(t, t.TempDir()), "--db", u.String()}
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	rep := decodeReport(t, stdout.Bytes())
	if !rep.DryRun || len(rep.NeedsCredential) != 2 {
		t.Fatalf("dry-run %+v", rep.NeedsCredential)
	}
	assertNoSecret(t, stdout.String(), stderr.String())
	pg, err = store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	if n := len(pg.ListConnectors()); n != 0 {
		t.Fatalf("dry-run count %d", n)
	}
	_ = pg.Close()

	stdout.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("apply exit %d %s", code, stderr.String())
	}
	assertNoSecret(t, stdout.String(), stderr.String())
	pg, err = store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	var found *store.ConnectorRecord
	for _, c := range pg.ListConnectors() {
		if c.Name == "goclaw-minh-an-notes" {
			found = c
		}
		blob := c.Name + c.Endpoint + c.CredentialRef + c.SchemaVersion + string(c.ManifestJSON)
		if strings.Contains(blob, "placeholder-minh-an") {
			t.Fatal("postgres row contains a placeholder secret")
		}
	}
	if found == nil || found.Enabled || found.CredentialRef != "" {
		t.Fatalf("%+v", found)
	}
	if len(pg.ListConnectors()) != 3 {
		t.Fatalf("count %d", len(pg.ListConnectors()))
	}
}

func writeDeny(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(path, []byte("blocked-sample\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func seed(t *testing.T, dbPath string) {
	t.Helper()
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "tu-van-sales", DisplayName: "sales"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(store.Agent{TenantID: "other-shop", AgentKey: "other-shop-ke-toan", DisplayName: "ke toan"}); err != nil {
		t.Fatal(err)
	}
}

func open(t *testing.T, dbPath string) *store.SQLiteStore {
	t.Helper()
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func decodeReport(t *testing.T, b []byte) mcpimport.Report {
	t.Helper()
	var rep mcpimport.Report
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatal(err)
	}
	return rep
}

func tenant(t *testing.T, rep mcpimport.Report, id string) mcpimport.TenantReport {
	t.Helper()
	for _, row := range rep.Tenants {
		if row.TenantID == id {
			return row
		}
	}
	t.Fatalf("missing %s", id)
	return mcpimport.TenantReport{}
}

func find(st *store.SQLiteStore, name string) *store.ConnectorRecord {
	for _, c := range st.ListConnectors() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func agentByKey(t *testing.T, st *store.SQLiteStore, key string) *store.Agent {
	t.Helper()
	for _, a := range st.ListAgents() {
		if a.AgentKey == key {
			return a
		}
	}
	t.Fatalf("missing agent %s", key)
	return nil
}

func assertNoSecret(t *testing.T, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if strings.Contains(part, "placeholder-minh-an") || strings.Contains(part, "super-secret-dsn-pass") {
			t.Fatal("output contains a secret")
		}
	}
}

func assertDB(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("placeholder-minh-an")) {
		t.Fatal("database contains a placeholder secret")
	}
}
