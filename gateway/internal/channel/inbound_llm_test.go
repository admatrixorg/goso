// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	fallbackModel = "ocg/deepseek-v4-flash"
	agentModel    = "gcli/grok-4.5"
)

func startChatCapture(t *testing.T) (baseURL string, gotModel *string) {
	t.Helper()
	var model string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		model = payload.Model
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "from-compat"}}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &model
}

func fallbackOpenAI(base, model string) llm.Provider {
	return &llm.OpenAI{APIKey: "fallback-key", Model: model, BaseURL: base, Label: "fallback"}
}

func setRouter9Capture(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("GOSO_LLM_PROVIDER", "router9")
	t.Setenv("GOSO_ROUTER9_BASE_URL", baseURL)
	t.Setenv("GOSO_ROUTER9_MODEL", fallbackModel)
	t.Setenv("GOSO_ROUTER9_API_KEY", "")
}

func TestResolveInboundLLM_EmptyKeepsFallback(t *testing.T) {
	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "telegram", DisplayName: "Telegram Bot"})
	if err != nil {
		t.Fatal(err)
	}
	got := resolveInboundLLM(st, a, llm.Echo{})
	if got.Name() != "echo" {
		t.Fatalf("name %s", got.Name())
	}
}

func TestResolveInboundLLM_NamedMissKeepsFallback(t *testing.T) {
	st := store.New()
	a, err := st.CreateAgent(store.Agent{
		AgentKey: "telegram", DisplayName: "Telegram Bot",
		LLMProvider: "nope", Model: agentModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := resolveInboundLLM(st, a, llm.Echo{})
	if got.Name() != "echo" {
		t.Fatalf("name %s", got.Name())
	}
}

func TestResolveInboundLLM_AppliesAgentModel(t *testing.T) {
	setRouter9Capture(t, "http://127.0.0.1:9/v1")
	st := store.New()
	a, err := st.CreateAgent(store.Agent{
		AgentKey: "telegram", DisplayName: "Telegram Bot",
		LLMProvider: "router9", Model: agentModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	fb := &llm.OpenAI{APIKey: "k", Model: fallbackModel, BaseURL: "http://127.0.0.1:8", Label: "fallback"}
	got := resolveInboundLLM(st, a, fb)
	o, ok := got.(*llm.OpenAI)
	if !ok || o.Model != agentModel || o.Label != "router9" {
		t.Fatalf("%#v", got)
	}
	if fb.Model != fallbackModel {
		t.Fatalf("mutated fallback %q", fb.Model)
	}
}
