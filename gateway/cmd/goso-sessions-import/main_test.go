// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/sessionimport"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCLIDryRunThenApply(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cli.db")
	deny := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(deny, []byte("held-shop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	export := filepath.Join("..", "..", "internal", "sessionimport", "testdata", "minh-an-sessions.jsonl")
	base := []string{"--deny-tenants", deny, "--source-tenant", "minh-an", "--export", export, "--tenant", "minh-an", "--db", dbPath}
	var stdout, stderr bytes.Buffer
	code := run(append(append([]string{}, base...), "--dry-run"), &stdout, &stderr)
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
	code = run(append(append([]string{}, base...), "--apply"), &stdout, &stderr)
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
	code = run(append(append([]string{}, base...), "--apply"), &stdout, &stderr)
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
	code = run([]string{"--deny-tenants", deny, "--source-tenant", "minh-an", "--export", export, "--tenant", "minh-an", "--db", "postgres://example.com/goso"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "non-local") {
		t.Fatalf("remote %d %s", code, stderr.String())
	}
}

func TestCLIDenylistExits(t *testing.T) {
	dir := t.TempDir()
	export := filepath.Join(dir, "one.jsonl")
	body := `{"id":"s-held","tenant_id":"held-shop","session_key":"k","user_id":"zalo-peer-88421","created_at":"2026-05-01T00:00:00Z","messages":[{"role":"user","content":"call 09000000123","created_at":"2026-05-01T00:00:01Z"}]}` + "\n"
	if err := os.WriteFile(export, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "none.db")
	runCase := func(deny string) (int, string, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		args := []string{"--export", export, "--tenant", "held-shop", "--source-tenant", "held-shop", "--db", dbPath}
		if deny != "" {
			args = append(args, "--deny-tenants", deny)
		}
		code := run(args, &stdout, &stderr)
		out := stdout.String() + stderr.String()
		if strings.Contains(out, "held-shop") || strings.Contains(out, "zalo-peer-88421") || strings.Contains(out, "09000000123") {
			t.Fatalf("identifier leaked: %s", out)
		}
		if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
			t.Fatal("database was created")
		}
		return code, stdout.String(), stderr.String()
	}
	if code, _, errText := runCase(filepath.Join(dir, "missing.txt")); code == 0 || !strings.Contains(errText, "missing") {
		t.Fatalf("missing %d %s", code, errText)
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte("# none\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errText := runCase(empty); code == 0 || !strings.Contains(errText, "empty") {
		t.Fatalf("empty %d %s", code, errText)
	}
	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte("not a token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errText := runCase(bad); code == 0 || !strings.Contains(errText, "malformed") {
		t.Fatalf("malformed %d %s", code, errText)
	}
	listed := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(listed, []byte("held-shop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, errText := runCase(listed)
	if code == 0 || !strings.Contains(errText, "denied") || !strings.Contains(stdout, `"denied": 1`) {
		t.Fatalf("denied %d %s %s", code, stdout, errText)
	}
}
