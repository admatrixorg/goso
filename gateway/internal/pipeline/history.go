// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"encoding/json"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const HistoryCap = 50

type assistantWire struct {
	Text      string         `json:"text,omitempty"`
	ToolCalls []llm.ToolCall `json:"tool_calls,omitempty"`
}

type toolWire struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// EncodeAssistant serializes an assistant tool_use turn for store.Message.Content.
func EncodeAssistant(text string, calls []llm.ToolCall) string {
	b, err := json.Marshal(assistantWire{Text: text, ToolCalls: calls})
	if err != nil {
		return text
	}
	return string(b)
}

// EncodeTool serializes a tool result turn.
func EncodeTool(toolCallID, content string) string {
	b, err := json.Marshal(toolWire{ToolCallID: toolCallID, Content: content})
	if err != nil {
		return content
	}
	return string(b)
}

// ToLLM converts stored messages into LLM messages, decoding tool_use pairs.
func ToLLM(msgs []*store.Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			continue
		}
		lm := llm.Message{Role: m.Role, Content: m.Content}
		switch m.Role {
		case "assistant":
			if text, calls, ok := decodeAssistant(m.Content); ok {
				lm.Content = text
				lm.ToolCalls = calls
			}
		case "tool":
			if id, content, ok := decodeTool(m.Content); ok {
				lm.ToolCallID = id
				lm.Content = security.WrapUntrusted(content)
			} else {
				lm.Content = security.WrapUntrusted(m.Content)
			}
		}
		out = append(out, lm)
	}
	return out
}

func decodeAssistant(content string) (string, []llm.ToolCall, bool) {
	var w assistantWire
	if err := json.Unmarshal([]byte(content), &w); err != nil {
		return "", nil, false
	}
	if len(w.ToolCalls) == 0 {
		return "", nil, false
	}
	return w.Text, w.ToolCalls, true
}

func decodeTool(content string) (string, string, bool) {
	var w toolWire
	if err := json.Unmarshal([]byte(content), &w); err != nil {
		return "", "", false
	}
	if w.ToolCallID == "" {
		return "", "", false
	}
	return w.ToolCallID, w.Content, true
}

// Sanitize drops orphan tool rows and unmatched assistant tool_use entries.
func Sanitize(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	open := map[string]int{}

	flushUnmatched := func() {
		if len(open) == 0 {
			return
		}
		seen := map[int]struct{}{}
		for _, idx := range open {
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			if idx < 0 || idx >= len(out) {
				continue
			}
			keep := make([]llm.ToolCall, 0, len(out[idx].ToolCalls))
			for _, c := range out[idx].ToolCalls {
				if _, unmatched := open[c.ID]; unmatched {
					continue
				}
				keep = append(keep, c)
			}
			out[idx].ToolCalls = keep
		}
		// Drop assistants that became empty after stripping unmatched tool_use.
		filtered := out[:0]
		drop := map[int]struct{}{}
		for idx := range seen {
			if len(out[idx].ToolCalls) == 0 && stringsEmpty(out[idx].Content) {
				drop[idx] = struct{}{}
			}
		}
		for i, m := range out {
			if _, ok := drop[i]; ok {
				continue
			}
			filtered = append(filtered, m)
		}
		out = filtered
		open = map[string]int{}
	}

	for _, m := range msgs {
		switch m.Role {
		case "tool":
			id := m.ToolCallID
			if id == "" {
				continue
			}
			if _, ok := open[id]; !ok {
				continue
			}
			out = append(out, m)
			delete(open, id)
		case "assistant":
			if len(m.ToolCalls) == 0 {
				flushUnmatched()
				out = append(out, m)
				continue
			}
			flushUnmatched()
			out = append(out, m)
			idx := len(out) - 1
			for _, c := range m.ToolCalls {
				if c.ID != "" {
					open[c.ID] = idx
				}
			}
		default:
			flushUnmatched()
			out = append(out, m)
		}
	}
	flushUnmatched()
	return out
}

func stringsEmpty(s string) bool { return s == "" }

// CapLast keeps the last n messages.
func CapLast(msgs []llm.Message, n int) []llm.Message {
	if n <= 0 || len(msgs) <= n {
		return msgs
	}
	return msgs[len(msgs)-n:]
}
