// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAI_NamedLabelAndChatTools(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		n++
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			payload := map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"tool_calls": []map[string]any{{
							"id":   "call_1",
							"type": "function",
							"function": map[string]any{
								"name":      "lookup",
								"arguments": `{"q":"hi"}`,
							},
						}},
					},
				}},
			}
			_ = json.NewEncoder(w).Encode(payload)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "done"}}},
		})
	}))
	defer srv.Close()
	p := &OpenAI{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client(), Label: "groq"}
	if p.Name() != "groq" {
		t.Fatalf("name %s", p.Name())
	}
	first, err := p.ChatTools(t.Context(), []Message{{Role: "user", Content: "hi"}}, []ToolSpec{{Name: "lookup"}})
	if err != nil || len(first.ToolCalls) != 1 || first.ToolCalls[0].Name != "lookup" {
		t.Fatalf("tool_use %v %+v", err, first)
	}
	if q, _ := first.ToolCalls[0].Arguments["q"].(string); q != "hi" {
		t.Fatalf("args %+v", first.ToolCalls[0].Arguments)
	}
	second, err := p.ChatTools(t.Context(), []Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: first.ToolCalls},
		{Role: "tool", ToolCallID: "call_1", Content: "ok"},
	}, []ToolSpec{{Name: "lookup"}})
	if err != nil || second.Text != "done" {
		t.Fatalf("text %v %+v", err, second)
	}
}

func TestOpenAI_Router9EmptyKeyAndV1Base(t *testing.T) {
	var sawPath, sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}` + "\n\ndata: [DONE]\n"))
	}))
	defer srv.Close()
	p := &OpenAI{
		APIKey: "", BaseURL: srv.URL + "/v1", Client: srv.Client(),
		Label: "router9", AllowEmptyKey: true,
	}
	reply, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}})
	if err != nil || reply != "pong" {
		t.Fatalf("chat %v %q", err, reply)
	}
	if sawPath != "/v1/chat/completions" {
		t.Fatalf("path %s (must not be /v1/v1/chat/completions)", sawPath)
	}
	if sawAuth != "" {
		t.Fatalf("auth %q", sawAuth)
	}
}

func TestOpenAI_GroqStillAppendsV1(t *testing.T) {
	var sawPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		if r.Header.Get("Authorization") != "Bearer k-groq" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "ok"}}},
		})
	}))
	defer srv.Close()
	p := &OpenAI{APIKey: "k-groq", BaseURL: srv.URL, Client: srv.Client(), Label: "groq"}
	reply, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}})
	if err != nil || reply != "ok" {
		t.Fatalf("groq %v %q", err, reply)
	}
	if sawPath != "/v1/chat/completions" {
		t.Fatalf("path %s", sawPath)
	}
	if _, err := (&OpenAI{Label: "groq"}).Chat(t.Context(), []Message{{Role: "user", Content: "hi"}}); err == nil || !strings.Contains(err.Error(), "missing API key") {
		t.Fatalf("empty groq key: %v", err)
	}
}

func TestAnthropic_CacheControlFull(t *testing.T) {
	t.Setenv("GOSO_PROMPT_CACHE", "")
	t.Setenv("GOSO_ANTHROPIC_CACHE_MODE", "")
	var rawBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		b, _ := json.Marshal(body)
		rawBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "ok"}},
		})
	}))
	defer srv.Close()

	p := &Anthropic{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client(), CacheMode: "full"}
	msgs := []Message{
		{Role: "system", Content: "identity"},
		{Role: "system", Content: "SOUL.md:\nsoul"},
		{Role: "assistant", Content: "prev"},
		{Role: "user", Content: "hi"},
	}
	if _, err := p.Chat(t.Context(), msgs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rawBody, "cache_control") {
		t.Fatal("expected cache_control in JSON when mode=full")
	}
	if !strings.Contains(rawBody, `"text":"identity"`) || !strings.Contains(rawBody, "SOUL.md") {
		t.Fatalf("stable prefix blocks missing: %s", rawBody)
	}
	if strings.Count(rawBody, `"cache_control"`) < 3 {
		t.Fatalf("want cache_control on system+bootstrap+last non-user, got %s", rawBody)
	}
	if strings.Contains(rawBody, `"role":"user","content":[{`) {
		t.Fatalf("user turn must not be cached: %s", rawBody)
	}

	rawBody = ""
	p.CacheMode = "full"
	sumMsgs := []Message{
		{Role: "system", Content: "identity"},
		{Role: "system", Content: "SOUL.md:\nsoul"},
		{Role: "system", Content: "Previous summary: old turn"},
		{Role: "user", Content: "hi"},
	}
	if _, err := p.Chat(t.Context(), sumMsgs); err != nil {
		t.Fatal(err)
	}
	if strings.Count(rawBody, `"cache_control"`) != 2 {
		t.Fatalf("summary must not be a cache breakpoint, got %s", rawBody)
	}

	rawBody = ""
	p.CacheMode = "none"
	if _, err := p.Chat(t.Context(), msgs); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rawBody, "cache_control") {
		t.Fatal("none CacheMode must omit cache_control")
	}

	rawBody = ""
	p.CacheMode = "bogus"
	if _, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rawBody, "cache_control") {
		t.Fatal("bogus CacheMode must not include cache_control")
	}
}

func TestAnthropic_PromptCacheEnvFull(t *testing.T) {
	t.Setenv("GOSO_ANTHROPIC_CACHE_MODE", "")
	t.Setenv("GOSO_PROMPT_CACHE", "full")
	var sawCache bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body)
		sawCache = strings.Contains(string(raw), "cache_control")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "ok"}},
		})
	}))
	defer srv.Close()
	p := &Anthropic{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client()}
	if _, err := p.Chat(t.Context(), []Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "hi"}}); err != nil {
		t.Fatal(err)
	}
	if !sawCache {
		t.Fatal("GOSO_PROMPT_CACHE=full must include cache_control")
	}
}

func TestOpenAI_NoFakeCacheControl(t *testing.T) {
	t.Setenv("GOSO_ANTHROPIC_CACHE_MODE", "full")
	t.Setenv("GOSO_PROMPT_CACHE", "full")
	var rawBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rawBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "ok"}}},
		})
	}))
	defer srv.Close()
	p := &OpenAI{APIKey: "k", BaseURL: srv.URL, Client: srv.Client()}
	if _, err := p.Chat(t.Context(), []Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "hi"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rawBody, "cache_control") {
		t.Fatalf("openai must not fake cache_control: %s", rawBody)
	}
}
