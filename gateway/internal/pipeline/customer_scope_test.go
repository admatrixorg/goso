// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Two customers on one Zalo agent. A's 2-message session is written by the
// idle flush and recalled only in A's new session; B's new session gets
// nothing, neither does it get legacy/web notes. A web session on the same
// agent still recalls agent-wide. No channel adapter runs: sessions are
// created with channel labels, so nothing is sent to Zalo.
func TestCustomerScope_ZaloCustomersIdleFlushAndRecall(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "")
	for name, st := range idleStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, _ := st.CreateAgent(store.Agent{AgentKey: "zalo-sale-" + name})
			a1, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "zalo-personal:CUST-A"})
			web0, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "web"})
			// Unscoped note from a web session (same shape as a legacy row).
			if _, err := st.SaveSummary(web0.ID, "ghi chú web: đơn SOFA-7731 giao quận 7"); err != nil {
				t.Fatal(err)
			}

			chat := &llm.Scripted{Replies: []llm.Reply{{Text: "dạ em ghi nhận"}}, RepeatLast: true}
			r := NewRunner(st, nil, chat, nil)
			if _, err := r.Run(context.Background(), a1.ID, "Đơn SOFA-7731 sofa gỗ sồi màu walnut, giao 15/11", ModeFull); err != nil {
				t.Fatal(err)
			}
			if msgs, _ := st.ListMessages(a1.ID); len(msgs) != 2 {
				t.Fatalf("want a 2-message channel session, got %d", len(msgs))
			}

			clk := &fakeClock{t: time.Now().UTC().Add(11 * time.Minute)}
			if n := newFlusher(st, clk, nil).Sweep(context.Background()); n != 1 {
				t.Fatalf("idle flush must write the channel session, wrote %d", n)
			}
			sum, err := st.LatestSummary(a1.ID)
			if err != nil || sum == nil || !strings.Contains(sum.Body, "walnut") || sum.CustomerKey != "zalo-personal:CUST-A" {
				t.Fatalf("channel note: %+v %v", sum, err)
			}

			ask := "Đơn SOFA-7731 sofa màu gì, giao ngày nào?"
			last := func() string { return recallNoteIn(chat.Recorded[len(chat.Recorded)-1]) }

			a2, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "zalo-personal:CUST-A"})
			if _, err := r.Run(context.Background(), a2.ID, ask, ModeFull); err != nil {
				t.Fatal(err)
			}
			note := last()
			if !strings.Contains(note, "walnut") {
				t.Fatalf("A's new session must recall A's note: %q", note)
			}
			if strings.Contains(note, "quận 7") {
				t.Fatalf("unscoped web note leaked into a channel session: %q", note)
			}

			b1, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "zalo-personal:CUST-B"})
			if _, err := r.Run(context.Background(), b1.ID, ask, ModeFull); err != nil {
				t.Fatal(err)
			}
			if note := last(); note != "" {
				t.Fatalf("B must recall nothing from A or web: %q", note)
			}

			web1, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "web 2"})
			if _, err := r.Run(context.Background(), web1.ID, ask, ModeFull); err != nil {
				t.Fatal(err)
			}
			if note := last(); !strings.Contains(note, "walnut") || !strings.Contains(note, "quận 7") {
				t.Fatalf("web session recalls agent-wide as before: %q", note)
			}
		})
	}
}

// The 12-message threshold and summarize=1 paths write through SaveSummary,
// so their notes carry the customer key too and stay out of other customers.
func TestCustomerScope_SummarizeStageStampsKey(t *testing.T) {
	for name, st := range idleStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, _ := st.CreateAgent(store.Agent{AgentKey: "zs-" + name})
			s, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "zalo-oa:U1"})
			for i := 0; i < SummarizeMessageMin; i++ {
				_, _ = st.AddMessage(store.Message{SessionID: s.ID, Role: "user", Content: "tin nhắn đơn KIWI-4821"})
			}
			if err := SummarizeStage(st)(context.Background(), &State{SessionID: s.ID, AgentID: ag.ID, Mode: ModeFull}); err != nil {
				t.Fatal(err)
			}
			f, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "telegram:42"})
			_, _ = st.AddMessage(store.Message{SessionID: f.ID, Role: "user", Content: "đơn KIWI-4821 telegram"})
			if err := SummarizeStage(st)(context.Background(), &State{SessionID: f.ID, AgentID: ag.ID, Mode: ModeFull, ForceSummarize: true}); err != nil {
				t.Fatal(err)
			}
			for sid, key := range map[string]string{s.ID: "zalo-oa:U1", f.ID: "telegram:42"} {
				sum, _ := st.LatestSummary(sid)
				if sum == nil || sum.CustomerKey != key {
					t.Fatalf("summary of %s: %+v want key %s", sid, sum, key)
				}
			}
		})
	}
}

