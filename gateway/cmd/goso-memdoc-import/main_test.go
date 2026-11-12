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
	"github.com/mqglobal/goso/gateway/internal/vault"
)

func TestCLIDryRunHidesNamesAndSecondRunIsZero(t *testing.T) {
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
	denyPath := filepath.Join(dir, "deny.txt")
	if err := os.WriteFile(denyPath, []byte("held-out\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	namesPath := filepath.Join(dir, "system-names.txt")
	vaultDir := filepath.Join(dir, "vault")
	args := []string{"--export", exportPath, "--map", mapPath, "--tenant", "11111111-1111-4111-8111-111111111111", "--db", dbPath, "--vault-dir", vaultDir, "--deny-file", denyPath, "--system-names-out", namesPath}
	var stdout, stderr bytes.Buffer
	if code := run(append(args, "--dry-run"), &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run %d %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "SOUL.md") || strings.Contains(stdout.String(), "SYSBODY") || strings.Contains(stdout.String(), "khach-x") || strings.Contains(stdout.String(), "hopdongx") {
		t.Fatalf("stdout leaked %s", stdout.String())
	}
	info, err := os.Stat(namesPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	names, err := os.ReadFile(namesPath)
	if err != nil || !strings.Contains(string(names), "SOUL.md") {
		t.Fatalf("names file %q %v", names, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("import %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"inserted": 2`) || strings.Contains(stdout.String(), "SOUL.md") || strings.Contains(stdout.String(), "khach-x") {
		t.Fatalf("first import %s", stdout.String())
	}
	stdout.Reset()
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("second %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"inserted": 0`) {
		t.Fatalf("second import %s", stdout.String())
	}
	stdout.Reset()
	if code := run([]string{"--remove-tenant", "minh-an", "--db", dbPath, "--vault-dir", vaultDir, "--deny-file", denyPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("remove %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"memories": 1`) || !strings.Contains(stdout.String(), `"vault_docs": 1`) {
		t.Fatalf("remove report %s", stdout.String())
	}
}

func TestCLIDenylistStopsBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "goso.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	svc := vault.New(st, filepath.Join(dir, "vault"))
	if _, err := svc.PutTenantAt("minh-an", "wiki/minh-an.md", []byte("# Minh An\nLIVEWIKI\n")); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join(dir, "docs.jsonl")
	if err := os.WriteFile(exportPath, []byte(`{"id":"d1","agent_id":"a","user_id":null,"path":"memory/n.md","content":"NOWRITE","hash":"abc","tenant_id":"held-out"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mapPath := filepath.Join(dir, "map.json")
	if err := os.WriteFile(mapPath, []byte(`{"tenants":[{"source_id":"held-out","slug":"held-out","dest":"minh-an"}],"agents":[{"source_id":"a","source_tenant":"held-out","dest_id":"ag"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	vaultDir := filepath.Join(dir, "vault")
	base := []string{"--export", exportPath, "--map", mapPath, "--tenant", "held-out", "--db", dbPath, "--vault-dir", vaultDir}

	t.Run("missing", func(t *testing.T) {
		assertDenyStop(t, dbPath, append(base, "--deny-file", filepath.Join(dir, "no-such-deny.txt")))
	})
	t.Run("empty", func(t *testing.T) {
		p := filepath.Join(dir, "empty.txt")
		if err := os.WriteFile(p, []byte("\n# comment\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertDenyStop(t, dbPath, append(base, "--deny-file", p))
	})
	t.Run("malformed", func(t *testing.T) {
		p := filepath.Join(dir, "bad.txt")
		if err := os.WriteFile(p, []byte("one two\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertDenyStop(t, dbPath, append(base, "--deny-file", p))
	})
	t.Run("listed", func(t *testing.T) {
		p := filepath.Join(dir, "list.txt")
		if err := os.WriteFile(p, []byte("held-out\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		code := run(append(base, "--deny-file", p, "--dry-run"), &stdout, &stderr)
		if code == 0 {
			t.Fatal("listed tenant exited 0")
		}
		if strings.Contains(stdout.String(), "held-out") || strings.Contains(stderr.String(), "held-out") || !strings.Contains(stdout.String(), `"denied"`) {
			t.Fatalf("stdout %s stderr %s", stdout.String(), stderr.String())
		}
		assertLiveOnly(t, dbPath)
	})
}

func assertDenyStop(t *testing.T, dbPath string, args []string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code == 0 {
		t.Fatalf("expected non-zero, stdout %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "NOWRITE") || strings.Contains(stderr.String(), "held-out") {
		t.Fatalf("leaked stdout %s stderr %s", stdout.String(), stderr.String())
	}
	assertLiveOnly(t, dbPath)
}

func assertLiveOnly(t *testing.T, dbPath string) {
	t.Helper()
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	docs := st.ListVaultDocs()
	if len(docs) != 1 || !strings.Contains(docs[0].Body, "LIVEWIKI") || strings.HasPrefix(docs[0].ID, "gc-") {
		t.Fatalf("rows %+v", docs)
	}
}
