// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
	var in []anthropicInMsg
	for _, m := range messages {
		switch m.Role {
		case "system":
			continue
		case "assistant":
			in = append(in, anthropicInMsg{Role: "assistant", Content: m.Content})
		default:
			in = append(in, anthropicInMsg{Role: "user", Content: m.Content})
		}
	}
	fullCache := anthropicCacheFull(a.CacheMode)
	payload := map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages":   in,
	}
	if stream {
		payload["stream"] = true
	}
	systemBlocks := anthropicSystemBlocks(messages, fullCache)
	if len(systemBlocks) == 1 && !fullCache {
		if text, ok := systemBlocks[0]["text"].(string); ok {
			payload["system"] = text
		}
	} else if len(systemBlocks) > 0 {
		payload["system"] = systemBlocks
	}
	if fullCache {
		cacheLastNonUser(in)
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

type anthropicInMsg struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func anthropicCacheFull(mode string) bool {
	if strings.EqualFold(strings.TrimSpace(mode), "full") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("GOSO_ANTHROPIC_CACHE_MODE")), "full") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv("GOSO_PROMPT_CACHE")), "full")
}

func ephemeralCacheControl() map[string]string {
	return map[string]string{"type": "ephemeral"}
}

// anthropicSystemBlocks emits one text block per incoming system message.
// CacheMode=full (or GOSO_PROMPT_CACHE=full) attaches cache_control to each.
func anthropicSystemBlocks(messages []Message, fullCache bool) []map[string]any {
	var blocks []map[string]any
	for _, m := range messages {
		if m.Role != "system" || strings.TrimSpace(m.Content) == "" {
			continue
		}
		block := map[string]any{"type": "text", "text": m.Content}
		if fullCache && cacheableSystem(m.Content) {
			block["cache_control"] = ephemeralCacheControl()
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// cacheLastNonUser attaches cache_control to the last assistant (non-user)
// block in the Anthropic messages array. User turns stay uncached.
// cacheableSystem is the stable prefix (identity/instructions and bootstrap
// files). Rolling "Previous summary:" is not a cache breakpoint.
func cacheableSystem(text string) bool {
	return !strings.HasPrefix(strings.TrimSpace(text), "Previous summary:")
}

func cacheLastNonUser(in []anthropicInMsg) {
	for i := len(in) - 1; i >= 0; i-- {
		if strings.EqualFold(in[i].Role, "user") {
			continue
		}
		text, _ := in[i].Content.(string)
		in[i].Content = []map[string]any{{
			"type":          "text",
			"text":          text,
			"cache_control": ephemeralCacheControl(),
		}}
		return
	}
}
