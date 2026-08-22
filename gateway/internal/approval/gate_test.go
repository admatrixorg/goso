// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package approval

import (
	"context"
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
	if _, err := g.Get(req.ID); err != ErrExpired {
		t.Fatalf("got %v", err)
	}
}
