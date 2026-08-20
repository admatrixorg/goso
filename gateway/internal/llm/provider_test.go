// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestRegistry(t *testing.T) {
	t.Setenv("GOSO_ANTHROPIC_API_KEY", "")
	t.Setenv("GOSO_OPENAI_API_KEY", "")
	r := NewRegistry()
	if r.HasReal() {
		t.Fatal("expected no real provider")
	}
	if r.Get("anthropic").Name() != "echo" {
		t.Fatal("expected echo fallback")
	}
	t.Setenv("GOSO_ANTHROPIC_API_KEY", "k1")
	r = NewRegistry()
	if !r.HasReal() || r.Get("anthropic").Name() != "anthropic" {
		t.Fatal("expected anthropic")
	}
}
