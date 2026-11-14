// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/skill"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCallTool_SkillUsesAgentTenant(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_SKILLS_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	if _, err := skill.Create("", "bao-gia", "shared quote"); err != nil {
		t.Fatal(err)
	}
	if _, err := skill.Create("minh-an-sales", "bao-gia", "Cố vấn Minh An sales quote"); err != nil {
		t.Fatal(err)
	}
	if _, err := skill.Create("minh-an-legal", "bao-gia", "Cố vấn Minh An legal quote"); err != nil {
		t.Fatal(err)
	}

	st := store.New()
	sales, err := st.CreateAgent(store.Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An", TenantID: "minh-an-sales"})
	if err != nil {
		t.Fatal(err)
	}
	legal, err := st.CreateAgent(store.Agent{AgentKey: "minh-an-legal", DisplayName: "Cố vấn Minh An", TenantID: "minh-an-legal"})
	if err != nil {
		t.Fatal(err)
	}
	rt := New(st, connector.NewRegistry(), approval.New(0), eventstore.New(8), llm.Echo{})

	salesBody := skillToolBody(t, rt, sales.ID, "use_skill", map[string]any{"name": "bao-gia"})
	if !strings.Contains(salesBody, "sales quote") || strings.Contains(salesBody, "legal quote") {
		t.Fatalf("sales use_skill %s", salesBody)
	}
	legalBody := skillToolBody(t, rt, legal.ID, "use_skill", map[string]any{"name": "bao-gia"})
	if !strings.Contains(legalBody, "legal quote") || strings.Contains(legalBody, "sales quote") {
		t.Fatalf("legal use_skill %s", legalBody)
	}
	fallback := skillToolBody(t, rt, "", "use_skill", map[string]any{"name": "bao-gia"})
	if !strings.Contains(fallback, "shared quote") || strings.Contains(fallback, "sales quote") {
		t.Fatalf("no-agent fallback %s", fallback)
	}

	salesSearch := skillToolBody(t, rt, sales.ID, "skill_search", map[string]any{"query": "sales"})
	if !strings.Contains(salesSearch, `"name":"bao-gia"`) || strings.Contains(salesSearch, "legal quote") {
		t.Fatalf("sales skill_search %s", salesSearch)
	}
	legalSearch := skillToolBody(t, rt, legal.ID, "skill_search", map[string]any{"query": "legal"})
	if !strings.Contains(legalSearch, "legal quote") || strings.Contains(legalSearch, "sales quote") {
		t.Fatalf("legal skill_search %s", legalSearch)
	}
}

func skillToolBody(t *testing.T, rt *Runtime, agentID, tool string, args map[string]any) string {
	t.Helper()
	ctx := context.Background()
	if agentID != "" {
		ctx = WithCallMeta(ctx, agentID, "", "agent:"+agentID)
	}
	cr, err := rt.CallTool(ctx, "builtin", tool, args)
	if err != nil {
		t.Fatal(err)
	}
	if cr == nil || cr.Result == nil {
		t.Fatal("nil result")
	}
	raw, err := json.Marshal(cr.Result.Content)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
