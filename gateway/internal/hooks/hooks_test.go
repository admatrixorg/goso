// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package hooks

import (
	"context"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestMatches(t *testing.T) {
	ev := pipeline.Event{Name: pipeline.PreToolUse, AgentID: "a1", Connector: "zalocrm", Tool: "contact_search"}
	cases := []struct {
		h    store.Hook
		want bool
	}{
		{store.Hook{Event: pipeline.PreToolUse, Enabled: true}, true},
		{store.Hook{Event: pipeline.PreToolUse, Enabled: false}, false},
		{store.Hook{Event: pipeline.PostToolUse, Enabled: true}, false},
		{store.Hook{Event: pipeline.PreToolUse, AgentID: "a2", Enabled: true}, false},
		{store.Hook{Event: pipeline.PreToolUse, AgentID: "a1", Enabled: true}, true},
		{store.Hook{Event: pipeline.PreToolUse, Matcher: "*", Enabled: true}, true},
		{store.Hook{Event: pipeline.PreToolUse, Matcher: "contact_*", Enabled: true}, true},
		{store.Hook{Event: pipeline.PreToolUse, Matcher: "zalocrm__*", Enabled: true}, true},
		{store.Hook{Event: pipeline.PreToolUse, Matcher: "pancake__*", Enabled: true}, false},
	}
	for i, c := range cases {
		h := c.h
		if got := matches(&h, ev); got != c.want {
			t.Fatalf("case %d %+v: %v", i, c.h, got)
		}
	}
}

func TestBlockOnlyOnPreToolUse(t *testing.T) {
	st := store.New()
	_, _ = st.CreateHook(store.Hook{TenantID: "minh-an", Event: pipeline.PreToolUse, HandlerType: HandlerBlock, Config: `{"message":"không"}`, Enabled: true})
	// A block row on PostToolUse cannot be created through Validate, but
	// the engine also ignores it if one exists.
	_, _ = st.CreateHook(store.Hook{TenantID: "minh-an", Event: pipeline.PostToolUse, HandlerType: HandlerBlock, Enabled: true})
	Invalidate()
	e := NewEngine(st)
	if d := e.Run(context.Background(), pipeline.Event{Name: pipeline.PreToolUse, TenantID: "minh-an", Tool: "x"}); !d.Block || d.Message != "không" {
		t.Fatalf("pre %+v", d)
	}
	if d := e.Run(context.Background(), pipeline.Event{Name: pipeline.PostToolUse, TenantID: "minh-an", Tool: "x"}); d.Block {
		t.Fatalf("post %+v", d)
	}
	if d := e.Run(context.Background(), pipeline.Event{Name: pipeline.PreToolUse, TenantID: "demo-shop", Tool: "x"}); d.Block {
		t.Fatalf("other tenant %+v", d)
	}
	var nilEngine *Engine
	if d := nilEngine.Run(context.Background(), pipeline.Event{Name: pipeline.PreToolUse}); d.Block {
		t.Fatal("nil engine blocked")
	}
}

func TestSignIsStable(t *testing.T) {
	a := Sign("k", "1700000000", []byte(`{"a":1}`))
	if a != Sign("k", "1700000000", []byte(`{"a":1}`)) || a == Sign("k2", "1700000000", []byte(`{"a":1}`)) || len(a) != len("sha256=")+64 {
		t.Fatalf("sign %s", a)
	}
}
