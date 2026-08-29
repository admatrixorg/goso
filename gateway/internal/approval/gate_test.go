// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package approval

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGate_SubmitAndDecide(t *testing.T) {
	g := New(time.Minute)
	req := g.Submit("zalocrm", "message_send", map[string]any{"text": "hi"}, map[string]any{"reason": "mutation"})
	if req.ID == "" || req.Status != StatusPending {
		t.Fatalf("submit %v", req)
	}
	got, err := g.Get(req.ID)
	if err != nil || got.Tool != "message_send" {
		t.Fatalf("Get: %v %v", err, got)
	}
	dec, err := g.Decide(context.Background(), req.ID, "approve")
	if err != nil || dec.Status != StatusApproved {
		t.Fatalf("Decide: %v %v", err, dec)
	}
	if _, err := g.Decide(context.Background(), req.ID, "reject"); err != ErrNotPending {
		t.Fatalf("second decide: %v", err)
	}
}

func TestGate_RejectDoesNotInvoke(t *testing.T) {
	var relayed atomic.Int32
	g := New(time.Minute)
	g.Relayer = func(ctx context.Context, req *Request, decision string) error {
		relayed.Add(1)
		if decision != DecisionReject {
			t.Errorf("decision %s", decision)
		}
		return nil
	}
	req := g.Submit("pos", "price_change", map[string]any{"sku": "A", "price": 1}, nil)
	dec, err := g.Decide(context.Background(), req.ID, "reject")
	if err != nil || dec.Status != StatusRejected {
		t.Fatalf("reject: %v %v", err, dec)
	}
	if relayed.Load() != 1 {
		t.Fatalf("relay count %d", relayed.Load())
	}
}

func TestGate_BadDecision(t *testing.T) {
	g := New(time.Minute)
	req := g.Submit("c", "t", nil, nil)
	if _, err := g.Decide(context.Background(), req.ID, "maybe"); err != ErrBadDecision {
		t.Fatalf("got %v", err)
	}
	if _, err := g.Get("nope"); err != ErrNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestGate_Expire(t *testing.T) {
	g := New(10 * time.Millisecond)
	req := g.Submit("c", "t", nil, nil)
	time.Sleep(20 * time.Millisecond)
	got, err := g.Get(req.ID)
	if err != ErrExpired {
		t.Fatalf("got %v", err)
	}
	if got == nil || got.Status != StatusExpired || !got.Stale {
		t.Fatalf("expired copy %+v", got)
	}
	if _, err := g.Decide(context.Background(), req.ID, "approve"); err != ErrExpired {
		t.Fatalf("decide expired: %v", err)
	}
}

func TestGate_ListInboxAndPublicOmitsArgs(t *testing.T) {
	g := New(time.Minute)
	secret := "sk-live-fixture-not-a-vendor-aaaa"
	req := g.SubmitMeta(SubmitIn{
		Connector: "zalocrm",
		Tool:      "message_send",
		Args:      map[string]any{"contact_id": "1", "text": "hi", "token": secret, "api_key": "gsk_abc"},
		Requester: "agent:agt_1",
		AgentID:   "agt_1",
		SessionID: "ses_1",
	})
	if req.Risk != RiskHigh {
		t.Fatalf("risk %s", req.Risk)
	}
	if strings.Contains(req.ArgPreview, secret) || strings.Contains(req.ArgPreview, "gsk_abc") {
		t.Fatalf("preview leaked %s", req.ArgPreview)
	}
	if strings.Contains(req.ArgPreview, `"text"`) || strings.Contains(req.ArgPreview, `"token"`) {
		t.Fatalf("payload key in preview %s", req.ArgPreview)
	}
	if !strings.Contains(req.ArgPreview, "contact_id") {
		t.Fatalf("preview missing contact_id %s", req.ArgPreview)
	}
	raw, _ := json.Marshal(req)
	if strings.Contains(string(raw), `"args"`) || strings.Contains(string(raw), secret) {
		t.Fatalf("json leaked %s", raw)
	}
	pub := Public(req)
	if pub.Kind != KindExecution || pub.ApprovalID != req.ID {
		t.Fatalf("public %+v", pub)
	}
	b, _ := json.Marshal(pub)
	if strings.Contains(string(b), secret) || strings.Contains(strings.ToLower(string(b)), `"args"`) {
		t.Fatalf("public json leaked %s", b)
	}
	inbox := g.List("")
	if len(inbox) != 1 || inbox[0].ID != req.ID {
		t.Fatalf("inbox %v", inbox)
	}
	g.Submit("pos", "price_change", map[string]any{"sku": "A"}, nil)
	if len(g.List("pending")) != 2 {
		t.Fatalf("pending %d", len(g.List("pending")))
	}
}

func TestGate_DenyReasonAndSingleResolution(t *testing.T) {
	g := New(time.Minute)
	req := g.Submit("builtin", "write_file", map[string]any{"path": "a.txt", "content": "sk-hidden-bbbbbbbb"}, nil)
	dec, err := g.DecideReason(context.Background(), req.ID, "deny", "unsafe path")
	if err != nil || dec.Status != StatusRejected || dec.Decision != DecisionReject || dec.Reason != "unsafe path" {
		t.Fatalf("deny: %v %+v", err, dec)
	}
	if _, err := g.DecideReason(context.Background(), req.ID, "approve", ""); err != ErrNotPending {
		t.Fatalf("second: %v", err)
	}
	if len(g.List("")) != 0 {
		t.Fatalf("resolved still in inbox")
	}
	all := g.List("all")
	if len(all) != 1 || all[0].Status != StatusRejected {
		t.Fatalf("all %v", all)
	}
}

func TestGate_ExecutorRunsOnApproveOnly(t *testing.T) {
	var n atomic.Int32
	g := New(time.Minute)
	g.Executor = func(ctx context.Context, req *Request) error {
		n.Add(1)
		if req.Tool != "write_file" || req.Args["path"] != "a.txt" {
			t.Errorf("exec %+v", req)
		}
		return nil
	}
	req := g.Submit("builtin", "write_file", map[string]any{"path": "a.txt"}, nil)
	if _, err := g.Decide(context.Background(), req.ID, "approve"); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 1 {
		t.Fatalf("exec %d", n.Load())
	}
	req2 := g.Submit("builtin", "edit", map[string]any{"path": "a.txt"}, nil)
	if _, err := g.Decide(context.Background(), req2.ID, "reject"); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 1 {
		t.Fatalf("reject executed %d", n.Load())
	}
}

func TestClassifyRiskAndNormalize(t *testing.T) {
	if ClassifyRisk("write_file") != RiskHigh || ClassifyRisk("media") != RiskMedium {
		t.Fatalf("risk")
	}
	if NormalizeDecision("DENY") != DecisionReject || NormalizeDecision("approve") != DecisionApprove {
		t.Fatalf("norm")
	}
}
