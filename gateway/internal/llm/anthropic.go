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

// Anthropic calls Anthropic Messages API via net/http.
type Anthropic struct {
	APIKey    string
	Model     string
	BaseURL   string // default https://api.anthropic.com
	Client    *http.Client
	CacheMode string // "full" includes cache_control; other values are ignored
}

func (a *Anthropic) Name() string { return "anthropic" }

func (a *Anthropic) ModelName() string {
	if a.Model != "" {
		return a.Model
	}
	return "claude-sonnet-4-20250514"
}

func (a *Anthropic) Chat(ctx context.Context, messages []Message) (string, error) {
	s, _, err := a.ChatUsage(ctx, messages)
	return s, err
}

func (a *Anthropic) ChatStream(ctx context.Context, messages []Message, onDelta StreamHandler) (string, error) {
	s, _, err := a.ChatStreamUsage(ctx, messages, onDelta)
	return s, err
}

func (a *Anthropic) ChatStreamUsage(ctx context.Context, messages []Message, onDelta StreamHandler) (string, Usage, error) {
	return a.call(ctx, messages, true, onDelta)
}

func (a *Anthropic) ChatUsage(ctx context.Context, messages []Message) (string, Usage, error) {
	return a.call(ctx, messages, false, nil)
}

func (a *Anthropic) call(ctx context.Context, messages []Message, stream bool, onDelta StreamHandler) (string, Usage, error) {
	if a.APIKey == "" {
		return "", Usage{}, fmt.Errorf("anthropic: missing API key")
	}
	base := a.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	model := a.Model
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	// Convert messages: Anthropic requires alternating user/assistant, no system in array.
	// Simplify: map all to user/assistant, system -> user.
	type inMsg struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}
	var in []inMsg
	var system string
	for _, m := range messages {
		switch m.Role {
		case "system":
			system += m.Content + "\n"
		case "assistant":
			in = append(in, inMsg{Role: "assistant", Content: m.Content})
		default:
			in = append(in, inMsg{Role: "user", Content: m.Content})
		}
	}
	fullCache := strings.EqualFold(strings.TrimSpace(a.CacheMode), "full")
	payload := map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages":   in,
	}
	if stream {
		payload["stream"] = true
	}
	if system != "" {
		if fullCache {
			payload["system"] = []map[string]any{{
				"type": "text", "text": system,
				"cache_control": map[string]string{"type": "ephemeral"},
			}}
		} else {
			payload["system"] = system
		}
	}
	if fullCache && len(in) > 0 {
		last := in[len(in)-1]
		text, _ := last.Content.(string)
		in[len(in)-1] = inMsg{Role: last.Role, Content: []map[string]any{{
			"type": "text", "text": text,
			"cache_control": map[string]string{"type": "ephemeral"},
		}}}
		payload["messages"] = in
	}
	body, _ := json.Marshal(payload)
	endpoint := strings.TrimRight(base, "/") + "/v1/messages"
	if err := checkEndpoint(endpoint); err != nil {
		return "", Usage{}, err
	}
	client := guardedClient(a.Client, 30*time.Second)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := client.Do(req)
	if err != nil {
		return "", Usage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", Usage{}, fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(b))
	}
	if stream {
		text, su, err := ReadAnthropicStreamDeltas(resp.Body, onDelta)
		if err != nil {
			return "", Usage{}, err
		}
		u := fallbackUsage(messages, text, su.PromptTokens, su.CompletionTokens)
		u.CacheReadTokens = su.CacheReadTokens
		return text, u, nil
	}
	b, _ := io.ReadAll(resp.Body)
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens          int `json:"input_tokens"`
			OutputTokens         int `json:"output_tokens"`
			CacheReadInputTokens int `json:"cache_read_input_tokens"`
			CacheReadTokens      int `json:"cache_read_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", Usage{}, err
	}
	if len(out.Content) == 0 {
		return "", Usage{}, fmt.Errorf("anthropic: empty content")
	}
	content := out.Content[0].Text
	u := fallbackUsage(messages, content, out.Usage.InputTokens, out.Usage.OutputTokens)
	u.CacheReadTokens = out.Usage.CacheReadInputTokens
	if u.CacheReadTokens == 0 {
		u.CacheReadTokens = out.Usage.CacheReadTokens
	}
	return content, u, nil
}
