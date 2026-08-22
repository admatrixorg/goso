// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package connector

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func sampleTool(name string, approval bool) Tool {
	return Tool{
		Name:             name,
		Description:      "test " + name,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
		RequiresApproval: approval,
	}
}

func TestRegistry_RegisterLookupList(t *testing.T) {
	r := NewRegistry()
	f := NewFake("alpha", []Tool{sampleTool("contact_search", false)})
	if err := r.Register(f); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(f); !errors.Is(err, ErrExists) {
		t.Fatalf("expected ErrExists, got %v", err)
	}
	got, err := r.Lookup("alpha")
	if err != nil || got.Name() != "alpha" {
		t.Fatalf("Lookup: %v %v", err, got)
	}
	if _, err := r.Lookup("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	list := r.List()
	if len(list) != 1 {
		t.Fatalf("List len=%d", len(list))
	}
	tools, err := got.ListTools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].Name != "contact_search" {
		t.Fatalf("ListTools: %v %v", err, tools)
	}
	res, err := got.Invoke(context.Background(), "contact_search", map[string]any{"query": "A"})
	if err != nil || res.Status != "ok" {
		t.Fatalf("Invoke: %v %v", err, res)
	}
}

func TestRegistry_DisabledInvokeUnavailable(t *testing.T) {
	r := NewRegistry()
	f := NewFake("crm", []Tool{sampleTool("contact_search", false)})
	_ = r.Register(f)
	if err := r.SetEnabled("crm", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	got, err := r.Lookup("crm")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	_, err = got.Invoke(context.Background(), "contact_search", map[string]any{"query": "A"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if err.Error() != "connector_unavailable" && !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sentinel %q", err)
	}
	if err := got.Health(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Health disabled: %v", err)
	}
	_ = r.SetEnabled("crm", true)
	if err := got.Health(context.Background()); err != nil {
		// gated snapshot still has enabled=false; lookup again
	}
	got2, _ := r.Lookup("crm")
	if _, err := got2.Invoke(context.Background(), "contact_search", map[string]any{"query": "A"}); err != nil {
		t.Fatalf("re-enabled invoke: %v", err)
	}
}
