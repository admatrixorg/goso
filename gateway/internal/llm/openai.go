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

// OpenAI calls OpenAI Chat Completions via net/http.
type OpenAI struct {
	APIKey  string
	Model   string
	BaseURL string // default https://api.openai.com
	Client  *http.Client
}

func (o *OpenAI) Name() string { return "openai" }

func (o *OpenAI) Chat(ctx context.Context, messages []Message) (string, error) {
	if o.APIKey == "" {
		return "", fmt.Errorf("openai: missing API key")
	}
	base := o.BaseURL
	if base == "" {
		base = "https://api.openai.com"
	}
	model := o.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	var in []map[string]string
	for _, m := range messages {
		role := m.Role
		if role == "" {
			role = "user"
		}
		in = append(in, map[string]string{"role": role, "content": m.Content})
	}
	body, _ := json.Marshal(map[string]any{
		"model":    model,
		"messages": in,
	})
	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("openai %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("openai: no choices")
	}
	return out.Choices[0].Message.Content, nil
}
