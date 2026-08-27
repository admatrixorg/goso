// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"context"
	"encoding/json"
)

// ToolSpec is a tool advertised to a ToolChat provider.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Connector   string          `json:"connector,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// ToolCall is one model-requested tool invocation.
type ToolCall struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name"`
	Connector string         `json:"connector,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// Reply is a ToolChat result: final text and/or tool calls.
type Reply struct {
	Text      string     `json:"text,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     Usage      `json:"usage,omitempty"`
}

// ToolChat is an optional provider surface. Pipeline uses it when present.
// Providers that only implement Chat stay text-only (no keyword tool dispatch).
type ToolChat interface {
	ChatTools(ctx context.Context, messages []Message, tools []ToolSpec) (Reply, error)
}
