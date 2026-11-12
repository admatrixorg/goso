// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/sessionimport"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCLIDryRunThenApply(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cli.db")
	export := filepath.Join("..", "..", "internal", "sessionimport", "testdata", "minh-an-sessions.jsonl")
	var stdout, stderr bytes.Buffer
	code := run([]string{"--export", export, "--tenant", "minh-an", "--db", dbPath, "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatal(stderr.String())
	}
	var rep sessionimport.Report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun {
		t.Fatal("dry_run")
	}
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db, _ := st.SQL()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("dry-run sessions %d %v", n, err)
	}
	var mem int
	if err := db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&mem); err != nil || mem != 0 {
		t.Fatal(err)
	}
	_ = st.Close()

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--export", export, "--tenant", "minh-an", "--db", dbPath, "--apply"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("apply exit %d %s", code, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.DryRun {
		t.Fatal("apply was a dry-run")
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	db, _ = st.SQL()
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("sessions %d %v", n, err)
	}
	var msgs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&msgs); err != nil || msgs != 6 {
		t.Fatalf("messages %d %v", msgs, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&mem); err != nil || mem != 0 {
		t.Fatal(err)
	}
	stdout.Reset()
	code = run([]string{"--export", export, "--tenant", "minh-an", "--db", dbPath, "--apply"}, &stdout, &stderr)
	if code != 0 {
		t.Fatal(stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	var skipped int
	for _, row := range rep.Tenants {
		if row.TenantID == "minh-an" {
			skipped = row.Skipped
			if row.New != 0 {
				t.Fatalf("rerun %+v", row)
			}
		}
	}
	if skipped != 6 {
		t.Fatalf("skipped %d", skipped)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("rerun sessions %d", n)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--export", export, "--tenant", "minh-an", "--db", dbPath, "--apply", "--dry-run"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "either") {
		t.Fatalf("conflict %d %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--export", export, "--tenant", "minh-an", "--db", "postgres://example.com/goso"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "non-local") {
		t.Fatalf("remote %d %s", code, stderr.String())
	}
}
