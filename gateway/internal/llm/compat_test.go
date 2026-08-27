// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"encoding/json"
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

func TestAnthropic_CacheControlFull(t *testing.T) {
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

	p := &Anthropic{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client(), CacheMode: "full"}
	if _, err := p.Chat(t.Context(), []Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "hi"}}); err != nil {
		t.Fatal(err)
	}
	if !sawCache {
		t.Fatal("expected cache_control in JSON when mode=full")
	}

	sawCache = false
	p.CacheMode = "bogus"
	if _, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatal(err)
	}
	if sawCache {
		t.Fatal("bogus CacheMode must not include cache_control")
	}
}
