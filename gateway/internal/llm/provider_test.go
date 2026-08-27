// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEcho(t *testing.T) {
	p := Echo{}
	s, _ := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if s != "echo: hi" {
		t.Fatalf("echo %q", s)
	}
}

func TestAnthropic_Mock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("api key header %q", r.Header.Get("x-api-key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "hello from claude"}},
		})
	}))
	defer srv.Close()
	p := &Anthropic{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client()}
	reply, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil || reply != "hello from claude" {
		t.Fatalf("anthropic %v %q", err, reply)
	}
}

func TestOpenAI_Mock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "hello gpt"}}},
		})
	}))
	defer srv.Close()
	p := &OpenAI{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client()}
	reply, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil || reply != "hello gpt" {
		t.Fatalf("openai %v %q", err, reply)
	}
}

func TestOpenAI_UsageFromProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "ok"}}},
			"usage":   map[string]int{"prompt_tokens": 11, "completion_tokens": 7},
		})
	}))
	defer srv.Close()
	p := &OpenAI{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client()}
	reply, u, err := p.ChatUsage(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil || reply != "ok" {
		t.Fatalf("chat %v %q", err, reply)
	}
	if u.Estimated || u.PromptTokens != 11 || u.CompletionTokens != 7 {
		t.Fatalf("usage %+v", u)
	}
}

func TestAnthropic_UsageFromProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "ok"}},
			"usage":   map[string]int{"input_tokens": 9, "output_tokens": 3},
		})
	}))
	defer srv.Close()
	p := &Anthropic{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client()}
	reply, u, err := p.ChatUsage(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil || reply != "ok" {
		t.Fatalf("chat %v %q", err, reply)
	}
	if u.Estimated || u.PromptTokens != 9 || u.CompletionTokens != 3 {
		t.Fatalf("usage %+v", u)
	}
	if u.CacheReadTokens != 0 {
		t.Fatalf("cache_read_tokens default %d", u.CacheReadTokens)
	}
}

func TestAnthropic_CacheReadTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "cached"}},
			"usage": map[string]int{
				"input_tokens":            9,
				"output_tokens":           3,
				"cache_read_input_tokens": 4,
			},
		})
	}))
	defer srv.Close()
	p := &Anthropic{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client()}
	_, u, err := p.ChatUsage(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if u.CacheReadTokens != 4 {
		t.Fatalf("cache_read_tokens %d", u.CacheReadTokens)
	}
}

func TestEstimateUsage_WhenProviderOmits(t *testing.T) {
	u := EstimateUsage([]Message{{Role: "user", Content: "abcd"}}, "abcdefgh")
	if !u.Estimated || u.PromptTokens != 1 || u.CompletionTokens != 2 {
		t.Fatalf("estimate %+v", u)
	}
	if u.CacheReadTokens != 0 {
		t.Fatalf("estimate cache_read_tokens %d", u.CacheReadTokens)
	}
}

func clearProviderEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GOSO_ANTHROPIC_API_KEY", "")
	t.Setenv("GOSO_ANTHROPIC_MODEL", "")
	t.Setenv("GOSO_ANTHROPIC_CACHE_MODE", "")
	t.Setenv("GOSO_OPENAI_API_KEY", "")
	t.Setenv("GOSO_OPENAI_MODEL", "")
	t.Setenv("GOSO_LLM_PROVIDER", "")
	for _, c := range OpenAICompatProviders() {
		t.Setenv(c.EnvKey, "")
		t.Setenv(c.EnvModel, "")
		if c.EnvURL != "" {
			t.Setenv(c.EnvURL, "")
		}
	}
}

func TestRegistry(t *testing.T) {
	clearProviderEnv(t)
	r := NewRegistry()
	if r.HasReal() {
		t.Fatal("expected no real provider")
	}
	if r.Get("anthropic").Name() != "echo" {
		t.Fatal("expected echo fallback")
	}
	if r.Preferred().Name() != "echo" {
		t.Fatal("expected preferred echo")
	}
	got := map[string]bool{}
	for _, n := range r.List() {
		got[n] = true
	}
	if !got["echo"] || len(got) != 1 {
		t.Fatalf("list %+v", r.List())
	}
	t.Setenv("GOSO_ANTHROPIC_API_KEY", "k1")
	r = NewRegistry()
	if !r.HasReal() || r.Get("anthropic").Name() != "anthropic" {
		t.Fatal("expected anthropic")
	}
}

