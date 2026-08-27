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
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func crmTool(name string, approval bool) connector.Tool {
	schema := `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`
	if name == "message_send" {
		schema = `{"type":"object","properties":{"contact_id":{"type":"string"},"text":{"type":"string"}},"required":["contact_id","text"]}`
	}
	return connector.Tool{
		Name:             name,
		Description:      name,
		InputSchema:      json.RawMessage(schema),
		RequiresApproval: approval,
	}
}

func TestTools_ListFromAgentConnectors(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "A"})
	reg := connector.NewRegistry()
	_ = reg.Register(connector.NewFake("zalocrm", []connector.Tool{crmTool("contact_search", false)}))
	_ = reg.Register(connector.NewFake("pos", []connector.Tool{crmTool("order_lookup", false)}))
	_, _ = st.CreateConnector(store.ConnectorRecord{Name: "zalocrm", Transport: "http", Endpoint: "http://127.0.0.1:9", Enabled: true})
	_, _ = st.CreateConnector(store.ConnectorRecord{Name: "pos", Transport: "http", Endpoint: "http://127.0.0.1:9", Enabled: true})
	_ = st.LinkAgentConnector(a.ID, "zalocrm")

	rt := New(st, reg, approval.New(0), eventstore.New(64), llm.Echo{})
	tools, err := rt.ListTools(context.Background(), a.ID)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Tool.Name != "contact_search" {
		t.Fatalf("expected only linked crm tools, got %v", tools)
	}
}

func TestTools_InvokeStoresRoleToolAndTrace(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "A"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	reg := connector.NewRegistry()
	fake := connector.NewFake("zalocrm", []connector.Tool{crmTool("contact_search", false)})
	_ = reg.Register(fake)
	_, _ = st.CreateConnector(store.ConnectorRecord{Name: "zalocrm", Transport: "http", Endpoint: "http://x", Enabled: true})
	_ = st.LinkAgentConnector(a.ID, "zalocrm")

	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "tc1", Name: "zalocrm__contact_search", Arguments: map[string]any{"query": "A"}}}},
		{Text: "found A"},
	}}
	rt := New(st, reg, approval.New(0), eventstore.New(64), scripted)
	out, err := rt.Chat(context.Background(), sess.ID, "search A")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(out.Trace) != 1 || out.Trace[0].Connector != "zalocrm" || out.Trace[0].Tool != "contact_search" {
		t.Fatalf("trace %v", out.Trace)
	}
	if out.Trace[0].LatencyMS < 0 {
		t.Fatalf("latency")
	}
	if len(fake.Calls()) != 1 {
		t.Fatalf("expected one Invoke, got %+v", fake.Calls())
	}
	msgs, _ := st.ListMessages(sess.ID)
	var hasTool bool
	for _, m := range msgs {
		if m.Role == "tool" {
			hasTool = true
		}
	}
	if !hasTool {
		t.Fatalf("expected role=tool message, got %#v", msgs)
	}
	if out.Reply != "found A" {
		t.Fatalf("reply %q", out.Reply)
	}
	if len(scripted.Recorded) < 2 {
		t.Fatalf("recorded %d", len(scripted.Recorded))
	}
	foundWrap := false
	for _, m := range scripted.Recorded[1] {
		if m.Role == "tool" && strings.Contains(m.Content, security.UntrustedBegin) && strings.Contains(m.Content, security.UntrustedEnd) {
			foundWrap = true
		}
	}
	if !foundWrap {
		t.Fatalf("expected untrusted wrap in LLM tool message: %#v", scripted.Recorded[1])
	}
}

func TestChat_NextTurnKeepsUntrustedWrap(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "A"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	reg := connector.NewRegistry()
	_ = reg.Register(connector.NewFake("zalocrm", []connector.Tool{crmTool("contact_search", false)}))
	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "tc1", Name: "zalocrm__contact_search", Arguments: map[string]any{"query": "A"}}}},
		{Text: "found A"},
		{Text: "later"},
	}}
	rt := New(st, reg, approval.New(0), eventstore.New(64), scripted)
	if _, err := rt.Chat(context.Background(), sess.ID, "search A"); err != nil {
		t.Fatal(err)
	}
	n := len(scripted.Recorded)
	if _, err := rt.Chat(context.Background(), sess.ID, "follow-up"); err != nil {
		t.Fatal(err)
	}
	if len(scripted.Recorded) <= n {
		t.Fatal("expected second-turn prompt")
	}
	foundWrap := false
	for _, m := range scripted.Recorded[n] {
		if m.Role == "tool" && strings.Contains(m.Content, security.UntrustedBegin) {
			foundWrap = true
		}
	}
	if !foundWrap {
		t.Fatalf("next turn missing wrap: %#v", scripted.Recorded[n])
	}
}

