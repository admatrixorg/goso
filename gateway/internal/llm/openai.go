// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAI calls OpenAI Chat Completions via net/http.
// Named OpenAI-compat providers reuse this type with Label + BaseURL.
type OpenAI struct {
	APIKey        string
	Model         string
	BaseURL       string // default https://api.openai.com
	Client        *http.Client
	Label         string // named provider; empty → "openai"
	Stream        bool   // when true, send stream=true and parse SSE
	AllowEmptyKey bool   // router9 may omit Authorization
}

func (o *OpenAI) Name() string {
	if o != nil && o.Label != "" {
		return o.Label
	}
	return "openai"
}

func (o *OpenAI) ModelName() string {
	if o != nil && o.Model != "" {
		return o.Model
	}
	return "gpt-4o-mini"
}

func (o *OpenAI) Chat(ctx context.Context, messages []Message) (string, error) {
	s, _, err := o.ChatUsage(ctx, messages)
	return s, err
}

func (o *OpenAI) ChatUsage(ctx context.Context, messages []Message) (string, Usage, error) {
	reply, u, err := o.complete(ctx, messages, nil, o != nil && o.Stream)
	if err != nil {
		return "", Usage{}, err
	}
	return reply.Text, u, nil
}

func (o *OpenAI) ChatTools(ctx context.Context, messages []Message, tools []ToolSpec) (Reply, error) {
	reply, u, err := o.complete(ctx, messages, tools, false)
	reply.Usage = u
	return reply, err
}

func (o *OpenAI) complete(ctx context.Context, messages []Message, tools []ToolSpec, stream bool) (Reply, Usage, error) {
	if o == nil {
		return Reply{}, Usage{}, fmt.Errorf("openai: missing API key")
	}
	if o.APIKey == "" && !o.AllowEmptyKey {
		return Reply{}, Usage{}, fmt.Errorf("%s: missing API key", o.Name())
	}
	base := o.BaseURL
	if base == "" {
		base = "https://api.openai.com"
	}
	model := o.ModelName()
	payload := map[string]any{
		"model":    model,
		"messages": openaiMessages(messages),
	}
	if ts := openaiTools(tools); len(ts) > 0 {
		payload["tools"] = ts
	}
	if stream {
		payload["stream"] = true
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Reply{}, Usage{}, err
	}
	timeout := 30 * time.Second
	if o.Label == "router9" {
		timeout = 120 * time.Second
	}
	endpoint := chatCompletionsURL(base)
	if err := checkEndpoint(endpoint); err != nil {
		return Reply{}, Usage{}, err
	}
	client := guardedClient(o.Client, timeout)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return Reply{}, Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.APIKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Reply{}, Usage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return Reply{}, Usage{}, fmt.Errorf("%s %d: %s", o.Name(), resp.StatusCode, string(b))
	}
	if stream {
		text, u, err := ReadOpenAIStream(resp.Body)
		if err != nil {
			return Reply{}, Usage{}, err
		}
		return Reply{Text: text}, fallbackUsage(messages, text, u.PromptTokens, u.CompletionTokens), nil
	}
	b, _ := io.ReadAll(resp.Body)
	reply, promptTok, completionTok, err := parseOpenAIChat(b)
	if err != nil {
		return Reply{}, Usage{}, err
	}
	return reply, fallbackUsage(messages, reply.Text, promptTok, completionTok), nil
}

// chatCompletionsURL joins BaseURL with the chat completions path.
// If BaseURL already ends with /v1, append /chat/completions only (no /v1/v1).
func chatCompletionsURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

// modelsURL joins BaseURL with the OpenAI-compat models path.
func modelsURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/models"
	}
	return base + "/v1/models"
}

func openaiMessages(messages []Message) []map[string]any {
	var in []map[string]any
	for _, m := range messages {
		role := m.Role
		if role == "" {
			role = "user"
		}
		msg := map[string]any{"role": role, "content": m.Content}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			var tcs []map[string]any
			for _, tc := range m.ToolCalls {
				args, _ := json.Marshal(tc.Arguments)
				if len(args) == 0 {
					args = []byte("{}")
				}
				tcs = append(tcs, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": string(args),
					},
				})
			}
			msg["tool_calls"] = tcs
		}
		in = append(in, msg)
	}
	return in
}

func openaiTools(tools []ToolSpec) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	var out []map[string]any
	for _, t := range tools {
		fn := map[string]any{"name": t.Name}
		if t.Description != "" {
			fn["description"] = t.Description
		}
		if len(t.InputSchema) > 0 {
			var params any
			if err := json.Unmarshal(t.InputSchema, &params); err == nil {
				fn["parameters"] = params
			}
		}
		out = append(out, map[string]any{"type": "function", "function": fn})
	}
	return out
}

func stripTrailingSSEDone(b []byte) []byte {
	b = bytes.TrimSpace(b)
	const marker = "data: [DONE]"
	if i := bytes.LastIndex(b, []byte(marker)); i >= 0 {
		if len(bytes.TrimSpace(b[i+len(marker):])) == 0 {
			return bytes.TrimSpace(b[:i])
		}
	}
	return b
}

func parseOpenAIChat(b []byte) (Reply, int, int, error) {
	b = stripTrailingSSEDone(b)
	var out struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return Reply{}, 0, 0, err
	}
	if len(out.Choices) == 0 {
		return Reply{}, 0, 0, fmt.Errorf("openai: no choices")
	}
	msg := out.Choices[0].Message
	reply := Reply{Text: msg.Content}
	for _, tc := range msg.ToolCalls {
		args := map[string]any{}
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
		reply.ToolCalls = append(reply.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}
	if reply.Text == "" && len(reply.ToolCalls) == 0 {
		return Reply{}, 0, 0, fmt.Errorf("openai: empty content")
	}
	return reply, out.Usage.PromptTokens, out.Usage.CompletionTokens, nil
}
