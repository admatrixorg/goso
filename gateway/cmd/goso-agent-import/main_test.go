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
	args := []string{
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
	if strings.Contains(stdout.String(), "SECRETMARKER") || strings.Contains(stderr.String(), "SECRETMARKER") || strings.Contains(stdout.String(), "EMBEDMARKER") {
		t.Fatal("cli leaked a dropped column")
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
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='goclaw_agent_import_map'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatal("dry-run created the map")
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
	rbArgs := []string{"--rollback", "--tenant", "minh-an", "--db", dbPath}
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
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{
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

func countKey(st *store.SQLiteStore, key string) int {
	n := 0
	for _, a := range st.ListAgents() {
		if a != nil && a.AgentKey == key {
			n++
		}
	}
	return n
}