func TestCallTool_RejectsDotDotPath(t *testing.T) {
	st := store.New()
	reg := connector.NewRegistry()
	_ = reg.Register(connector.NewFake("fs", []connector.Tool{{
		Name:        "read",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
	}}))
	rt := New(st, reg, approval.New(0), eventstore.New(64), llm.Echo{})
	_, err := rt.CallTool(context.Background(), "fs", "read", map[string]any{"path": "../etc/passwd"})
	if err == nil || !strings.Contains(err.Error(), "path escape") {
		t.Fatalf("expected path escape, got %v", err)
	}
}

func TestCallTool_WorkspaceAbsolute(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	st := store.New()
	reg := connector.NewRegistry()
	_ = reg.Register(connector.NewFake("fs", []connector.Tool{{
		Name:        "write",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
	}}))
	rt := New(st, reg, approval.New(0), eventstore.New(64), llm.Echo{})
	_, err := rt.CallTool(context.Background(), "fs", "write", map[string]any{"path": "/etc/passwd"})
	if err == nil {
		t.Fatal("expected outside workspace")
	}
}

func TestChat_EchoTurnsWriteSummaryAndNextPrompt(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "A"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	rt := New(st, connector.NewRegistry(), approval.New(0), eventstore.New(64), llm.Echo{})

	for i := 0; i < 11; i++ {
		if _, err := rt.Chat(context.Background(), sess.ID, "turn-start-"+itoa(i+1)); err != nil {
			t.Fatalf("chat %d: %v", i+1, err)
		}
	}
	if _, err := rt.Chat(context.Background(), sess.ID, "omega-end"); err != nil {
		t.Fatalf("chat 12: %v", err)
	}
	sum, err := st.LatestSummary(sess.ID)
	if err != nil || sum == nil || sum.Kind != store.KindEpisodic {
		t.Fatalf("summary %v %v", err, sum)
	}
	if !strings.Contains(sum.Body, "turn-start-1") || !strings.Contains(sum.Body, "omega-end") {
		t.Fatalf("echo summary %q", sum.Body)
	}
	if n := len([]rune(sum.Body)); n > 500 {
		t.Fatalf("summary runes %d", n)
	}

	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "next-ok"}}}
	rt.LLM = scripted
	if _, err := rt.Chat(context.Background(), sess.ID, "follow-up"); err != nil {
		t.Fatalf("follow-up: %v", err)
	}
	if len(scripted.Recorded) == 0 {
		t.Fatal("expected recorded prompt")
	}
	found := false
	for _, m := range scripted.Recorded[0] {
		if m.Role == "system" && strings.Contains(m.Content, "Previous summary:") && strings.Contains(m.Content, "omega-end") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("previous summary missing: %#v", scripted.Recorded[0])
	}
}

func TestChat_SummarizeFlag(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "A"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	rt := New(st, connector.NewRegistry(), approval.New(0), eventstore.New(64), llm.Echo{})
	if _, err := rt.ChatOpts(context.Background(), sess.ID, "solo-user", "", true); err != nil {
		t.Fatalf("ChatOpts: %v", err)
	}
	sum, err := st.LatestSummary(sess.ID)
	if err != nil || sum == nil || !strings.Contains(sum.Body, "solo-user") {
		t.Fatalf("flag summary %v %v", err, sum)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 8)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestChat_EchoNoTools(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "A"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	reg := connector.NewRegistry()
	_ = reg.Register(connector.NewFake("zalocrm", []connector.Tool{crmTool("contact_search", false)}))
	_, _ = st.CreateConnector(store.ConnectorRecord{Name: "zalocrm", Transport: "http", Endpoint: "http://x", Enabled: true})
	_ = st.LinkAgentConnector(a.ID, "zalocrm")

	rt := New(st, reg, approval.New(0), eventstore.New(64), llm.Echo{})
	out, err := rt.Chat(context.Background(), sess.ID, "tìm khách A")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(out.Trace) != 0 {
		t.Fatalf("echo must not dispatch tools, trace %v", out.Trace)
	}
	if out.Reply != "echo: tìm khách A" {
		t.Fatalf("reply %q", out.Reply)
	}
	msgs, _ := st.ListMessages(sess.ID)
	for _, m := range msgs {
		if m.Role == "tool" {
			t.Fatalf("echo stored tool row: %#v", msgs)
		}
	}
}

func TestChat_HistorySanitizeDropsOrphanTool(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "A"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	_, _ = st.AddMessage(store.Message{SessionID: sess.ID, Role: "tool", Content: `{"tool_call_id":"orphan","content":"nope"}`})

	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "ok"}}}
	rt := New(st, connector.NewRegistry(), approval.New(0), eventstore.New(64), scripted)
	if _, err := rt.Chat(context.Background(), sess.ID, "hello"); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(scripted.Recorded) == 0 {
		t.Fatal("expected recorded payload")
	}
	for _, m := range scripted.Recorded[0] {
		if m.Role == "tool" {
			t.Fatalf("orphan tool sent to LLM: %#v", scripted.Recorded[0])
		}
	}
}

