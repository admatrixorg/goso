// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"
	"log"
	"sync"
)

// Hook names fired around a chat run.
const (
	SessionStart     = "SessionStart"
	UserPromptSubmit = "UserPromptSubmit"
	PreToolUse       = "PreToolUse"
	PostToolUse      = "PostToolUse"
	Stop             = "Stop"
)

// Event is delivered to registered hook handlers.
type Event struct {
	Name      string
	TenantID  string
	SessionID string
	AgentID   string
	Tool      string
	Connector string
	Arguments map[string]any
	Result    string
	Error     string
}

// Handler is an in-process lifecycle callback.
type Handler func(ctx context.Context, ev Event)

// Decision is what a PreToolUse gate answers. Block denies the tool call;
// Message is what the model sees in place of the tool result.
type Decision struct {
	Block   bool
	Message string
}

// Source supplies hooks configured outside the process (per tenant, from
// the store). Run is called for every event after the in-process handlers.
type Source interface {
	Run(ctx context.Context, ev Event) Decision
}

// Dispatcher is an in-process hook registry plus an optional configured
// Source. Failures never abort the run.
type Dispatcher struct {
	mu       sync.Mutex
	handlers map[string][]Handler
	source   Source
}

// NewDispatcher returns an empty dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: make(map[string][]Handler)}
}

// On registers a handler for a hook name.
func (d *Dispatcher) On(name string, h Handler) {
	if d == nil || h == nil || name == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handlers == nil {
		d.handlers = make(map[string][]Handler)
	}
	d.handlers[name] = append(d.handlers[name], h)
}

// SetSource installs the configured-hook source (nil removes it).
func (d *Dispatcher) SetSource(s Source) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.source = s
	d.mu.Unlock()
}

// Fire runs handlers for ev.Name. Panics are logged; the run continues.
func (d *Dispatcher) Fire(ctx context.Context, ev Event) {
	_ = d.Gate(ctx, ev)
}

// Gate runs handlers and the configured source for ev and returns the
// source's decision. Only PreToolUse decisions are acted on by the runner.
// A panicking source is logged and answers "allow".
func (d *Dispatcher) Gate(ctx context.Context, ev Event) (dec Decision) {
	if d == nil {
		return Decision{}
	}
	d.mu.Lock()
	hs := append([]Handler(nil), d.handlers[ev.Name]...)
	src := d.source
	d.mu.Unlock()
	for _, h := range hs {
		func(h Handler) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("hook %s: %v", ev.Name, rec)
				}
			}()
			h(ctx, ev)
		}(h)
	}
	if src == nil {
		return Decision{}
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("hook source %s: %v", ev.Name, rec)
			dec = Decision{}
		}
	}()
	return src.Run(ctx, ev)
}
