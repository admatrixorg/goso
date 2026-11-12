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
	    {"tenant_id":"11111111-1111-4111-8111-111111111111","agent_key":"tu-van-sales","name":"AGENTS.md","body":"Chiết khấu tối đa của Minh An là 9 phần trăm.","embedding":"EMBEDMARKERvector"},
	    {"tenant_id":"22222222-2222-4222-8222-222222222222","agent_key":"tu-van-sales","name":"AGENTS.md","body":"FOREIGNAGENTMARKER"}
	  ],
	  "user_context_files": [
	    {"tenant_id":"11111111-1111-4111-8111-111111111111","user_key":"lan","name":"USER.md","body":"Lan chỉ nhận báo giá một dòng."}
	  ]
	}`)
	if err := os.WriteFile(exportPath, body, 0o644); err != nil {
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
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("dry-run exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"dry_run": true`) || !strings.Contains(stdout.String(), `"inserted": 2`) || !strings.Contains(stdout.String(), `"rejected": 1`) {
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
	lan, err := st.GetContextFile("minh-an", store.ContextScopeUser, "lan", "USER.md")
	if err != nil || lan.Body != "Lan chỉ nhận báo giá một dòng." {
		t.Fatalf("user %+v %v", lan, err)
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
	stdout.Reset()
	stderr.Reset()
	code := run([]string{
		"--export", exportPath,
		"--source-tenant", "minh-an",
		"--tenant", "minh-an",
		"--db", filepath.Join(dir, "goso.db"),
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