func TestChat_PromptModes(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "Agent One"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	full := &llm.Scripted{Replies: []llm.Reply{{Text: "full-ok"}}}
	rt := New(st, connector.NewRegistry(), approval.New(0), eventstore.New(64), full)
	if _, err := rt.ChatWithMode(context.Background(), sess.ID, "hi", "full"); err != nil {
		t.Fatalf("full: %v", err)
	}
	if len(full.Recorded) == 0 || full.Recorded[0][0].Role != "system" || full.Recorded[0][0].Content == "" {
		t.Fatalf("full system missing: %#v", full.Recorded)
	}

	sess2, _ := st.CreateSession(store.Session{AgentID: a.ID})
	none := &llm.Scripted{Replies: []llm.Reply{{Text: "none-ok"}}}
	rt.LLM = none
	if _, err := rt.ChatWithMode(context.Background(), sess2.ID, "hi", "none"); err != nil {
		t.Fatalf("none: %v", err)
	}
	if len(none.Recorded) == 0 {
		t.Fatal("none recorded empty")
	}
	for _, m := range none.Recorded[0] {
		if m.Role == "system" && m.Content != "" {
			t.Fatalf("none sent system: %#v", none.Recorded[0])
		}
	}

	if _, err := rt.ChatWithMode(context.Background(), sess.ID, "hi", "nope"); err == nil {
		t.Fatal("expected unknown mode error")
	}
}

func TestChat_MaxIterations(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "A"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	reg := connector.NewRegistry()
	_ = reg.Register(connector.NewFake("zalocrm", []connector.Tool{crmTool("contact_search", false)}))
	_, _ = st.CreateConnector(store.ConnectorRecord{Name: "zalocrm", Transport: "http", Endpoint: "http://x", Enabled: true})
	_ = st.LinkAgentConnector(a.ID, "zalocrm")

	scripted := &llm.Scripted{
		RepeatLast: true,
		Replies: []llm.Reply{{ToolCalls: []llm.ToolCall{{
			Name: "zalocrm__contact_search", Arguments: map[string]any{"query": "loop"},
		}}}},
	}
	rt := New(st, reg, approval.New(0), eventstore.New(64), scripted)
	out, err := rt.Chat(context.Background(), sess.ID, "loop")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if out.Reply != "max_iterations" {
		t.Fatalf("reply %q", out.Reply)
	}
	if n := len(scripted.Recorded); n != 20 {
		t.Fatalf("think calls %d want 20", n)
	}
}

func TestChat_HooksOnToolPath(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "A"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	reg := connector.NewRegistry()
	_ = reg.Register(connector.NewFake("zalocrm", []connector.Tool{crmTool("contact_search", false)}))
	_, _ = st.CreateConnector(store.ConnectorRecord{Name: "zalocrm", Transport: "http", Endpoint: "http://x", Enabled: true})
	_ = st.LinkAgentConnector(a.ID, "zalocrm")

	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "h1", Name: "zalocrm__contact_search", Arguments: map[string]any{"query": "A"}}}},
		{Text: "done"},
	}}
	rt := New(st, reg, approval.New(0), eventstore.New(64), scripted)
	var names []string
	rt.Hooks.On(pipeline.SessionStart, func(context.Context, pipeline.Event) { names = append(names, pipeline.SessionStart) })
	rt.Hooks.On(pipeline.UserPromptSubmit, func(context.Context, pipeline.Event) { names = append(names, pipeline.UserPromptSubmit) })
	rt.Hooks.On(pipeline.PreToolUse, func(context.Context, pipeline.Event) { names = append(names, pipeline.PreToolUse) })
	rt.Hooks.On(pipeline.PostToolUse, func(context.Context, pipeline.Event) { names = append(names, pipeline.PostToolUse) })
	rt.Hooks.On(pipeline.Stop, func(context.Context, pipeline.Event) { names = append(names, pipeline.Stop) })

	if _, err := rt.Chat(context.Background(), sess.ID, "search A"); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	want := []string{pipeline.SessionStart, pipeline.UserPromptSubmit, pipeline.PreToolUse, pipeline.PostToolUse, pipeline.Stop}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("hooks %v want %v", names, want)
	}

	names = nil
	if _, err := rt.Chat(context.Background(), sess.ID, "follow-up"); err != nil {
		t.Fatalf("second Chat: %v", err)
	}
	for _, n := range names {
		if n == pipeline.SessionStart {
			t.Fatalf("SessionStart must not fire on later turns: %v", names)
		}
	}
}

