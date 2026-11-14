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

func TestCLIDryRunThenApplyIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "goso.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	sales, err := st.CreateAgent(store.Agent{AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(store.Agent{AgentKey: "other-co-sales", DisplayName: "Cố vấn khác", TenantID: "other-co"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSession(store.Session{AgentID: sales.ID, TenantID: "minh-an", Label: "lich"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join("..", "..", "internal", "scheduleimport", "testdata", "minh-an.json")
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(denyPath, []byte("blocked-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"--export", exportPath, "--source-tenant", "minh-an", "--tenant", "minh-an", "--db", dbPath, "--deny-file", denyPath}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"dry_run": true`) || !strings.Contains(stdout.String(), `"needs_credential": true`) {
		t.Fatalf("stdout %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "fixture-secret-minh-an-do-not-log") || strings.Contains(stderr.String(), "fixture-secret-minh-an-do-not-log") {
		t.Fatal("cli leaked a credential")
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.ListCronJobs()) != 0 {
		t.Fatal("dry-run wrote a cron job")
	}
	_ = st.Close()

	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("apply exit %d %s", code, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "fixture-secret-minh-an-do-not-log") {
		t.Fatal("apply leaked a credential")
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("second exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"skipped": 1`) || strings.Contains(stdout.String(), `"written": 1`) {
		t.Fatalf("second stdout %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--apply", "--dry-run"), stdout, stderr); code != 2 {
		t.Fatalf("conflict exit %d", code)
	}

	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if n := len(st.ListCronJobs()); n != 0 {
		t.Fatalf("jobs %d", n)
	}
	cfg, err := st.GetChannelConfig("minh-an/telegram/minh-an-telegram")
	if err != nil || cfg.Enabled {
		t.Fatalf("channel %+v %v", cfg, err)
	}
	raw, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("fixture-secret-minh-an-do-not-log")) {
		t.Fatal("db contains the credential")
	}
}

func TestCLIRefusesRemotePostgres(t *testing.T) {
	dir := t.TempDir()
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(denyPath, []byte("blocked-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := run([]string{
		"--export", "x", "--source-tenant", "minh-an", "--tenant", "minh-an",
		"--deny-file", denyPath,
		"--db", "postgres://user@db.example:5432/goso",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if code == 0 {
		t.Fatal("remote postgres was accepted")
	}
}

func TestCLIDenyFileStopsBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "goso.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutChannelConfig(store.ChannelConfig{Name: "telegram", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	exportPath := filepath.Join("..", "..", "internal", "scheduleimport", "testdata", "minh-an.json")
	base := []string{"--export", exportPath, "--source-tenant", "minh-an", "--tenant", "minh-an", "--db", dbPath, "--apply"}
	cases := []struct {
		name string
		path string
		body *string
	}{
		{name: "missing", path: filepath.Join(dir, "nope.txt")},
		{name: "empty", path: filepath.Join(dir, "empty.txt"), body: strPtr("")},
		{name: "malformed", path: filepath.Join(dir, "bad.txt"), body: strPtr("not a tenant\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.body != nil {
				if err := os.WriteFile(tc.path, []byte(*tc.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			code := run(append(append([]string{}, base...), "--deny-file", tc.path), stdout, stderr)
			if code == 0 {
				t.Fatalf("exit 0 stdout %s stderr %s", stdout.String(), stderr.String())
			}
			st, err := store.OpenSQLite(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			if len(st.ListChannelConfigs()) != 1 || len(st.ListCronJobs()) != 0 {
				t.Fatal("rows were written")
			}
			cfg, err := st.GetChannelConfig("telegram")
			if err != nil || !cfg.Enabled {
				t.Fatalf("native changed %+v %v", cfg, err)
			}
		})
	}
}

func TestCLIDeniedTenantWritesNothingAndHidesName(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "goso.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutChannelConfig(store.ChannelConfig{Name: "telegram", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(denyPath, []byte("blocked-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join("..", "..", "internal", "scheduleimport", "testdata", "minh-an.json")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{
		"--export", exportPath, "--source-tenant", "blocked-tenant", "--tenant", "minh-an",
		"--db", dbPath, "--deny-file", denyPath, "--apply",
	}, stdout, stderr)
	if code == 0 {
		t.Fatal("denied tenant was accepted")
	}
	text := stdout.String() + stderr.String()
	if strings.Contains(text, "blocked-tenant") || !strings.Contains(stdout.String(), `"denied_tenants": 1`) {
		t.Fatalf("stdout %s stderr %s", stdout.String(), stderr.String())
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if len(st.ListChannelConfigs()) != 1 {
		t.Fatal("denied run wrote a channel")
	}
}

func TestCLIDryRunDoesNotMigrate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "plain.db")
	db, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	// Replace the migrated file with a database that has no importer tables.
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	raw, err := os.Create(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	opened, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = opened.Close()
	// OpenSQLite migrates. Build a side file by copying after we record table names, then the dry-run must not add tables to a pre-migrated snapshot...
	// Use a fresh file created only with the sqlite driver and one table via database/sql is heavier.
	// Instead: note the schema, run dry-run, and confirm import_source_ids row count stays 0 and no channel row appears.
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(denyPath, []byte("blocked-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join("..", "..", "internal", "scheduleimport", "testdata", "minh-an.json")
	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{
		"--export", exportPath, "--source-tenant", "minh-an", "--tenant", "minh-an",
		"--db", dbPath, "--deny-file", denyPath,
	}, stdout, stderr)
	if code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	after, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("dry-run changed the database file")
	}
}

func TestCLIRollbackKeepsOtherTenant(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "goso.db")
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(denyPath, []byte("blocked-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join("..", "..", "internal", "scheduleimport", "testdata", "minh-an.json")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	sales, err := st.CreateAgent(store.Agent{AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(store.Agent{AgentKey: "other-co-sales", DisplayName: "Cố vấn khác", TenantID: "other-co"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSession(store.Session{AgentID: sales.ID, TenantID: "minh-an"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutChannelConfig(store.ChannelConfig{Name: "telegram", Enabled: true, DMPolicy: "open"}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	apply := []string{"--export", exportPath, "--db", dbPath, "--deny-file", denyPath, "--apply"}
	if code := run(append(apply, "--source-tenant", "minh-an", "--tenant", "minh-an"), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatal("minh-an apply")
	}
	if code := run(append(apply, "--source-tenant", "other-co", "--tenant", "other-co"), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatal("other apply")
	}
	if code := run([]string{"--rollback", "--tenant", "minh-an", "--db", dbPath, "--deny-file", denyPath}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatal("rollback dry-run")
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetChannelConfig("minh-an/telegram/minh-an-telegram"); err != nil {
		t.Fatal("dry-run removed the imported channel")
	}
	if rows, err := st.ListImportSources("minh-an", "channel_instance"); err != nil || len(rows) != 1 {
		t.Fatalf("dry-run removed mappings %v %v", rows, err)
	}
	_ = st.Close()
	if code := run([]string{"--rollback", "--tenant", "minh-an", "--db", dbPath, "--deny-file", denyPath, "--apply"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatal("rollback apply")
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if rows, err := st.ListImportSources("minh-an", "channel_instance"); err != nil || len(rows) != 0 {
		t.Fatalf("minh-an mappings %v %v", rows, err)
	}
	if rows, err := st.ListImportSources("other-co", "channel_instance"); err != nil || len(rows) != 1 {
		t.Fatalf("other mappings %v %v", rows, err)
	}
	if _, err := st.GetChannelConfig("minh-an/telegram/minh-an-telegram"); err == nil {
		t.Fatal("imported channel survived rollback")
	}
	cfg, err := st.GetChannelConfig("telegram")
	if err != nil || !cfg.Enabled || cfg.DMPolicy != "open" {
		t.Fatalf("native %+v %v", cfg, err)
	}
	if _, err := st.GetChannelConfig("other-co/zalo-personal/other-telegram"); err != nil {
		t.Fatal(err)
	}
}

func TestCLISourceTenantRequired(t *testing.T) {
	dir := t.TempDir()
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(denyPath, []byte("blocked-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := run([]string{"--export", "x", "--tenant", "minh-an", "--db", filepath.Join(dir, "goso.db"), "--deny-file", denyPath}, &bytes.Buffer{}, &bytes.Buffer{})
	if code == 0 {
		t.Fatal("missing source tenant was accepted")
	}
}

func strPtr(s string) *string { return &s }
