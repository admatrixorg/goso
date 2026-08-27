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

// Dispatcher is an in-process hook registry. Failures never abort the run.
type Dispatcher struct {
	mu       sync.Mutex
	handlers map[string][]Handler
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

// Fire runs handlers for ev.Name. Panics are logged; the run continues.
func (d *Dispatcher) Fire(ctx context.Context, ev Event) {
	if d == nil {
		return
	}
	d.mu.Lock()
	hs := append([]Handler(nil), d.handlers[ev.Name]...)
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
}