func TestChat_HookPanicDoesNotAbort(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "A"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	reg := connector.NewRegistry()
	_ = reg.Register(connector.NewFake("zalocrm", []connector.Tool{crmTool("contact_search", false)}))
	_, _ = st.CreateConnector(store.ConnectorRecord{Name: "zalocrm", Transport: "http", Endpoint: "http://x", Enabled: true})
	_ = st.LinkAgentConnector(a.ID, "zalocrm")
	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "p1", Name: "zalocrm__contact_search", Arguments: map[string]any{"query": "A"}}}},
		{Text: "ok"},
	}}
	rt := New(st, reg, approval.New(0), eventstore.New(64), scripted)
	rt.Hooks.On(pipeline.PreToolUse, func(context.Context, pipeline.Event) { panic("hook boom") })
	out, err := rt.Chat(context.Background(), sess.ID, "search A")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if out.Reply != "ok" {
		t.Fatalf("reply %q", out.Reply)
	}
}

func TestChat_RejectsUnadvertisedTool(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "A"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	reg := connector.NewRegistry()
	crm := connector.NewFake("zalocrm", []connector.Tool{crmTool("contact_search", false)})
	pos := connector.NewFake("pos", []connector.Tool{crmTool("order_lookup", false)})
	_ = reg.Register(crm)
	_ = reg.Register(pos)
	_, _ = st.CreateConnector(store.ConnectorRecord{Name: "zalocrm", Transport: "http", Endpoint: "http://x", Enabled: true})
	_, _ = st.CreateConnector(store.ConnectorRecord{Name: "pos", Transport: "http", Endpoint: "http://x", Enabled: true})
	_ = st.LinkAgentConnector(a.ID, "zalocrm")

	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "bad", Name: "pos__order_lookup", Arguments: map[string]any{"order_id": "9"}}}},
		{Text: "refused"},
	}}
	rt := New(st, reg, approval.New(0), eventstore.New(64), scripted)
	out, err := rt.Chat(context.Background(), sess.ID, "lookup")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(pos.Calls()) != 0 {
		t.Fatalf("unadvertised tool ran: %+v", pos.Calls())
	}
	if len(out.Trace) != 1 || out.Trace[0].Status != "error" {
		t.Fatalf("trace %v", out.Trace)
	}
}

func TestChat_PendingStopsRemainingCalls(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "k1", DisplayName: "A"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	reg := connector.NewRegistry()
	crm := connector.NewFake("zalocrm", []connector.Tool{
		crmTool("message_send", true),
		crmTool("contact_search", false),
	})
	_ = reg.Register(crm)
	_, _ = st.CreateConnector(store.ConnectorRecord{Name: "zalocrm", Transport: "http", Endpoint: "http://x", Enabled: true})
	_ = st.LinkAgentConnector(a.ID, "zalocrm")

	scripted := &llm.Scripted{Replies: []llm.Reply{{ToolCalls: []llm.ToolCall{
		{ID: "s1", Name: "zalocrm__message_send", Arguments: map[string]any{"contact_id": "1", "text": "hi"}},
		{ID: "s2", Name: "zalocrm__contact_search", Arguments: map[string]any{"query": "A"}},
	}}}}
	rt := New(st, reg, approval.New(0), eventstore.New(64), scripted)
	out, err := rt.Chat(context.Background(), sess.ID, "send then search")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(out.Trace) != 1 || out.Trace[0].Status != "pending_approval" {
		t.Fatalf("trace %v", out.Trace)
	}
	if !strings.Contains(out.Reply, "pending_approval") {
		t.Fatalf("reply %q", out.Reply)
	}
	if len(crm.Calls()) != 0 {
		t.Fatalf("search must not run after pending: %+v", crm.Calls())
	}
}

func TestTools_SensitivePendingApproval(t *testing.T) {
	st := store.New()
	reg := connector.NewRegistry()
	fake := connector.NewFake("zalocrm", []connector.Tool{crmTool("message_send", true)})
	_ = reg.Register(fake)
	rt := New(st, reg, approval.New(0), eventstore.New(64), llm.Echo{})
	cr, err := rt.CallTool(context.Background(), "zalocrm", "message_send", map[string]any{"contact_id": "1", "text": "hi"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !cr.Pending || cr.Result == nil || cr.Result.ApprovalID == "" {
		t.Fatalf("expected pending, got %#v", cr)
	}
	if len(fake.Calls()) != 0 {
		t.Fatalf("Invoke must not run for approval tools: %+v", fake.Calls())
	}
	ev := rt.Events.Filter(eventstore.KindPendingApproval, "zalocrm", 10)
	if len(ev) != 1 {
		t.Fatalf("events %v", ev)
	}
}
