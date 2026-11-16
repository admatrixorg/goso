// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/builtin"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/skill"
	"github.com/mqglobal/goso/gateway/internal/store"
)

type recMessenger struct {
	mu      sync.Mutex
	sends   []map[string]any
	callers []builtin.Caller
	tenants []string
}

func (m *recMessenger) Resolve(ctx context.Context, args map[string]any) (map[string]any, *connector.InvokeResult) {
	if args["text"] == "bad" {
		return args, &connector.InvokeResult{Tool: builtin.ToolMessage, Connector: builtin.ConnectorName, Status: "error", Content: map[string]any{"error": "channel not found"}}
	}
	return map[string]any{"text": args["text"], "channel": "tenant-a/telegram/shop", "chat_id": "42"}, nil
}

func (m *recMessenger) Send(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sends = append(m.sends, args)
	m.callers = append(m.callers, builtin.CallerFrom(ctx))
	m.tenants = append(m.tenants, skill.TenantFrom(ctx))
	return &connector.InvokeResult{Tool: builtin.ToolMessage, Connector: builtin.ConnectorName, Status: "ok", Content: map[string]any{"sent": true}}, nil
}

func TestCallTool_MessageGoesThroughApproval(t *testing.T) {
	rec := &recMessenger{}
	prev := builtin.MessageSender
	builtin.MessageSender = rec
	t.Cleanup(func() { builtin.MessageSender = prev })

	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "sales-a", DisplayName: "Sales A", TenantID: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	gate := approval.New(0)
	rt := New(st, connector.NewRegistry(), gate, eventstore.New(64), llm.Echo{})
	ctx := WithCallMeta(context.Background(), a.ID, "sess-1", "agent:"+a.ID)

	cr, err := rt.CallTool(ctx, builtin.ConnectorName, builtin.ToolMessage, map[string]any{"text": "Chào Minh An"})
	if err != nil || !cr.Pending || cr.Result.Status != "pending_approval" {
		t.Fatalf("%v %+v", err, cr)
	}
	if len(rec.sends) != 0 {
		t.Fatal("sent before approval")
	}
	pending := gate.List(approval.StatusPending)
	if len(pending) != 1 {
		t.Fatalf("pending %d", len(pending))
	}
	req := pending[0]
	if req.Args["channel"] != "tenant-a/telegram/shop" || req.Args["chat_id"] != "42" || req.AgentID != a.ID || req.SessionID != "sess-1" {
		t.Fatalf("approver must see the resolved target: %+v", req)
	}
	if _, err := gate.Decide(context.Background(), req.ID, approval.DecisionApprove); err != nil {
		t.Fatal(err)
	}
	if len(rec.sends) != 1 || rec.callers[0].AgentID != a.ID || rec.callers[0].SessionID != "sess-1" || !store.SameTenant(rec.tenants[0], "tenant-a") {
		t.Fatalf("%+v %+v %+v", rec.sends, rec.callers, rec.tenants)
	}

	// A call the messenger refuses never reaches the approval inbox.
	cr, err = rt.CallTool(ctx, builtin.ConnectorName, builtin.ToolMessage, map[string]any{"text": "bad"})
	if err != nil || cr.Pending || cr.Result.Status != "error" {
		t.Fatalf("%v %+v", err, cr)
	}
	if len(gate.List(approval.StatusPending)) != 0 || len(rec.sends) != 1 {
		t.Fatal("refusal queued or sent")
	}
}

func TestRuntimeTools_OpsToolsAdvertisedAndRouted(t *testing.T) {
	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "sales-a", DisplayName: "Sales A", TenantID: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: a.ID, TenantID: "tenant-a", Label: "web"})
	if err != nil {
		t.Fatal(err)
	}
	rt := New(st, connector.NewRegistry(), approval.New(0), eventstore.New(64), llm.Echo{})
	tools := runtimeTools{rt: rt}
	specs, err := tools.List(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, s := range specs {
		have[s.Name] = true
	}
	for _, n := range []string{pipeline.ToolCron, pipeline.ToolSessionStatus, pipeline.ToolSessionsSend, "builtin__datetime", "builtin__message"} {
		if !have[n] {
			t.Fatalf("%s not advertised", n)
		}
	}
	ctx := WithCallMeta(context.Background(), a.ID, sess.ID, "")
	out, err := tools.Call(ctx, a.ID, llm.ToolCall{Name: pipeline.ToolCron, Arguments: map[string]any{"action": "add", "spec": "0 2 * * *", "message": "Nhắc việc"}})
	if err != nil || out.Trace.Status != "ok" {
		t.Fatalf("%v %+v", err, out)
	}
	jobs := st.ListCronJobsTenant("tenant-a")
	if len(jobs) != 1 || jobs[0].SessionID != sess.ID {
		t.Fatalf("%+v", jobs)
	}
	out, err = tools.Call(ctx, a.ID, llm.ToolCall{Name: pipeline.ToolSessionStatus})
	if err != nil || !strings.Contains(out.Content, sess.ID) {
		t.Fatalf("%v %+v", err, out)
	}
	// A turn started by sessions_send cannot send again.
	nested := context.WithValue(ctx, sessionsSendKey{}, true)
	other, _ := st.CreateSession(store.Session{AgentID: a.ID, TenantID: "tenant-a", Label: "notes"})
	if _, err := tools.Call(nested, a.ID, llm.ToolCall{Name: pipeline.ToolSessionsSend, Arguments: map[string]any{"session_id": other.ID, "message": "x"}}); err == nil {
		t.Fatal("nested sessions_send accepted")
	}
}
