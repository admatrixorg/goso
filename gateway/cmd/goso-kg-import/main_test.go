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
{"table":"kg_entities","id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb3","tenant_id":"11111111-1111-4111-8111-111111111111","agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","user_id":"khach-a","name":"Ghi chu khach","entity_type":"person","description":"PRIVMARKER","created_at":"2026-01-01T00:00:00Z","valid_from":"2026-01-01T00:00:00Z"}
{"table":"kg_entities","id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb9","tenant_id":"33333333-3333-4333-8333-333333333333","agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","user_id":"","name":"Khong nhap","entity_type":"org","description":"MQMARKER","created_at":"2026-01-01T00:00:00Z"}
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
	if err := os.WriteFile(tenantMap, []byte(`{"tenants":[
		{"source_tenant":"11111111-1111-4111-8111-111111111111","dest_tenant":"minh-an","slug":"minh-an"},
		{"source_tenant":"33333333-3333-4333-8333-333333333333","dest_tenant":"mq-capital","slug":"mq-capital"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentMap, []byte(`{"agents":[{"goclaw_agent_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","goso_agent_id":"`+ag.ID+`","source_tenant":"11111111-1111-4111-8111-111111111111"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"--export", exportPath, "--tenant-map", tenantMap, "--agent-map", agentMap, "--db", dbPath}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run(append(args, "--dry-run"), stdout, stderr); code != 0 {
		t.Fatalf("dry-run %d %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "KEEPMARKER") || strings.Contains(stdout.String(), "PRIVMARKER") || strings.Contains(stdout.String(), "EMBEDMARKER") || strings.Contains(stderr.String(), "KEEPMARKER") || strings.Contains(stderr.String(), "MQMARKER") {
		t.Fatal("report leaked a body")
	}
	if !strings.Contains(stdout.String(), `"will_write": 1`) || !strings.Contains(stdout.String(), `"customer-private": 1`) || !strings.Contains(stdout.String(), `"mq": 1`) || !strings.Contains(stdout.String(), `"team/scope": 0`) {
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
	if code := run([]string{"--remove", "--dest-tenant", "minh-an", "--db", dbPath, "--dry-run"}, stdout, stderr); code != 0 {
		t.Fatalf("dry remove %d %s", code, stderr.String())
	}
	stdout.Reset()
	if code := run([]string{"--remove", "--dest-tenant", "minh-an", "--db", dbPath}, stdout, stderr); code != 0 {
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
	if code := run([]string{"--db", "postgres://goso:goso@example.com:5432/goso"}, stdout, stderr); code == 0 {
		t.Fatal("remote database was accepted")
	}
	stderr.Reset()
	if code := run([]string{"--export", "postgres://goso:secret@127.0.0.1/goso", "--tenant-map", tenantMap, "--agent-map", agentMap, "--db", dbPath}, stdout, stderr); code == 0 {
		t.Fatal("dsn export was accepted")
	}
	if strings.Contains(stderr.String(), "secret") {
		t.Fatal("export dsn leaked")
	}
}
