// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import "context"

// StreamHandler is invoked once per text delta as bytes arrive.
type StreamHandler func(delta string)

// Streamer is an optional native streaming surface. Providers that cannot
// emit mid-reply deltas should omit it; ChatStream then sends one honest chunk.
type Streamer interface {
	ChatStream(ctx context.Context, messages []Message, onDelta StreamHandler) (string, error)
}

// StreamToolChat streams text deltas and may return tool calls from the same turn.
type StreamToolChat interface {
	ChatStreamTools(ctx context.Context, messages []Message, tools []ToolSpec, onDelta StreamHandler) (Reply, error)
}

type streamUsageProvider interface {
	ChatStreamUsage(ctx context.Context, messages []Message, onDelta StreamHandler) (string, Usage, error)
}

func emitDelta(onDelta StreamHandler, delta string) {
	if onDelta == nil || delta == "" {
		return
	}
	onDelta(delta)
}

// ChatStream calls a native stream when the provider implements one.
// Otherwise it completes Chat and emits a single honest chunk (no fake splits).
func ChatStream(ctx context.Context, p Provider, messages []Message, onDelta StreamHandler) (string, Usage, error) {
	if p == nil {
		p = Echo{}
	}
	if err := ctx.Err(); err != nil {
		return "", Usage{}, err
	}
	if u, ok := p.(streamUsageProvider); ok {
		return u.ChatStreamUsage(ctx, messages, onDelta)
	}
	if s, ok := p.(Streamer); ok {
		text, err := s.ChatStream(ctx, messages, onDelta)
		if err != nil {
			return text, Usage{}, err
		}
		return text, EstimateUsage(messages, text), nil
	}
	text, usage, err := ChatUsage(ctx, p, messages)
	if err != nil {
		return text, usage, err
	}
	emitDelta(onDelta, text)
	return text, usage, nil
}
