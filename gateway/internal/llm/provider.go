// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import "context"

// Message is a chat turn.
type Message struct {
	Role       string     `json:"role"` // user | assistant | system | tool
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Provider can generate a reply from a conversation.
type Provider interface {
	Name() string
	Chat(ctx context.Context, messages []Message) (string, error)
}

// Echo is a fallback provider that echoes the last user message.
// StreamParts, when set, are emitted as ChatStream deltas (tests: 1–N chunks).
type Echo struct {
	StreamParts []string
}

func (Echo) Name() string      { return "echo" }
func (Echo) ModelName() string { return "echo" }

func (e Echo) Chat(ctx context.Context, messages []Message) (string, error) {
	s, _, err := e.ChatUsage(ctx, messages)
	return s, err
}

func (e Echo) ChatUsage(ctx context.Context, messages []Message) (string, Usage, error) {
	if err := ctx.Err(); err != nil {
		return "", Usage{}, err
	}
	if len(e.StreamParts) > 0 {
		var b string
		for _, p := range e.StreamParts {
			b += p
		}
		return b, EstimateUsage(messages, b), nil
	}
	reply := "echo: (no message)"
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			reply = "echo: " + messages[i].Content
			break
		}
	}
	return reply, EstimateUsage(messages, reply), nil
}

func (e Echo) ChatStream(ctx context.Context, messages []Message, onDelta StreamHandler) (string, error) {
	s, _, err := e.ChatStreamUsage(ctx, messages, onDelta)
	return s, err
}

func (e Echo) ChatStreamUsage(ctx context.Context, messages []Message, onDelta StreamHandler) (string, Usage, error) {
	reply, usage, err := e.ChatUsage(ctx, messages)
	if err != nil {
		return "", Usage{}, err
	}
	if len(e.StreamParts) > 0 {
		for _, p := range e.StreamParts {
			emitDelta(onDelta, p)
		}
		return reply, usage, nil
	}
	emitDelta(onDelta, reply)
	return reply, usage, nil
}
