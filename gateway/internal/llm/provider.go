// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import "context"

// Message is a chat turn.
type Message struct {
	Role    string `json:"role"` // user | assistant | system
	Content string `json:"content"`
}

// Provider can generate a reply from a conversation.
type Provider interface {
	Name() string
	Chat(ctx context.Context, messages []Message) (string, error)
}

// Echo is a fallback provider that echoes the last user message.
type Echo struct{}

func (Echo) Name() string { return "echo" }
func (Echo) Chat(_ context.Context, messages []Message) (string, error) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return "echo: " + messages[i].Content, nil
		}
	}
	return "echo: (no message)", nil
}
