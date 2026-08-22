// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package connector

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealth_RetryThenSuccess(t *testing.T) {
	var n atomic.Int32
	f := NewFake("flaky", []Tool{sampleTool("contact_search", false)})
	f.SetInvoke(func(ctx context.Context, tool string, args map[string]any) (*InvokeResult, error) {
		if n.Add(1) < 3 {
			return nil, context.DeadlineExceeded
		}
		return &InvokeResult{Tool: tool, Connector: "flaky", Status: "ok", Content: map[string]any{"ok": true}}, nil
	})
	c := WithRetry(f, RetryConfig{Timeout: time.Second, Retries: 3})
	res, err := c.Invoke(context.Background(), "contact_search", map[string]any{"query": "A"})
	if err != nil || res.Status != "ok" {
		t.Fatalf("Invoke: %v %v", err, res)
	}
	if n.Load() != 3 {
		t.Fatalf("attempts %d", n.Load())
	}
}

func TestHealth_TimeoutUnavailableNoHang(t *testing.T) {
	f := NewFake("slow", []Tool{sampleTool("contact_search", false)})
	f.SetInvoke(func(ctx context.Context, tool string, args map[string]any) (*InvokeResult, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			return &InvokeResult{Status: "ok"}, nil
		}
	})
	c := WithRetry(f, RetryConfig{Timeout: 50 * time.Millisecond, Retries: 1})
	start := time.Now()
	_, err := c.Invoke(context.Background(), "contact_search", map[string]any{"query": "A"})
	elapsed := time.Since(start)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("hung for %s", elapsed)
	}
}

func TestHealth_ParentCancel(t *testing.T) {
	f := NewFake("slow", []Tool{sampleTool("contact_search", false)})
	f.SetInvoke(func(ctx context.Context, tool string, args map[string]any) (*InvokeResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	c := WithRetry(f, RetryConfig{Timeout: time.Second, Retries: 5})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err := c.Invoke(ctx, "contact_search", map[string]any{"query": "A"})
	if !errors.Is(err, ErrUnavailable) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}