func TestRegistry_NamedCompat(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("GOSO_GROQ_API_KEY", "k-groq")
	r := NewRegistry()
	if !r.HasReal() || r.Get("groq").Name() != "groq" {
		t.Fatal("expected groq")
	}
	if r.Get("openrouter").Name() != "echo" {
		t.Fatal("empty openrouter must be absent")
	}
	if r.Preferred().Name() != "groq" {
		t.Fatalf("preferred %s", r.Preferred().Name())
	}
	p, ok := r.Get("groq").(*OpenAI)
	if !ok || p.BaseURL != "https://api.groq.com/openai" {
		t.Fatalf("groq %+v", p)
	}
}

func TestRegistry_PreferredAnthropicOverNamed(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("GOSO_ANTHROPIC_API_KEY", "k1")
	t.Setenv("GOSO_GROQ_API_KEY", "k-groq")
	r := NewRegistry()
	if r.Preferred().Name() != "anthropic" {
		t.Fatalf("preferred %s", r.Preferred().Name())
	}
}

func TestOpenAICompatCatalog(t *testing.T) {
	want := []string{"openrouter", "groq", "deepseek", "gemini", "mistral", "xai", "minimax", "dashscope", "router9"}
	got := OpenAICompatProviders()
	if len(got) != len(want) {
		t.Fatalf("len %d", len(got))
	}
	for i, spec := range got {
		if spec.Name != want[i] {
			t.Fatalf("name[%d] %s", i, spec.Name)
		}
		if spec.BaseURL == "" || spec.EnvKey == "" || spec.Model == "" {
			t.Fatalf("incomplete %+v", spec)
		}
		if spec.Name != "router9" && spec.AllowEmptyKey {
			t.Fatalf("%s must skip empty key", spec.Name)
		}
	}
}

func TestRegistry_Router9ConstructsOnURLEmptyKey(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("GOSO_ROUTER9_BASE_URL", "http://127.0.0.1:20127/v1")
	t.Setenv("GOSO_ROUTER9_API_KEY", "")
	r := NewRegistry()
	p, ok := r.Get("router9").(*OpenAI)
	if !ok || p.Label != "router9" {
		t.Fatalf("router9 %+v", r.Get("router9"))
	}
	if p.APIKey != "" {
		t.Fatal("key must stay empty")
	}
	if p.BaseURL != "http://127.0.0.1:20127/v1" {
		t.Fatalf("base %s", p.BaseURL)
	}
	if p.ModelName() != "cx/gpt-5.6-sol" {
		t.Fatalf("model %s", p.ModelName())
	}
	if !p.AllowEmptyKey {
		t.Fatal("AllowEmptyKey")
	}
	if p.Client == nil || p.Client.Timeout < 120*time.Second {
		t.Fatalf("timeout %+v", p.Client)
	}
	found := false
	for _, n := range r.List() {
		if n == "router9" {
			found = true
		}
	}
	if !found {
		t.Fatalf("list %v", r.List())
	}
	if r.Preferred().Name() != "router9" {
		t.Fatalf("preferred %s", r.Preferred().Name())
	}
}

func TestRegistry_Router9AbsentWithoutURL(t *testing.T) {
	clearProviderEnv(t)
	r := NewRegistry()
	if r.Get("router9").Name() != "echo" {
		t.Fatal("router9 must be absent without BASE_URL")
	}
}

func TestRegistry_LLMProviderOverride(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("GOSO_ROUTER9_BASE_URL", "http://127.0.0.1:20127/v1")
	t.Setenv("GOSO_GROQ_API_KEY", "k-groq")
	t.Setenv("GOSO_LLM_PROVIDER", "groq")
	r := NewRegistry()
	if r.Preferred().Name() != "groq" {
		t.Fatalf("override %s", r.Preferred().Name())
	}
	t.Setenv("GOSO_LLM_PROVIDER", "missing")
	r = NewRegistry()
	if r.Preferred().Name() != "router9" {
		t.Fatalf("unknown override falls to router9, got %s", r.Preferred().Name())
	}
}

func TestChatCompletionsURL(t *testing.T) {
	if got := chatCompletionsURL("http://127.0.0.1:20127/v1"); got != "http://127.0.0.1:20127/v1/chat/completions" {
		t.Fatalf("v1 suffix %s", got)
	}
	if got := chatCompletionsURL("https://api.groq.com/openai"); got != "https://api.groq.com/openai/v1/chat/completions" {
		t.Fatalf("groq %s", got)
	}
	if got := chatCompletionsURL("https://openrouter.ai/api/"); got != "https://openrouter.ai/api/v1/chat/completions" {
		t.Fatalf("slash %s", got)
	}
}
