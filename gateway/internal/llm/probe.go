// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const probeTimeout = 20 * time.Second

// TestResult is the public payload of POST /api/providers/{name}/test.
// Never includes api_key. ok is true only after a real probe succeeds.
type TestResult struct {
	OK        bool     `json:"ok"`
	LatencyMS int64    `json:"latency_ms"`
	Models    []string `json:"models,omitempty"`
	Reply     string   `json:"reply,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// APIKeySecretName is the secrets-table key for a provider API key.
func APIKeySecretName(name string) string {
	return "provider:" + strings.TrimSpace(name) + ":api_key"
}

// ValidType reports whether typ is one of the four configure types.
func ValidType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case TypeOpenAICompat, TypeAnthropic, TypeEcho, TypeRouter9:
		return true
	default:
		return false
	}
}

// Build constructs a provider from persisted fields (sqlite overlay).
func Build(name, typ, baseURL, model, apiKey string) (Provider, error) {
	name = strings.TrimSpace(name)
	typ = strings.TrimSpace(typ)
	baseURL = strings.TrimSpace(baseURL)
	model = strings.TrimSpace(model)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if !ValidType(typ) {
		return nil, fmt.Errorf("unknown type %q", typ)
	}
	client := &http.Client{Timeout: probeTimeout}
	switch typ {
	case TypeEcho:
		return Echo{}, nil
	case TypeAnthropic:
		return &Anthropic{APIKey: apiKey, Model: model, BaseURL: baseURL, Client: client}, nil
	case TypeRouter9:
		return &OpenAI{
			APIKey: apiKey, Model: model, BaseURL: baseURL, Label: name,
			AllowEmptyKey: true, Client: client,
		}, nil
	default:
		return &OpenAI{
			APIKey: apiKey, Model: model, BaseURL: baseURL, Label: name,
			Client: client,
		}, nil
	}
}

// Probe runs a real models-list or 1-turn chat against p. Never fakes ok.
func Probe(ctx context.Context, p Provider, kind string) TestResult {
	kind = strings.TrimSpace(strings.ToLower(kind))
	if kind == "" {
		kind = "models"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	switch kind {
	case "chat":
		return probeChat(ctx, p)
	case "models":
		return probeModels(ctx, p)
	default:
		return TestResult{OK: false, Error: `kind must be "models" or "chat"`}
	}
}

func probeChat(ctx context.Context, p Provider) TestResult {
	start := time.Now()
	if p == nil {
		return TestResult{OK: false, LatencyMS: time.Since(start).Milliseconds(), Error: "provider missing"}
	}
	reply, err := p.Chat(ctx, []Message{{Role: "user", Content: "ping"}})
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return TestResult{OK: false, LatencyMS: ms, Error: err.Error()}
	}
	return TestResult{OK: true, LatencyMS: ms, Reply: reply}
}

func probeModels(ctx context.Context, p Provider) TestResult {
	start := time.Now()
	if p == nil {
		return TestResult{OK: false, LatencyMS: time.Since(start).Milliseconds(), Error: "provider missing"}
	}
	if _, isEcho := p.(Echo); isEcho || p.Name() == "echo" {
		return TestResult{OK: true, LatencyMS: time.Since(start).Milliseconds(), Models: []string{"echo"}}
	}
	base, key, client, anthropic := httpMeta(p)
	if base == "" {
		return TestResult{OK: false, LatencyMS: time.Since(start).Milliseconds(), Error: "no base_url"}
	}
	if client == nil {
		client = &http.Client{Timeout: probeTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL(base), nil)
	if err != nil {
		return TestResult{OK: false, LatencyMS: time.Since(start).Milliseconds(), Error: err.Error()}
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if anthropic {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	resp, err := client.Do(req)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return TestResult{OK: false, LatencyMS: ms, Error: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		if msg == "" {
			msg = resp.Status
		}
		return TestResult{OK: false, LatencyMS: ms, Error: fmt.Sprintf("%d: %s", resp.StatusCode, msg)}
	}
	ids := parseModelIDs(body)
	return TestResult{OK: true, LatencyMS: ms, Models: ids}
}

func httpMeta(p Provider) (base, key string, client *http.Client, anthropic bool) {
	switch v := p.(type) {
	case *OpenAI:
		base = v.BaseURL
		if base == "" {
			base = "https://api.openai.com"
		}
		return base, v.APIKey, v.Client, false
	case *Anthropic:
		base = v.BaseURL
		if base == "" {
			base = "https://api.anthropic.com"
		}
		return base, v.APIKey, v.Client, true
	default:
		return "", "", nil, false
	}
}

func parseModelIDs(raw []byte) []string {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	ids := make([]string, 0, len(out.Data))
	for _, d := range out.Data {
		if d.ID != "" {
			ids = append(ids, d.ID)
		}
	}
	return ids
}
