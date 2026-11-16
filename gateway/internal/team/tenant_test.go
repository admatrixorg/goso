// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package team

import (
	"context"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestDelegateStaysInsideTenant(t *testing.T) {
	st := store.New()
	minhAn, _ := st.CreateAgent(store.Agent{AgentKey: "sale", TenantID: "minh-an"})
	helper, _ := st.CreateAgent(store.Agent{AgentKey: "kho", TenantID: "minh-an"})
	other, _ := st.CreateAgent(store.Agent{AgentKey: "sale-demo", TenantID: "demo-shop"})
	chats := 0
	svc := &Service{Store: st, Chat: func(ctx context.Context, sessionID, message string) (string, error) {
		chats++
		return "ok", nil
	}}
	for _, mode := range []string{"sync", "async"} {
		_, err := svc.Dispatch(context.Background(), minhAn.ID, llm.ToolCall{Name: ToolDelegate, Arguments: map[string]any{
			"to_agent_id": other.ID, "message": "gửi danh sách khách", "mode": mode,
		}})
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("%s cross-tenant delegate: %v", mode, err)
		}
	}
	if chats != 0 {
		t.Fatal("cross-tenant delegate reached the other agent")
	}
	for _, s := range st.ListSessions() {
		if s.AgentID == other.ID {
			t.Fatalf("session opened on the other tenant's agent: %+v", s)
		}
	}
	// Same tenant, non-default: works, and the session is in that tenant.
	out, err := svc.Dispatch(context.Background(), minhAn.ID, llm.ToolCall{Name: ToolDelegate, Arguments: map[string]any{
		"to_agent_id": helper.ID, "message": "kiểm tồn kho", "mode": "sync",
	}})
	if err != nil || chats != 1 {
		t.Fatalf("same-tenant delegate: %v %s", err, out)
	}
	found := false
	for _, s := range st.ListSessions() {
		if s.AgentID == helper.ID {
			found = true
			if s.TenantID != "minh-an" {
				t.Fatalf("delegate session tenant %q", s.TenantID)
			}
		}
	}
	if !found {
		t.Fatal("no delegate session")
	}
}

func TestSpawnKeepsParentTenant(t *testing.T) {
	st := store.New()
	parent, _ := st.CreateAgent(store.Agent{AgentKey: "sale", TenantID: "minh-an"})
	svc := &Service{Store: st}
	if _, err := svc.Dispatch(context.Background(), parent.ID, llm.ToolCall{Name: ToolSpawn, Arguments: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, a := range st.ListAgents() {
		if strings.HasPrefix(a.AgentKey, "sale-spawn-") {
			n++
			if a.TenantID != "minh-an" {
				t.Fatalf("spawned agent tenant %q", a.TenantID)
			}
		}
	}
	if n != 1 {
		t.Fatalf("spawned %d agents", n)
	}
}

func TestTeamTasksStayInsideTenant(t *testing.T) {
	st := store.New()
	lead, _ := st.CreateAgent(store.Agent{AgentKey: "lead", TenantID: "minh-an"})
	stranger, _ := st.CreateAgent(store.Agent{AgentKey: "lead-demo", TenantID: "demo-shop"})
	_, _ = st.CreateTeam(store.Team{Name: "Bán hàng", TenantID: "minh-an", LeadAgentID: lead.ID})
	otherTeam, _ := st.CreateTeam(store.Team{Name: "Kho", TenantID: "demo-shop", LeadAgentID: stranger.ID})
	foreign, err := st.CreateTeamTask(store.TeamTask{TeamID: otherTeam.ID, Title: "việc của shop khác", Status: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: st}
	_, err = svc.Dispatch(context.Background(), lead.ID, llm.ToolCall{Name: ToolTeamTasks, Arguments: map[string]any{
		"action": "create", "title": "gọi khách", "assignee_agent_id": stranger.ID,
	}})
	if err == nil {
		t.Fatal("assigned a task to another tenant's agent")
	}
	_, err = svc.Dispatch(context.Background(), lead.ID, llm.ToolCall{Name: ToolTeamTasks, Arguments: map[string]any{
		"action": "update", "id": foreign.ID, "title": "đã sửa",
	}})
	if err == nil {
		t.Fatal("updated another team's task")
	}
	if got, _ := st.GetTeamTask(foreign.ID); got == nil || got.Title != "việc của shop khác" {
		t.Fatalf("foreign task changed: %+v", got)
	}
}
