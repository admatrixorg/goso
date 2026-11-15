// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

// vaultRoot resolves t.TempDir through symlinks. On macOS the temp dir sits
// under /var, a link to /private/var, and the vault refuses a root it cannot
// match after resolving.
func vaultRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func vaultFixture(t *testing.T) (store.StoreIface, string, *store.VaultDoc, *store.VaultDoc) {
	t.Helper()
	st := store.New()
	root := vaultRoot(t)
	svc := vault.New(st, root)
	mine, err := svc.PutTenant("Ngân sách quảng cáo", "Quy tắc chia ngân sách Meta theo phễu.", "acme")
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := svc.PutTenant("Ngân sách bí mật", "Ngân sách nội bộ của tenant khác.", "other")
	if err != nil {
		t.Fatal(err)
	}
	return st, root, mine, theirs
}

func TestDispatchVaultTool_SearchStaysInTenant(t *testing.T) {
	st, root, mine, theirs := vaultFixture(t)
	body, err := DispatchVaultTool(st, root, "acme", "", llm.ToolCall{
		Name: ToolVaultSearch, Arguments: map[string]any{"query": "ngân sách"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Hits []store.VaultSearchHit `json:"hits"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Hits) != 1 || out.Hits[0].ID != mine.ID {
		t.Fatalf("hits %s", body)
	}
	if strings.Contains(body, theirs.ID) || strings.Contains(body, "bí mật") {
		t.Fatalf("leaked other tenant %s", body)
	}
}

func TestDispatchVaultTool_SearchNeedsQuery(t *testing.T) {
	st, root, _, _ := vaultFixture(t)
	if _, err := DispatchVaultTool(st, root, "acme", "", llm.ToolCall{Name: ToolVaultSearch}); err == nil {
		t.Fatal("empty query must fail")
	}
}

func TestDispatchVaultTool_ReadByIDAndPath(t *testing.T) {
	st, root, mine, _ := vaultFixture(t)
	for _, args := range []map[string]any{{"id": mine.ID}, {"path": mine.Path}} {
		body, err := DispatchVaultTool(st, root, "acme", "", llm.ToolCall{Name: ToolVaultRead, Arguments: args})
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		var got vaultReadResult
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Fatal(err)
		}
		if got.ID != mine.ID || !strings.Contains(got.Body, "phễu") || got.Truncated {
			t.Fatalf("%v: %s", args, body)
		}
	}
}

func TestDispatchVaultTool_ReadOtherTenantIsNotFound(t *testing.T) {
	st, root, _, theirs := vaultFixture(t)
	for _, args := range []map[string]any{{"id": theirs.ID}, {"path": theirs.Path}} {
		body, err := DispatchVaultTool(st, root, "acme", "", llm.ToolCall{Name: ToolVaultRead, Arguments: args})
		if !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("%v: err %v body %s", args, err, body)
		}
		if strings.Contains(body, "nội bộ") {
			t.Fatalf("leaked body %s", body)
		}
	}
}

func TestDispatchVaultTool_ReadNeedsIDOrPath(t *testing.T) {
	st, root, _, _ := vaultFixture(t)
	if _, err := DispatchVaultTool(st, root, "acme", "", llm.ToolCall{Name: ToolVaultRead}); err == nil {
		t.Fatal("missing id and path must fail")
	}
}

func TestDispatchVaultTool_ChannelCallerRefused(t *testing.T) {
	st, root, mine, _ := vaultFixture(t)
	ag, err := st.CreateAgent(store.Agent{AgentKey: "bot", TenantID: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: ag.ID, TenantID: "acme", Label: "telegram:42"})
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []llm.ToolCall{
		{Name: ToolVaultSearch, Arguments: map[string]any{"query": "ngân sách"}},
		{Name: ToolVaultRead, Arguments: map[string]any{"id": mine.ID}},
	} {
		body, err := DispatchVaultTool(st, root, "acme", sess.ID, call)
		if !errors.Is(err, errVaultChannel) || strings.Contains(body, "phễu") || strings.Contains(body, mine.ID) {
			t.Fatalf("%s: err %v body %s", call.Name, err, body)
		}
	}
}

func TestDispatchVaultTool_ReadCapsLargeBody(t *testing.T) {
	st := store.New()
	root := vaultRoot(t)
	big := strings.Repeat("ấ", VaultReadMaxBytes) // 3 bytes per rune
	doc, err := vault.New(st, root).PutTenant("Tài liệu dài", big, "acme")
	if err != nil {
		t.Fatal(err)
	}
	body, err := DispatchVaultTool(st, root, "acme", "", llm.ToolCall{
		Name: ToolVaultRead, Arguments: map[string]any{"id": doc.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got vaultReadResult
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.Body) > VaultReadMaxBytes || !utf8.ValidString(got.Body) {
		t.Fatalf("truncated %v len %d valid %v", got.Truncated, len(got.Body), utf8.ValidString(got.Body))
	}
}

func TestIsVaultTool(t *testing.T) {
	for _, n := range []string{"vault_search", "vault_read", "x__vault_read"} {
		if !IsVaultTool(n) {
			t.Fatalf("%s", n)
		}
	}
	if IsVaultTool("memory_search") {
		t.Fatal("memory_search is not a vault tool")
	}
}
