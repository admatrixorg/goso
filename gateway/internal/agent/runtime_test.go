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
	_ = reg.Register(connector.NewFake("zalocrm", []connector.Tool{crmTool("contact_search", false)}))
	_, _ = st.CreateConnector(store.ConnectorRecord{Name: "zalocrm", Transport: "http", Endpoint: "http://x", Enabled: true})
	_ = st.LinkAgentConnector(a.ID, "zalocrm")

	rt := New(st, reg, approval.New(0), eventstore.New(64), llm.Echo{})
	out, err := rt.Chat(context.Background(), sess.ID, "tìm khách A")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(out.Trace) != 1 || out.Trace[0].Connector != "zalocrm" {
		t.Fatalf("trace %v", out.Trace)
	}
	if out.Trace[0].LatencyMS < 0 {
		t.Fatalf("latency")
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
	if !strings.HasPrefix(out.Reply, "echo:") {
		t.Fatalf("reply %q", out.Reply)
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
