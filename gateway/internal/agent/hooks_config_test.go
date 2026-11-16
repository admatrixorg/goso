// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/hooks"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func hookFixture(t *testing.T, tenant string) (*store.Store, *store.Agent, *store.Session, *connector.Fake) {
	t.Helper()
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "sale", DisplayName: "Minh An", TenantID: tenant})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID, TenantID: tenant})
	fake := connector.NewFake("zalocrm", []connector.Tool{crmTool("contact_search", false)})
	_, _ = st.CreateConnector(store.ConnectorRecord{Name: "zalocrm", Transport: "http", Endpoint: "http://x", Enabled: true})
	_ = st.LinkAgentConnector(a.ID, "zalocrm")
	return st, a, sess, fake
}

func toolThenDone() *llm.Scripted {
	return &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "zalocrm__contact_search", Arguments: map[string]any{"query": "Minh An"}}}},
		{Text: "done"},
	}}
}

func lastToolMessage(s *llm.Scripted) string {
	if len(s.Recorded) < 2 {
		return ""
	}
	msgs := s.Recorded[1]
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "tool" {
			return msgs[i].Content
		}
	}
	return ""
}

func TestConfiguredBlockHookDeniesTool(t *testing.T) {
	st, a, sess, fake := hookFixture(t, "minh-an")
	if _, err := st.CreateHook(store.Hook{TenantID: "minh-an", Event: "PreToolUse", Matcher: "zalocrm__contact_*", HandlerType: "block", Config: `{"message":"Không tra khách ngoài giờ"}`, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	hooks.Invalidate()
	reg := connector.NewRegistry()
	_ = reg.Register(fake)
	scripted := toolThenDone()
	rt := New(st, reg, approval.New(0), eventstore.New(64), scripted)
	out, err := rt.Chat(context.Background(), sess.ID, "tìm Minh An")
	if err != nil || out.Reply != "done" {
		t.Fatalf("run broke: %v %+v", err, out)
	}
	if n := len(fake.Calls()); n != 0 {
		t.Fatalf("blocked tool ran %d times", n)
	}
	if got := lastToolMessage(scripted); !strings.Contains(got, "blocked by hook: Không tra khách ngoài giờ") {
		t.Fatalf("model saw %q", got)
	}
	_ = a
}

func TestConfiguredBlockHookScopedToTenantAndAgent(t *testing.T) {
	st, a, sess, fake := hookFixture(t, "minh-an")
	// Another tenant's block hook and a hook for another agent do nothing here.
	_, _ = st.CreateHook(store.Hook{TenantID: "demo-shop", Event: "PreToolUse", HandlerType: "block", Enabled: true})
	_, _ = st.CreateHook(store.Hook{TenantID: "minh-an", AgentID: "someone-else", Event: "PreToolUse", HandlerType: "block", Enabled: true})
	_, _ = st.CreateHook(store.Hook{TenantID: "minh-an", Event: "PreToolUse", Matcher: "other_*", HandlerType: "block", Enabled: true})
	_, _ = st.CreateHook(store.Hook{TenantID: "minh-an", Event: "PreToolUse", HandlerType: "block", Enabled: false})
	hooks.Invalidate()
	reg := connector.NewRegistry()
	_ = reg.Register(fake)
	rt := New(st, reg, approval.New(0), eventstore.New(64), toolThenDone())
	if _, err := rt.Chat(context.Background(), sess.ID, "tìm"); err != nil {
		t.Fatal(err)
	}
	if n := len(fake.Calls()); n != 1 {
		t.Fatalf("tool calls %d want 1", n)
	}
	// Now a hook for this agent: the next run is blocked (cache invalidated).
	_, _ = st.CreateHook(store.Hook{TenantID: "minh-an", AgentID: a.ID, Event: "PreToolUse", HandlerType: "block", Enabled: true})
	hooks.Invalidate()
	scripted := toolThenDone()
	rt.LLM = scripted
	if _, err := rt.Chat(context.Background(), sess.ID, "tìm lại"); err != nil {
		t.Fatal(err)
	}
	if n := len(fake.Calls()); n != 1 {
		t.Fatalf("tool ran after agent hook: %d", n)
	}
	if got := lastToolMessage(scripted); !strings.Contains(got, "tool call blocked by hook") {
		t.Fatalf("model saw %q", got)
	}
}

func TestConfiguredHTTPHookSignsAndNeverBreaksRun(t *testing.T) {
	t.Setenv("GOSO_SSRF", "0")
	t.Setenv("GOSO_MASTER_KEY", strings.Repeat("ab", 32))
	var mu sync.Mutex
	type got struct {
		event, sig, ts string
		body           []byte
	}
	var seen []got
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, got{r.Header.Get(hooks.HeaderEvent), r.Header.Get(hooks.HeaderSignature), r.Header.Get(hooks.HeaderTimestamp), b})
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError) // a failing receiver must not break the run
	}))
	defer srv.Close()

	st, _, sess, fake := hookFixture(t, "minh-an")
	enc, err := hooks.SealSecret("demo-signing-secret")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(map[string]any{"url": srv.URL, "timeout_ms": 2000})
	for _, ev := range []string{"PostToolUse", "Stop"} {
		if _, err := st.CreateHook(store.Hook{TenantID: "minh-an", Event: ev, HandlerType: "http", Config: string(cfg), SecretEnc: enc, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	// A dead URL on another event: delivery fails, the run goes on.
	dead, _ := json.Marshal(map[string]any{"url": "http://127.0.0.1:1/never", "timeout_ms": 200})
	_, _ = st.CreateHook(store.Hook{TenantID: "minh-an", Event: "UserPromptSubmit", HandlerType: "http", Config: string(dead), Enabled: true})
	hooks.Invalidate()
	reg := connector.NewRegistry()
	_ = reg.Register(fake)
	rt := New(st, reg, approval.New(0), eventstore.New(64), toolThenDone())
	out, err := rt.Chat(context.Background(), sess.ID, "tìm Minh An")
	if err != nil || out.Reply != "done" {
		t.Fatalf("run broke: %v %+v", err, out)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(seen)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("deliveries %d want 2", len(seen))
	}
	events := map[string]bool{}
	for _, g := range seen {
		events[g.event] = true
		if g.sig != hooks.Sign("demo-signing-secret", g.ts, g.body) {
			t.Fatalf("bad signature for %s", g.event)
		}
		var p map[string]any
		if err := json.Unmarshal(g.body, &p); err != nil || p["tenant_id"] != "minh-an" || p["event"] != g.event {
			t.Fatalf("payload %s %v", g.body, err)
		}
		if g.event == "PostToolUse" && (p["tool"] != "contact_search" || p["connector"] != "zalocrm") {
			t.Fatalf("tool payload %s", g.body)
		}
	}
	if !events["PostToolUse"] || !events["Stop"] {
		t.Fatalf("events %v", events)
	}
}
