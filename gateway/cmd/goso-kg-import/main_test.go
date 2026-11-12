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

func TestCLIDryRunThenSecondRunThenRemove(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "kg.jsonl")
	body := `{"table":"kg_entities","id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1","tenant_id":"11111111-1111-4111-8111-111111111111","agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","user_id":"","name":"Kho Cần Thơ","entity_type":"place","description":"KEEPMARKER","properties":{"khu":"mien tay"},"created_at":"2026-01-15T00:00:00Z","valid_from":null,"embedding":"EMBEDMARKER"}
{"table":"kg_entities","id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb3","tenant_id":"11111111-1111-4111-8111-111111111111","agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","user_id":"user-minh-an-01","name":"Ghi chu khach","entity_type":"person","description":"PRIVMARKER 0900000111","created_at":"2026-01-01T00:00:00Z","valid_from":"2026-01-01T00:00:00Z"}
{"table":"kg_entities","id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb9","tenant_id":"denied-tenant","agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","user_id":"","name":"Khong nhap","entity_type":"org","description":"DENIEDMARKER","created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(exportPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "goso.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ag, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "co-van-minh-an", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	tenantMap := filepath.Join(dir, "tenants.json")
	agentMap := filepath.Join(dir, "agents.json")
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(tenantMap, []byte(`{"tenants":[
		{"source_tenant":"11111111-1111-4111-8111-111111111111","dest_tenant":"minh-an","slug":"minh-an"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentMap, []byte(`{"agents":[{"goclaw_agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","goso_agent_id":"`+ag.ID+`","source_tenant":"11111111-1111-4111-8111-111111111111"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(denyPath, []byte("denied-tenant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--export", exportPath, "--tenant-map", tenantMap, "--agent-map", agentMap,
		"--source-tenant", "11111111-1111-4111-8111-111111111111", "--tenant", "minh-an",
		"--deny-file", denyPath, "--db", dbPath,
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(append(append([]string{}, args...), "--dry-run"), stdout, stderr); code != 0 {
		t.Fatalf("dry-run %d %s", code, stderr.String())
	}
	out := stdout.String() + stderr.String()
	for _, secret := range []string{"KEEPMARKER", "PRIVMARKER", "EMBEDMARKER", "user-minh-an-01", "0900000111", "denied-tenant"} {
		if strings.Contains(out, secret) {
			t.Fatalf("output leaked %s", secret)
		}
	}
	if !strings.Contains(stdout.String(), `"will_write": 1`) || !strings.Contains(stdout.String(), `"customer-private": 1`) || !strings.Contains(stdout.String(), `"denied_tenants": 1`) || !strings.Contains(stdout.String(), `"team/scope": 0`) {
		t.Fatalf("stdout %s", stdout.String())
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetKGEntity("gc-kg:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1"); err == nil {
		t.Fatal("dry-run wrote")
	}
	_ = st.Close()

	stdout.Reset()
	stderr.Reset()
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("import %d %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(args, stdout, stderr); code != 0 {
		t.Fatalf("second %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"will_write": 0`) || !strings.Contains(stdout.String(), `"exists": 1`) {
		t.Fatalf("second stdout %s", stdout.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "user-minh-an-01") || strings.Contains(stdout.String()+stderr.String(), "0900000111") {
		t.Fatal("second run leaked an identifier")
	}
	stdout.Reset()
	if code := run([]string{"--remove", "--tenant", "minh-an", "--deny-file", denyPath, "--db", dbPath, "--dry-run"}, stdout, stderr); code != 0 {
		t.Fatalf("dry remove %d %s", code, stderr.String())
	}
	stdout.Reset()
	if code := run([]string{"--remove", "--tenant", "minh-an", "--deny-file", denyPath, "--db", dbPath}, stdout, stderr); code != 0 {
		t.Fatalf("remove %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"entities": 1`) {
		t.Fatalf("remove stdout %s", stdout.String())
	}
	st, err = store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.GetKGEntity("gc-kg:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1"); err != store.ErrNotFound {
		t.Fatal("removed row remains")
	}
}

func TestCLIDenyFileBlocksBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "kg.jsonl")
	if err := os.WriteFile(exportPath, []byte(`{"table":"kg_entities","id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1","tenant_id":"11111111-1111-4111-8111-111111111111","agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","user_id":"","name":"Kho","entity_type":"place","created_at":"2026-01-15T00:00:00Z"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tenantMap := filepath.Join(dir, "tenants.json")
	agentMap := filepath.Join(dir, "agents.json")
	if err := os.WriteFile(tenantMap, []byte(`{"tenants":[{"source_tenant":"11111111-1111-4111-8111-111111111111","dest_tenant":"minh-an","slug":"minh-an"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentMap, []byte(`{"agents":[{"goclaw_agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","goso_agent_id":"agent-1","source_tenant":"11111111-1111-4111-8111-111111111111"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	base := []string{"--export", exportPath, "--tenant-map", tenantMap, "--agent-map", agentMap, "--source-tenant", "11111111-1111-4111-8111-111111111111", "--tenant", "minh-an", "--dry-run"}
	cases := []struct {
		name string
		file string
		body *string
	}{
		{name: "missing", file: filepath.Join(dir, "no-such-deny.txt")},
		{name: "empty", file: filepath.Join(dir, "empty.txt"), body: strptr("")},
		{name: "malformed", file: filepath.Join(dir, "bad.txt"), body: strptr("{")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.body != nil {
				if err := os.WriteFile(tc.file, []byte(*tc.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			dbPath := filepath.Join(dir, "db-"+tc.name+".db")
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			code := run(append(append([]string{}, base...), "--deny-file", tc.file, "--db", dbPath), stdout, stderr)
			if code == 0 {
				t.Fatal("expected non-zero")
			}
			if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
				t.Fatal("database was created")
			}
		})
	}
	denied := filepath.Join(dir, "denied.txt")
	if err := os.WriteFile(denied, []byte("minh-an\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "denied.db")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(append(append([]string{}, base...), "--deny-file", denied, "--db", dbPath), stdout, stderr); code == 0 {
		t.Fatal("denied tenant ran")
	}
	if strings.Contains(stdout.String()+stderr.String(), "minh-an") {
		t.Fatal("denied name was printed")
	}
	if !strings.Contains(stdout.String(), `"denied_tenants": 1`) {
		t.Fatalf("stdout %s", stdout.String())
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("denied run created a database")
	}
}

func strptr(s string) *string { return &s }
