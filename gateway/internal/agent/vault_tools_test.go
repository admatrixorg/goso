// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

func TestRuntimeTools_VaultUsesAgentTenant(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_VAULT_DIR", root)

	st := store.New()
	svc := vault.New(st, root)
	if _, err := svc.PutTenant("Ngân sách Meta", "sales-plan chia ngân sách theo phễu", "minh-an-sales"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PutTenant("Ngân sách pháp lý", "legal-plan ngân sách hợp đồng", "minh-an-legal"); err != nil {
		t.Fatal(err)
	}
	sales, err := st.CreateAgent(store.Agent{AgentKey: "minh-an-sales", TenantID: "minh-an-sales"})
	if err != nil {
		t.Fatal(err)
	}
	legal, err := st.CreateAgent(store.Agent{AgentKey: "minh-an-legal", TenantID: "minh-an-legal"})
	if err != nil {
		t.Fatal(err)
	}
	rt := New(st, connector.NewRegistry(), approval.New(0), eventstore.New(8), llm.Echo{})
	tools := runtimeTools{rt: rt}

	specs, err := tools.List(context.Background(), sales.ID)
	if err != nil {
		t.Fatal(err)
	}
	advertised := map[string]bool{}
	for _, s := range specs {
		advertised[s.Name] = true
	}
	if !advertised[pipeline.ToolVaultSearch] || !advertised[pipeline.ToolVaultRead] {
		t.Fatalf("vault tools not advertised: %v", advertised)
	}

	for _, tc := range []struct {
		agent, want, deny string
	}{
		{sales.ID, "sales-plan", "legal-plan"},
		{legal.ID, "legal-plan", "sales-plan"},
	} {
		out, err := tools.Call(context.Background(), tc.agent, llm.ToolCall{
			Name: pipeline.ToolVaultSearch, Arguments: map[string]any{"query": "ngân sách"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.Content, tc.want) || strings.Contains(out.Content, tc.deny) {
			t.Fatalf("agent %s search %s", tc.agent, out.Content)
		}
	}
}
