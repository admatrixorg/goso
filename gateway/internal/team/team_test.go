// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package team

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestResolveMode(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "m1"})
	if ResolveMode(st, a.ID) != ModeManual {
		t.Fatalf("default manual")
	}
	b, _ := st.CreateAgent(store.Agent{AgentKey: "m2"})
	_ = st.AddAgentLink(a.ID, b.ID)
	if ResolveMode(st, a.ID) != ModeExplicit {
		t.Fatalf("links → explicit")
	}
	_, _ = st.CreateTeam(store.Team{Name: "T", LeadAgentID: a.ID})
	if ResolveMode(st, a.ID) != ModeAuto {
		t.Fatalf("team → auto")
	}
	c, _ := st.CreateAgent(store.Agent{AgentKey: "forced", OrchestrationMode: "manual"})
	_, _ = st.CreateTeam(store.Team{Name: "U", LeadAgentID: c.ID})
	if ResolveMode(st, c.ID) != ModeManual {
		t.Fatalf("persisted mode wins")
	}
}

func TestToolSpecsGating(t *testing.T) {
	if len(ToolSpecs(ModeManual)) != 0 {
		t.Fatalf("manual should advertise no team tools")
	}
	ex := ToolSpecs(ModeExplicit)
	if len(ex) != 1 || ex[0].Name != ToolDelegate {
		t.Fatalf("explicit %#v", ex)
	}
	auto := ToolSpecs(ModeAuto)
	names := map[string]bool{}
	for _, tspec := range auto {
		names[tspec.Name] = true
	}
	if !names[ToolSpawn] || !names[ToolDelegate] || !names[ToolTeamTasks] {
		t.Fatalf("auto %#v", auto)
	}
}

func TestDelegateSyncAsyncBidir(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "from"})
	b, _ := st.CreateAgent(store.Agent{AgentKey: "to"})
	chatted := make(chan string, 1)
	svc := &Service{Store: st, Chat: func(ctx context.Context, sessionID, message string) (string, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("sync delegate should set a deadline")
		}
		chatted <- message
		return "echo: " + message, nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := svc.Dispatch(ctx, a.ID, llm.ToolCall{
		Name: ToolDelegate,
		Arguments: map[string]any{
			"to_agent_id":   b.ID,
			"message":       "ping",
			"mode":          "sync",
			"bidirectional": true,
		},
	})
	if err != nil || !strings.Contains(out, "echo: ping") {
		t.Fatalf("sync %v %s", err, out)
	}
	if !st.HasAgentLink(a.ID, b.ID) || !st.HasAgentLink(b.ID, a.ID) {
		t.Fatal("bidirectional links missing")
	}
	select {
	case <-chatted:
	default:
		t.Fatal("sync did not wait for child chat")
	}

	async, err := svc.Dispatch(ctx, a.ID, llm.ToolCall{
		Name: ToolDelegate,
		Arguments: map[string]any{
			"to_agent_id": b.ID,
			"message":     "later",
			"mode":        "async",
		},
	})
	if err != nil || !strings.Contains(async, `"mode":"async"`) {
		t.Fatalf("async %v %s", err, async)
	}
	if !strings.Contains(async, `"id":`) {
		t.Fatalf("async missing id %s", async)
	}
}

func TestSpawnAndTeamTasks(t *testing.T) {
	st := store.New()
	lead, _ := st.CreateAgent(store.Agent{AgentKey: "lead", Model: "echo"})
	_, _ = st.CreateTeam(store.Team{Name: "Ops", LeadAgentID: lead.ID})
	svc := &Service{Store: st}
	out, err := svc.Dispatch(context.Background(), lead.ID, llm.ToolCall{Name: ToolSpawn})
	if err != nil || !strings.Contains(out, "agent_id") {
		t.Fatalf("spawn %v %s", err, out)
	}
	if len(st.ListAgents()) != 2 {
		t.Fatalf("expected clone")
	}
	created, err := svc.Dispatch(context.Background(), lead.ID, llm.ToolCall{
		Name:      ToolTeamTasks,
		Arguments: map[string]any{"action": "create", "title": "Board item", "status": "todo"},
	})
	if err != nil || !strings.Contains(created, "Board item") {
		t.Fatalf("create task %v %s", err, created)
	}
	listed, err := svc.Dispatch(context.Background(), lead.ID, llm.ToolCall{
		Name:      ToolTeamTasks,
		Arguments: map[string]any{"action": "list", "status": "todo"},
	})
	if err != nil || !strings.Contains(listed, "Board item") {
		t.Fatalf("list %v %s", err, listed)
	}
}

func TestNoteAndTEAMFile(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "n1", DisplayName: "Nia"})
	_, _ = st.CreateTeam(store.Team{Name: "Alpha", LeadAgentID: a.ID})
	note := Note(st, a.ID)
	if !strings.Contains(note, "Team: Alpha") || !strings.Contains(note, "Nia") {
		t.Fatalf("note %q", note)
	}
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_DIR", root)
	if err := os.WriteFile(filepath.Join(root, "TEAM.md"), []byte("stand-up at 9"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ReadTEAMFile(); got != "stand-up at 9" {
		t.Fatalf("TEAM.md %q", got)
	}
}

func TestEvolutionGuardrail(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "evo", DisplayName: "Evo"})
	st.RecordChatRun(a.ID)
	st.RecordChatRun(a.ID)
	st.RecordChatRun(a.ID)
	st.RecordToolUse(a.ID, "x", true)
	st.RecordToolUse(a.ID, "x", true)
	st.RecordAdvertisedTools(a.ID, []string{"never_used_tool"})
	sugs := Suggestions(st, a.ID)
	if len(sugs) == 0 {
		t.Fatal("expected suggestions")
	}
	for _, s := range sugs {
		if ForbiddenWrite(s.Text) || ForbiddenWrite(s.ID) || ForbiddenWrite(s.Rule) {
			t.Fatalf("suggestion names a protected field: %#v", s)
		}
	}
	_, err := Apply(st, a.ID, "rename-identity")
	if err == nil {
		t.Fatal("expected reject rename")
	}
	updated, err := Apply(st, a.ID, RuleHighToolError)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "Evo" || updated.AgentKey != "evo" {
		t.Fatalf("identity changed %#v", updated)
	}
	if !strings.Contains(updated.Instructions, PrefixHighError) {
		t.Fatalf("prefix missing %q", updated.Instructions)
	}
}
