// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"context"
	"strings"
	"sync"
)

// Scripted is a test/e2e ToolChat provider that returns canned Replies in order.
type Scripted struct {
	Label      string
	Replies    []Reply
	RepeatLast bool
	E2E        bool

	mu            sync.Mutex
	turn          int
	Recorded      [][]Message
	RecordedTools [][]ToolSpec
}

// NewE2EScripted returns a provider used only when GOSO_E2E_SCRIPTED=1.
// Turn 1 requests contact_search; later turns return final text.
func NewE2EScripted() *Scripted {
	return &Scripted{Label: "scripted", E2E: true}
}

func (s *Scripted) Name() string {
	if s != nil && s.Label != "" {
		return s.Label
	}
	return "scripted"
}

func (s *Scripted) ModelName() string { return s.Name() }

func (s *Scripted) Chat(ctx context.Context, messages []Message) (string, error) {
	r, err := s.ChatTools(ctx, messages, nil)
	if err != nil {
		return "", err
	}
	return r.Text, nil
}

func (s *Scripted) ChatTools(_ context.Context, messages []Message, tools []ToolSpec) (Reply, error) {
	if s == nil {
		return Reply{Text: "scripted: empty"}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Recorded = append(s.Recorded, cloneMessages(messages))
	s.RecordedTools = append(s.RecordedTools, cloneToolSpecs(tools))

	if s.E2E {
		s.turn++
		if s.turn == 1 {
			return e2eFirst(messages, tools), nil
		}
		return Reply{Text: "ok"}, nil
	}

	if len(s.Replies) == 0 {
		return Reply{Text: "scripted: empty"}, nil
	}
	idx := s.turn
	if idx >= len(s.Replies) {
		if s.RepeatLast {
			idx = len(s.Replies) - 1
		} else {
			return Reply{Text: s.Replies[len(s.Replies)-1].Text}, nil
		}
	} else {
		s.turn++
	}
	return cloneReply(s.Replies[idx]), nil
}

func e2eFirst(messages []Message, tools []ToolSpec) Reply {
	query := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			query = messages[i].Content
			break
		}
	}
	name := "contact_search"
	conn := ""
	for _, t := range tools {
		if strings.Contains(t.Name, "contact_search") {
			name = t.Name
			conn = t.Connector
			break
		}
	}
	return Reply{ToolCalls: []ToolCall{{
		ID:        "e2e_contact_search",
		Name:      name,
		Connector: conn,
		Arguments: map[string]any{"query": query},
	}}}
}

func cloneMessages(in []Message) []Message {
	if in == nil {
		return nil
	}
	out := make([]Message, len(in))
	copy(out, in)
	for i := range out {
		if len(in[i].ToolCalls) > 0 {
			out[i].ToolCalls = cloneToolCalls(in[i].ToolCalls)
		}
	}
	return out
}

func cloneToolSpecs(in []ToolSpec) []ToolSpec {
	if in == nil {
		return nil
	}
	out := make([]ToolSpec, len(in))
	copy(out, in)
	return out
}

func cloneToolCalls(in []ToolCall) []ToolCall {
	out := make([]ToolCall, len(in))
	for i, c := range in {
		out[i] = c
		if c.Arguments != nil {
			args := make(map[string]any, len(c.Arguments))
			for k, v := range c.Arguments {
				args[k] = v
			}
			out[i].Arguments = args
		}
	}
	return out
}

func cloneReply(r Reply) Reply {
	return Reply{Text: r.Text, ToolCalls: cloneToolCalls(r.ToolCalls)}
}