// memory_search / memory_expand / sessions_* called from a channel session see
// only their own customer; web callers keep tenant-wide visibility.
func TestCustomerScope_Tools(t *testing.T) {
	for name, st := range idleStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, _ := st.CreateAgent(store.Agent{AgentKey: "zt-" + name})
			a, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "zalo-personal:A"})
			b, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "zalo-personal:B"})
			web, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "web"})
			_, _ = st.PutMemory(store.Memory{SessionID: a.ID, Kind: store.KindDurable, Body: "PINEAPPLE-9 của khách A"})
			_, _ = st.AddMessage(store.Message{SessionID: a.ID, Role: "user", Content: "PINEAPPLE-9 tin nhắn A"})
			_, _ = st.PutMemory(store.Memory{SessionID: web.ID, Kind: store.KindDurable, Body: "PINEAPPLE-9 web note"})
			ext, err := st.PutKGEntity(store.KGEntity{Name: "PINEAPPLE-9 extracted", Kind: "fact", Source: store.KGSourceExtracted, AgentID: ag.ID})
			if err != nil {
				t.Fatal(err)
			}
			cur, err := st.PutKGEntity(store.KGEntity{Name: "PINEAPPLE-9 catalog", Kind: "product"})
			if err != nil {
				t.Fatal(err)
			}

			search := func(caller string) []store.KGSearchHit {
				t.Helper()
				body, err := DispatchMemoryTool(st, store.DefaultTenant, caller, llm.ToolCall{Name: ToolMemorySearch, Arguments: map[string]any{"query": "PINEAPPLE-9"}})
				if err != nil {
					t.Fatal(err)
				}
				var hits []store.KGSearchHit
				if err := json.Unmarshal([]byte(body), &hits); err != nil {
					t.Fatal(err)
				}
				return hits
			}
			ids := func(hits []store.KGSearchHit) map[string]bool {
				out := map[string]bool{}
				for _, h := range hits {
					out[h.SessionID+"|"+h.ID] = true
					if h.SessionID == b.ID || h.SessionID == web.ID {
						out["leak"] = true
					}
					if h.ID == ext.ID {
						out["extracted"] = true
					}
					if h.ID == cur.ID {
						out["curated"] = true
					}
					if h.SessionID == a.ID {
						out["a"] = true
					}
				}
				return out
			}
			if got := ids(search(b.ID)); got["a"] || got["leak"] || got["extracted"] || !got["curated"] {
				t.Fatalf("B search: %v", got)
			}
			if got := ids(search(a.ID)); !got["a"] || got["extracted"] || !got["curated"] {
				t.Fatalf("A search: %v", got)
			}
			if got := ids(search(web.ID)); !got["a"] || !got["extracted"] {
				t.Fatalf("web search unchanged: %v", got)
			}
			if got := ids(search("")); !got["a"] || !got["extracted"] {
				t.Fatalf("no-session search unchanged: %v", got)
			}
			if _, err := DispatchMemoryTool(st, store.DefaultTenant, b.ID, llm.ToolCall{Name: ToolMemoryExpand, Arguments: map[string]any{"id": ext.ID}}); err == nil {
				t.Fatal("channel caller must not expand an extracted entity")
			}
			if _, err := DispatchMemoryTool(st, store.DefaultTenant, b.ID, llm.ToolCall{Name: ToolMemoryExpand, Arguments: map[string]any{"id": cur.ID}}); err != nil {
				t.Fatalf("curated entity expand: %v", err)
			}

			list := func(caller string) int {
				body, err := DispatchSessionTool(st, store.DefaultTenant, caller, llm.ToolCall{Name: ToolSessionsList})
				if err != nil {
					t.Fatal(err)
				}
				var out struct {
					Sessions []sessionListItem `json:"sessions"`
				}
				_ = json.Unmarshal([]byte(body), &out)
				return len(out.Sessions)
			}
			if n := list(b.ID); n != 1 {
				t.Fatalf("B lists only its own session, got %d", n)
			}
			if n := list(web.ID); n != 3 {
				t.Fatalf("web lists all, got %d", n)
			}
			if _, err := DispatchSessionTool(st, store.DefaultTenant, b.ID, llm.ToolCall{Name: ToolSessionsHistory, Arguments: map[string]any{"session_id": a.ID}}); err == nil {
				t.Fatal("B must not read A's history")
			}
			if _, err := DispatchSessionTool(st, store.DefaultTenant, web.ID, llm.ToolCall{Name: ToolSessionsHistory, Arguments: map[string]any{"session_id": a.ID}}); err != nil {
				t.Fatalf("web reads history: %v", err)
			}
		})
	}
}
