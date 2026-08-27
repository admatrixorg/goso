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
type Echo struct{}

func (Echo) Name() string      { return "echo" }
func (Echo) ModelName() string { return "echo" }

func (Echo) Chat(ctx context.Context, messages []Message) (string, error) {
	s, _, err := Echo{}.ChatUsage(ctx, messages)
	return s, err
}

func (Echo) ChatUsage(_ context.Context, messages []Message) (string, Usage, error) {
	reply := "echo: (no message)"
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			reply = "echo: " + messages[i].Content
			break
		}
	}
	return reply, EstimateUsage(messages, reply), nil
}
