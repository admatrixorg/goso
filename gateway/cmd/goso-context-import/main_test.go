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

func TestCLIDryRunIsDefaultAndRerunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "export.json")
	body := []byte(`{
	  "agent_context_files": [
	    {"tenant_id":"11111111-1111-4111-8111-111111111111","agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","file_name":"AGENTS.md","content":"Chiết khấu tối đa của Minh An là 9 phần trăm.","created_at":"2026-03-01T04:05:06Z","updated_at":"2026-04-02T05:06:07Z","embedding":"EMBEDMARKERvector"},
	    {"tenant_id":"22222222-2222-4222-8222-222222222222","agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","file_name":"AGENTS.md","content":"FOREIGNAGENTMARKER"}
	  ],
	  "user_context_files": [
	    {"tenant_id":"11111111-1111-4111-8111-111111111111","agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","user_id":"lan","file_name":"USER.md","content":"Lan chỉ nhận báo giá một dòng.","created_at":"2026-03-01T04:05:06Z","updated_at":"2026-04-02T05:06:07Z"}
	  ]
	}`)
	if err := os.WriteFile(exportPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	mapPath := filepath.Join(dir, "agents.json")
	if err := os.WriteFile(mapPath, []byte(`{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa":"tu-van-sales"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(denyPath, []byte("not-this-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "goso.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ag, err := st.CreateAgent(store.Agent{AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--export", exportPath,
		"--source-tenant", "11111111-1111-4111-8111-111111111111",
		"--tenant", "minh-an",
		"--db", dbPath,
		"--agent-map", mapPath,
		"--deny-file", denyPath,
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"dry_run": true`) || !strings.Contains(stdout.String(), `"inserted": 2`) || !strings.Contains(stdout.String(), `"rejected": 1`) || !strings.Contains(stdout.String(), `"denied_tenants": 0`) {
		t.Fatalf("stdout %s", stdout.String())
	}
	assertNoBodies(t, stdout.String()+stderr.String())
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetContextFile("minh-an", store.ContextScopeAgent, ag.ID, "AGENTS.md"); err == nil {
		t.Fatal("default run wrote a file")
	}
	_ = st.Close()

	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--apply", "--dry-run"), stdout, stderr); code != 2 {
		t.Fatalf("conflict exit %d %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("apply exit %d %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), `"dry_run": true`) || !strings.Contains(stdout.String(), `"inserted": 2`) {
		t.Fatalf("apply stdout %s", stdout.String())
	}
	assertNoBodies(t, stdout.String()+stderr.String())

	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--apply"), stdout, stderr); code != 0 {
		t.Fatalf("rerun exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"inserted": 0`) || !strings.Contains(stdout.String(), `"updated": 0`) || !strings.Contains(stdout.String(), `"skipped": 2`) {
		t.Fatalf("rerun stdout %s", stdout.String())
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.GetContextFile("minh-an", store.ContextScopeAgent, ag.ID, "AGENTS.md")
	if err != nil || got.Body != "Chiết khấu tối đa của Minh An là 9 phần trăm." {
		t.Fatalf("stored %+v %v", got, err)
	}
	lan, err := st.GetContextFile("minh-an", store.ContextScopeUser, "goclaw:lan", "USER.md")
	if err != nil || lan.Body != "Lan chỉ nhận báo giá một dòng." {
		t.Fatalf("user %+v %v", lan, err)
	}
}

func TestCLIDenylistFailsClosed(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "export.json")
	if err := os.WriteFile(exportPath, []byte(`{"agent_context_files":[],"user_context_files":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "goso.db")
	args := []string{"--export", exportPath, "--source-tenant", "minh-an", "--tenant", "minh-an", "--db", dbPath, "--apply"}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(args, stdout, stderr); code == 0 || !strings.Contains(stderr.String(), "missing") {
		t.Fatalf("missing exit %d %s", code, stderr.String())
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("missing denylist opened the database")
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte("\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--deny-file", empty), stdout, stderr); code == 0 || !strings.Contains(stderr.String(), "empty") {
		t.Fatalf("empty exit %d %s", code, stderr.String())
	}
	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(append(args, "--deny-file", bad), stdout, stderr); code == 0 || !strings.Contains(stderr.String(), "malformed") {
		t.Fatalf("malformed exit %d %s", code, stderr.String())
	}
	list := filepath.Join(dir, "list.txt")
	if err := os.WriteFile(list, []byte("blocked-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code := run([]string{"--export", exportPath, "--source-tenant", "minh-an", "--tenant", "blocked-tenant", "--db", dbPath, "--deny-file", list}, stdout, stderr)
	if code == 0 || strings.Contains(stdout.String(), "blocked-tenant") || strings.Contains(stderr.String(), "blocked-tenant") || !strings.Contains(stdout.String(), `"denied_tenants": 1`) {
		t.Fatalf("denied exit %d stdout %s stderr %s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("denied tenant opened the database")
	}
}

func TestCLIRequiresTenantAndRefusesRemoteDB(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run([]string{"--export", "x", "--source-tenant", "t", "--db", "x.db"}, stdout, stderr); code != 2 {
		t.Fatalf("missing tenant exit %d", code)
	}
	if err := requireLoopback("postgres://goso@db.example/goso"); err == nil {
		t.Fatal("remote host allowed")
	}
	t.Setenv("GOSO_DATABASE_URL", "postgres://goso@127.0.0.1/goso")
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "export.json")
	if err := os.WriteFile(exportPath, []byte(`{"agent_context_files":[],"user_context_files":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	deny := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(deny, []byte("not-this-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code := run([]string{
		"--export", exportPath,
		"--source-tenant", "minh-an",
		"--tenant", "minh-an",
		"--db", filepath.Join(dir, "goso.db"),
		"--deny-file", deny,
		"--apply",
	}, stdout, stderr)
	if code != 1 || !strings.Contains(stderr.String(), "GOSO_DATABASE_URL") {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
}

func assertNoBodies(t *testing.T, text string) {
	t.Helper()
	for _, marker := range []string{"9 phần trăm", "FOREIGNAGENTMARKER", "EMBEDMARKERvector", "báo giá một dòng"} {
		if strings.Contains(text, marker) {
			t.Fatalf("output contains %q: %s", marker, text)
		}
	}
}
