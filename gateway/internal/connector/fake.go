// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package connector

import (
	"context"
	"fmt"
	"sync"
)

// Fake is an in-memory connector for unit tests (no network).
type Fake struct {
	name   string
	mu     sync.Mutex
	tools  []Tool
	calls  []FakeCall
	health error
	invoke func(ctx context.Context, tool string, args map[string]any) (*InvokeResult, error)
}

// FakeCall records an Invoke.
type FakeCall struct {
	Tool string
	Args map[string]any
}

// NewFake returns a named in-memory connector.
func NewFake(name string, tools []Tool) *Fake {
	cp := make([]Tool, len(tools))
	copy(cp, tools)
	return &Fake{name: name, tools: cp}
}

func (f *Fake) Name() string { return f.name }

func (f *Fake) ListTools(context.Context) ([]Tool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Tool, len(f.tools))
	copy(out, f.tools)
	return out, nil
}

func (f *Fake) Invoke(ctx context.Context, tool string, args map[string]any) (*InvokeResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, FakeCall{Tool: tool, Args: args})
	fn := f.invoke
	tools := f.tools
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, tool, args)
	}
	found := false
	for _, t := range tools {
		if t.Name == tool {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("unknown tool %q", tool)
	}
	return &InvokeResult{
		Tool:      tool,
		Connector: f.name,
		Content:   map[string]any{"ok": true, "tool": tool, "args": args},
		Status:    "ok",
	}, nil
}

func (f *Fake) Health(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.health
}

// SetHealth injects the next Health error (nil = healthy).
func (f *Fake) SetHealth(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.health = err
}

// SetInvoke overrides Invoke.
func (f *Fake) SetInvoke(fn func(ctx context.Context, tool string, args map[string]any) (*InvokeResult, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invoke = fn
}

// Calls returns recorded Invoke calls.
func (f *Fake) Calls() []FakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]FakeCall, len(f.calls))
	copy(out, f.calls)
	return out
}
