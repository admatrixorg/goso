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

func TestMain(m *testing.M) {
	_ = os.Setenv("GOSO_LITE", "")
	_ = os.Setenv("GOSO_DATABASE_URL", "")
	os.Exit(m.Run())
}

func TestCLIDryRunApplyRollback(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join("..", "..", "internal", "agentimport", "testdata", "minh-an.json")
	dbPath := filepath.Join(dir, "goso.db")
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(denyPath, []byte("blocked-sample\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--denylist", denyPath,
		"--export", exportPath,
		"--source-tenant", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
		"--tenant", "minh-an",
		"--db", dbPath,
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(append(append([]string{}, args...), "--dry-run=false"), stdout, stderr); code != 0 {
		t.Fatalf("dry-run=false exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"dry_run": true`) || !strings.Contains(stdout.String(), `"inserted": 4`) {
		t.Fatalf("stdout %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "SECRETMARKER") || strings.Contains(stderr.String(), "SECRETMARKER") || strings.Contains(stdout.String(), "EMBEDMARKER") || strings.Contains(stdout.String(), "blocked-sample") {
		t.Fatal("cli leaked a dropped column")
	}
	if !strings.Contains(stdout.String(), `"denied_tenants": 0`) {
		t.Fatalf("missing denied count %s", stdout.String())
	}
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if n := countKey(st, "minh-an-tu-van"); n != 0 {
		t.Fatal("dry-run wrote")
	}
	db, pg := st.SQL()
	if db == nil || pg {
		t.Fatal("sql")
	}
	var mapped int
	if err := db.QueryRow(`SELECT COUNT(*) FROM goclaw_agent_import_map`).Scan(&mapped); err != nil {
		t.Fatal(err)
	}
	if mapped != 0 {
		t.Fatal("dry-run wrote map rows")
	}
	_ = st.Close()

	stdout.Reset()
	stderr.Reset()
	if code := run(append(append([]string{}, args...), "--apply", "--dry-run"), stdout, stderr); code != 2 {
		t.Fatalf("combined exit %d %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(append(append([]string{}, args...), "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("apply exit %d %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "SECRETMARKER") || strings.Contains(stderr.String(), "Mau tu van") {
		t.Fatal("cli leaked row text")
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(append([]string{}, args...), "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("second exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"inserted": 0`) || !strings.Contains(stdout.String(), `"unchanged"`) {
		t.Fatalf("second stdout %s", stdout.String())
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if countKey(st, "minh-an-tu-van") != 1 || countKey(st, "minh-an-b-tu-van") != 0 {
		t.Fatal("apply counts")
	}
	_ = st.Close()

	stdout.Reset()
	stderr.Reset()
	rbArgs := []string{"--denylist", denyPath, "--rollback", "--tenant", "minh-an", "--db", dbPath}
	if code := run(rbArgs, stdout, stderr); code != 0 {
		t.Fatalf("rollback dry-run %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"dry_run": true`) || !strings.Contains(stdout.String(), `"agents": 4`) {
		t.Fatalf("rollback stdout %s", stdout.String())
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if countKey(st, "minh-an-tu-van") != 1 {
		t.Fatal("rollback dry-run deleted")
	}
	_ = st.Close()

	stdout.Reset()
	stderr.Reset()
	if code := run(append(rbArgs, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("rollback apply %d %s", code, stderr.String())
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if countKey(st, "minh-an-tu-van") != 0 || countKey(st, "minh-an-nghi") != 0 {
		t.Fatal("rollback left agents")
	}
}

func TestCLIRefusesRemoteAndBadFlags(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(nil, stdout, stderr); code != 2 {
		t.Fatalf("usage %d", code)
	}
	dir := t.TempDir()
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(denyPath, []byte("blocked-sample\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{
		"--denylist", denyPath,
		"--export", "x",
		"--source-tenant", "t",
		"--tenant", "minh-an",
		"--db", "postgres://goso:goso@example.com:5432/goso",
		"--apply",
	}, stdout, stderr); code != 1 || !strings.Contains(stderr.String(), "non-local") {
		t.Fatalf("remote %d %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--rollback", "--export", "x", "--tenant", "minh-an", "--db", filepath.Join(t.TempDir(), "a.db")}, stdout, stderr); code != 2 {
		t.Fatalf("rollback export %d %s", code, stderr.String())
	}
}

func TestCLIDenylistStopsBeforeWork(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join("..", "..", "internal", "agentimport", "testdata", "minh-an.json")
	dbPath := filepath.Join(dir, "goso.db")
	base := []string{
		"--export", exportPath,
		"--source-tenant", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
		"--tenant", "minh-an",
		"--db", dbPath,
		"--apply",
	}
	cases := []struct {
		name string
		body *string
	}{
		{name: "missing", body: nil},
		{name: "empty", body: strPtr("# comment only\n")},
		{name: "malformed", body: strPtr("not valid\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".txt")
			if tc.body == nil {
				path = filepath.Join(dir, "no-such-file.txt")
			} else if err := os.WriteFile(path, []byte(*tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			code := run(append([]string{"--denylist", path}, base...), stdout, stderr)
			if code == 0 {
				t.Fatalf("exit 0 stderr %s", stderr.String())
			}
			if _, err := os.Stat(dbPath); err == nil {
				t.Fatal("database was created")
			}
			if strings.Contains(stdout.String()+stderr.String(), "blocked-sample") {
				t.Fatal("leaked a denylist name")
			}
		})
	}
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(denyPath, []byte("blocked-sample\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{
		"--denylist", denyPath,
		"--export", exportPath,
		"--source-tenant", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
		"--tenant", "blocked-sample",
		"--db", dbPath,
		"--apply",
	}, stdout, stderr)
	if code == 0 || !strings.Contains(stderr.String(), "tenant is denied") {
		t.Fatalf("denied exit %d %s", code, stderr.String())
	}
	if _, err := os.Stat(dbPath); err == nil {
		t.Fatal("denied run created a database")
	}
	if strings.Contains(stdout.String(), "blocked-sample") || strings.Contains(stderr.String(), "blocked-sample") {
		t.Fatal("report named a denied tenant")
	}
	if !strings.Contains(stdout.String(), `"denied_tenants": 1`) {
		t.Fatalf("stdout %s", stdout.String())
	}
}

func strPtr(s string) *string { return &s }

func countKey(st *store.SQLiteStore, key string) int {
	n := 0
	for _, a := range st.ListAgents() {
		if a != nil && a.AgentKey == key {
			n++
		}
	}
	return n
}
