// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Anthropic calls Anthropic Messages API via net/http.
type Anthropic struct {
	APIKey  string
	Model   string
	BaseURL string // default https://api.anthropic.com
	Client  *http.Client
}

func (a *Anthropic) Name() string { return "anthropic" }

func (a *Anthropic) Chat(ctx context.Context, messages []Message) (string, error) {
	if a.APIKey == "" {
		return "", fmt.Errorf("anthropic: missing API key")
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
		Content string `json:"content"`
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
	body, _ := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages":   in,
		"system":     system,
	})
	// Anthropic API expects system as string if present; omit if empty.
	if system == "" {
		// re-marshal without system
		body, _ = json.Marshal(map[string]any{
			"model":      model,
			"max_tokens": 1024,
			"messages":   in,
		})
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if len(out.Content) == 0 {
		return "", fmt.Errorf("anthropic: empty content")
	}
	return out.Content[0].Text, nil
}
