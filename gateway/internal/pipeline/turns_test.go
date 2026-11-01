// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// addToolTurn stores one exchange of a tool-using agent: the user line,
// toolsPerTurn tool-call requests each followed by its result, an event row,
// then the final assistant reply.
func addToolTurn(t *testing.T, st store.StoreIface, sid string, i, toolsPerTurn int) {
	t.Helper()
	add := func(role, content string) {
		if _, err := st.AddMessage(store.Message{SessionID: sid, Role: role, Content: content}); err != nil {
			t.Fatal(err)
		}
	}
	add("user", fmt.Sprintf("câu hỏi %d về đơn SOFA-7731", i))
	for k := 0; k < toolsPerTurn; k++ {
		id := fmt.Sprintf("call-%d-%d", i, k)
		add("assistant", EncodeAssistant("", []llm.ToolCall{{ID: id, Name: "lookup_order"}}))
		add("tool", EncodeTool(id, `{"order":"SOFA-7731","color":"walnut"}`))
	}
	add("event", "tool_done")
	add("assistant", fmt.Sprintf("dạ trả lời %d", i))
}

func TestConversationTurns_IgnoresToolAndEventRows(t *testing.T) {
	st := store.New()
	ag, _ := st.CreateAgent(store.Agent{AgentKey: "turns"})
	s, _ := st.CreateSession(store.Session{AgentID: ag.ID})
	for i := 0; i < 3; i++ {
		addToolTurn(t, st, s.ID, i, 3)
	}
	raw, _ := st.ListMessages(s.ID)
	if len(raw) < SummarizeMessageMin {
		t.Fatalf("fixture must exceed the raw threshold, rows=%d", len(raw))
	}
	if n := ConversationTurns(raw); n != 6 {
		t.Fatalf("turns=%d want 6 (3 user + 3 final assistant), rows=%d", n, len(raw))
	}
}

// Bug: tool rows counted toward SummarizeMessageMin, so a tool-using agent
// summarised after 2-3 exchanges and the idle flush then saw the session as
// covered. Only real user/assistant turns count now.
func TestSummarizeStage_ToolRowsDoNotHitThreshold(t *testing.T) {
	for name, st := range idleStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, _ := st.CreateAgent(store.Agent{AgentKey: "tools-" + name})
			s, _ := st.CreateSession(store.Session{AgentID: ag.ID})
			stage := SummarizeStage(st)
			for i := 0; i < 3; i++ {
				addToolTurn(t, st, s.ID, i, 3)
				if err := stage(context.Background(), &State{SessionID: s.ID}); err != nil {
					t.Fatal(err)
				}
			}
			raw, _ := st.ListMessages(s.ID)
			if sum, _ := st.LatestSummary(s.ID); sum != nil {
				t.Fatalf("3 tool-using turns (%d rows) must not summarise: %+v", len(raw), sum)
			}

			// The idle flush still owns this session and writes its note once.
			clk := &fakeClock{t: time.Now().UTC().Add(11 * time.Minute)}
			f := newFlusher(st, clk, nil)
			if n := f.Sweep(context.Background()); n != 1 {
				t.Fatalf("idle flush wrote %d, want 1", n)
			}
			sum, _ := st.LatestSummary(s.ID)
			if sum == nil || !strings.Contains(sum.Body, "SOFA-7731") || strings.Contains(sum.Body, "walnut") {
				t.Fatalf("idle note must hold the user lines, not tool output: %+v", sum)
			}
			if n := f.Sweep(context.Background()); n != 0 {
				t.Fatalf("duplicate flush %d", n)
			}

			// A trailing tool/event row is not a new turn: still covered.
			time.Sleep(2 * time.Millisecond)
			_, _ = st.AddMessage(store.Message{SessionID: s.ID, Role: "tool", Content: EncodeTool("late", "x")})
			_, _ = st.AddMessage(store.Message{SessionID: s.ID, Role: "event", Content: "late"})
			clk.t = time.Now().UTC().Add(11 * time.Minute)
			if n := newFlusher(st, clk, nil).Sweep(context.Background()); n != 0 {
				t.Fatalf("tool/event rows after the note must not re-flush, wrote %d", n)
			}
		})
	}
}

func TestSummarizeStage_TwelveRealTurnsStillSummarise(t *testing.T) {
	for name, st := range idleStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, _ := st.CreateAgent(store.Agent{AgentKey: "real-" + name})
			s, _ := st.CreateSession(store.Session{AgentID: ag.ID})
			stage := SummarizeStage(st)
			for i := 0; i < SummarizeMessageMin; i++ {
				role := "user"
				if i%2 == 1 {
					role = "assistant"
				}
				_, _ = st.AddMessage(store.Message{SessionID: s.ID, Role: role, Content: fmt.Sprintf("dòng %d KIWI-4821", i)})
				if err := stage(context.Background(), &State{SessionID: s.ID}); err != nil {
					t.Fatal(err)
				}
				sum, _ := st.LatestSummary(s.ID)
				if i < SummarizeMessageMin-1 && sum != nil {
					t.Fatalf("summarised early at turn %d", i+1)
				}
				if i == SummarizeMessageMin-1 && (sum == nil || !strings.Contains(sum.Body, "KIWI-4821")) {
					t.Fatalf("12 real turns must summarise: %+v", sum)
				}
			}
		})
	}
}

// Same 12-turn threshold with tool rows mixed in: 12 real turns hit it.
func TestSummarizeStage_TwelveTurnsWithToolsHitThreshold(t *testing.T) {
	st := store.New()
	ag, _ := st.CreateAgent(store.Agent{AgentKey: "mix"})
	s, _ := st.CreateSession(store.Session{AgentID: ag.ID})
	stage := SummarizeStage(st)
	for i := 0; i < 5; i++ {
		addToolTurn(t, st, s.ID, i, 2)
		_ = stage(context.Background(), &State{SessionID: s.ID})
	}
	if sum, _ := st.LatestSummary(s.ID); sum != nil {
		t.Fatal("10 turns must not summarise")
	}
	addToolTurn(t, st, s.ID, 5, 2)
	_ = stage(context.Background(), &State{SessionID: s.ID})
	if sum, _ := st.LatestSummary(s.ID); sum == nil {
		t.Fatal("12 turns must summarise")
	}
}

// The first idle sweep stays off the prune's 60 s start.
func TestIdleFirstSweepAvoidsPruneStart(t *testing.T) {
	if idleFirstSweepDelay == pruneStartDelay {
		t.Fatalf("idle first sweep %s collides with prune start %s", idleFirstSweepDelay, pruneStartDelay)
	}
	if (idleFirstSweepDelay-pruneStartDelay)%idleSweepEvery == 0 {
		t.Fatalf("idle sweeps (%s + k*%s) land on the prune start %s", idleFirstSweepDelay, idleSweepEvery, pruneStartDelay)
	}
}
