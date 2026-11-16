// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package team

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func delegateTo(svc *Service, from, to string) (string, error) {
	return svc.Dispatch(context.Background(), from, llm.ToolCall{Name: ToolDelegate, Arguments: map[string]any{
		"to_agent_id": to, "message": "Minh An hỏi giá", "mode": "sync",
	}})
}

func linkRule(t *testing.T, st store.StoreIface) {
	t.Helper()
	sale, _ := st.CreateAgent(store.Agent{AgentKey: "sale", TenantID: "minh-an"})
	kho, _ := st.CreateAgent(store.Agent{AgentKey: "kho", TenantID: "minh-an"})
	ketoan, _ := st.CreateAgent(store.Agent{AgentKey: "ke-toan", TenantID: "minh-an"})
	other, _ := st.CreateAgent(store.Agent{AgentKey: "kho", TenantID: "demo-shop"})
	svc := &Service{Store: st, Chat: func(ctx context.Context, sessionID, message string) (string, error) {
		return "ok", nil
	}}

	// No links: any agent of the tenant, by id or by agent_key.
	if _, err := delegateTo(svc, sale.ID, ketoan.ID); err != nil {
		t.Fatalf("no links, same tenant: %v", err)
	}
	if _, err := delegateTo(svc, sale.ID, "kho"); err != nil {
		t.Fatalf("no links, by key: %v", err)
	}
	if _, err := delegateTo(svc, sale.ID, other.ID); err == nil {
		t.Fatal("no links, other tenant allowed")
	}
	if links, _ := st.ListAgentLinks(sale.ID); len(links) != 0 {
		t.Fatalf("delegate created links %+v", links)
	}

	// With a link: only linked agents.
	if err := st.AddAgentLink(sale.ID, kho.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := delegateTo(svc, sale.ID, kho.ID); err != nil {
		t.Fatalf("linked target: %v", err)
	}
	if _, err := delegateTo(svc, sale.ID, "kho"); err != nil {
		t.Fatalf("linked target by key resolves inside the tenant: %v", err)
	}
	if _, err := delegateTo(svc, sale.ID, ketoan.ID); err == nil || !strings.Contains(err.Error(), "not linked") {
		t.Fatalf("unlinked target: %v", err)
	}
	// Links are one-way: kho has none of its own, so it may reach anyone.
	if _, err := delegateTo(svc, kho.ID, ketoan.ID); err != nil {
		t.Fatalf("agent without links: %v", err)
	}

	// Cross-tenant links are refused at create time.
	if err := st.AddAgentLink(sale.ID, other.ID); !errors.Is(err, store.ErrCrossTenantLink) {
		t.Fatalf("cross-tenant link: %v", err)
	}
	if st.HasAgentLink(sale.ID, other.ID) {
		t.Fatal("cross-tenant link stored")
	}
}

func TestDelegateFollowsLinksMemory(t *testing.T) {
	linkRule(t, store.New())
}

func TestDelegateFollowsLinksSQLite(t *testing.T) {
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "links.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	linkRule(t, st)
}
