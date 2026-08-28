// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package team

import (
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func seedHighError(t *testing.T, st store.StoreIface, agentID string, runs int) {
	t.Helper()
	for i := 0; i < runs; i++ {
		st.RecordChatRun(agentID)
	}
	for i := 0; i < runs; i++ {
		st.RecordToolUse(agentID, "x", true)
	}
	st.RecordAdvertisedTools(agentID, []string{"idle_tool"})
}

func TestTick_AutoAdaptOffNoop(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "off", DisplayName: "Off", Instructions: "base"})
	seedHighError(t, st, a.ID, 20)
	res, err := Tick(st, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "noop" || res.Reason != "auto_adapt_off" {
		t.Fatalf("want noop auto_adapt_off got %#v", res)
	}
	got, _ := st.GetAgent(a.ID)
	if got.Instructions != "base" {
		t.Fatalf("instructions changed %q", got.Instructions)
	}
	if res.Guardrails.AutoAdapt {
		t.Fatal("default auto_adapt must be false")
	}
	if res.Guardrails.MinRuns != store.DefaultMinRuns {
		t.Fatalf("min_runs %d", res.Guardrails.MinRuns)
	}
}

func TestTick_AppliesFirstUnlockedPending(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "on", DisplayName: "On", Instructions: "base"})
	seedHighError(t, st, a.ID, 5)
	if err := st.PutEvolutionGuardrails(a.ID, store.EvolutionGuardrails{AutoAdapt: true, MinRuns: 5}); err != nil {
		t.Fatal(err)
	}
	res, err := Tick(st, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "applied" || res.SuggestionID != RuleHighToolError {
		t.Fatalf("want applied high-tool-error got %#v", res)
	}
	if res.Agent == nil || !strings.Contains(res.Agent.Instructions, PrefixHighError) {
		t.Fatalf("instructions %#v", res.Agent)
	}
	if res.Agent.DisplayName != "On" || res.Agent.AgentKey != "on" {
		t.Fatalf("identity %#v", res.Agent)
	}
}

func TestTick_RollbackOnErrorRateDrop(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "rb", DisplayName: "Rb", Instructions: "base"})
	seedHighError(t, st, a.ID, 5)
	_ = st.PutEvolutionGuardrails(a.ID, store.EvolutionGuardrails{AutoAdapt: true, MinRuns: 5})
	first, err := Tick(st, a.ID)
	if err != nil || first.Action != "applied" {
		t.Fatalf("apply %#v %v", first, err)
	}
	st.RecordToolUse(a.ID, "x", true)
	st.RecordToolUse(a.ID, "x", true)
	second, err := Tick(st, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Action != "rolled_back" {
		t.Fatalf("want rolled_back got %#v", second)
	}
	if second.Agent == nil || second.Agent.Instructions != "base" {
		t.Fatalf("rollback instructions %#v", second.Agent)
	}
}

func TestTick_CannotUnlockNameKey(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "lock", DisplayName: "Lock"})
	if err := st.PutEvolutionGuardrails(a.ID, store.EvolutionGuardrails{
		AutoAdapt: true, MinRuns: 3, Locked: nil,
	}); err != nil {
		t.Fatal(err)
	}
	g := st.GetEvolutionGuardrails(a.ID)
	for _, need := range []string{"display_name", "agent_key"} {
		found := false
		for _, k := range g.Locked {
			if k == need {
				found = true
			}
		}
		if !found {
			t.Fatalf("locked missing %s %#v", need, g.Locked)
		}
	}
}

func TestApply_NameStillRejected(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "nm", DisplayName: "Nm"})
	_, err := Apply(st, a.ID, "display_name")
	if err == nil {
		t.Fatal("expected reject display_name")
	}
}

func TestAutoEnabled_DefaultOff(t *testing.T) {
	t.Setenv("GOSO_EVOLUTION_AUTO", "")
	if AutoEnabled() {
		t.Fatal("default must be off")
	}
	t.Setenv("GOSO_EVOLUTION_AUTO", "1")
	if !AutoEnabled() {
		t.Fatal("want on")
	}
}

func TestTickAll_RespectsAutoAdapt(t *testing.T) {
	st := store.New()
	off, _ := st.CreateAgent(store.Agent{AgentKey: "aoff", Instructions: "keep"})
	on, _ := st.CreateAgent(store.Agent{AgentKey: "aon", Instructions: "keep"})
	seedHighError(t, st, off.ID, 5)
	seedHighError(t, st, on.ID, 5)
	_ = st.PutEvolutionGuardrails(on.ID, store.EvolutionGuardrails{AutoAdapt: true, MinRuns: 5})
	TickAll(st)
	gotOff, _ := st.GetAgent(off.ID)
	gotOn, _ := st.GetAgent(on.ID)
	if gotOff.Instructions != "keep" {
		t.Fatalf("off changed %q", gotOff.Instructions)
	}
	if !strings.Contains(gotOn.Instructions, PrefixHighError) {
		t.Fatalf("on unchanged %q", gotOn.Instructions)
	}
}
