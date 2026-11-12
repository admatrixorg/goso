// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCLIDryRunThenImportIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "goso.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ag, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "co-van", DisplayName: "Co van Minh An", Model: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join(dir, "docs.jsonl")
	body := `{"id":"d1","agent_id":"agent-minh","user_id":null,"path":"memory/notes.md","content":"VAULTMEM minh an","hash":"abc","created_at":"2026-03-01T04:05:06Z","tenant_id":"11111111-1111-4111-8111-111111111111","team_id":null,"custom_scope":null}
{"id":"s1","agent_id":"agent-minh","user_id":null,"path":"_system/SOUL.md","content":"SYSBODY minh an","hash":"abc","tenant_id":"11111111-1111-4111-8111-111111111111"}
{"id":"u1","agent_id":"agent-minh","user_id":"khach-x","path":"memory/hop.md","content":"Minh An hopdongx","hash":"abc","tenant_id":"11111111-1111-4111-8111-111111111111"}
`
	if err := os.WriteFile(exportPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	mapPath := filepath.Join(dir, "map.json")
	raw, _ := json.Marshal(map[string]any{
		"tenants": []map[string]string{{"source_id": "11111111-1111-4111-8111-111111111111", "slug": "minh-an", "dest": "minh-an"}},
		"agents":  []map[string]string{{"source_id": "agent-minh", "source_tenant": "11111111-1111-4111-8111-111111111111", "dest_id": ag.ID}},
	})
	if err := os.WriteFile(mapPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	vaultDir := filepath.Join(dir, "vault")
	var stdout, stderr bytes.Buffer
	code := run([]string{"--export", exportPath, "--map", mapPath, "--db", dbPath, "--vault-dir", vaultDir, "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("dry-run %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SOUL.md") || strings.Contains(stdout.String(), "SYSBODY") || strings.Contains(stdout.String(), "hopdongx") {
		t.Fatalf("stdout %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--export", exportPath, "--map", mapPath, "--db", dbPath, "--vault-dir", vaultDir}, &stdout, &stderr); code != 0 {
		t.Fatalf("import %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"inserted": 2`) {
		t.Fatalf("first import %s", stdout.String())
	}
	stdout.Reset()
	if code := run([]string{"--export", exportPath, "--map", mapPath, "--db", dbPath, "--vault-dir", vaultDir}, &stdout, &stderr); code != 0 {
		t.Fatalf("second %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"inserted": 0`) {
		t.Fatalf("second import %s", stdout.String())
	}
	stdout.Reset()
	if code := run([]string{"--remove-tenant", "minh-an", "--db", dbPath, "--vault-dir", vaultDir}, &stdout, &stderr); code != 0 {
		t.Fatalf("remove %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"memories": 1`) || !strings.Contains(stdout.String(), `"vault_docs": 1`) {
		t.Fatalf("remove report %s", stdout.String())
	}
}
