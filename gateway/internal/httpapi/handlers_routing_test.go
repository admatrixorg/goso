// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

type recordedChat struct {
	mu     sync.Mutex
	models []string
}

func (r *recordedChat) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		var body struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(b, &body)
		r.mu.Lock()
		r.models = append(r.models, body.Model)
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "ok"}}},
		})
	}
}

func (r *recordedChat) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.models))
	copy(out, r.models)
	return out
}

func TestChat_NamedProvidersHitOwnServers(t *testing.T) {
	t.Setenv("GOSO_SSRF", "")
	t.Setenv("GOSO_E2E_SCRIPTED", "")
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_DEV_MODE", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)

	var recA, recB, recEnv recordedChat
	srvA := httptest.NewServer(recA.handler())
	srvB := httptest.NewServer(recB.handler())
	srvEnv := httptest.NewServer(recEnv.handler())
	t.Cleanup(srvA.Close)
	t.Cleanup(srvB.Close)
	t.Cleanup(srvEnv.Close)

	t.Setenv("GOSO_LLM_PROVIDER", "router9")
	t.Setenv("GOSO_ROUTER9_BASE_URL", srvEnv.URL+"/v1")
	t.Setenv("GOSO_ROUTER9_MODEL", "env-model")

	st := store.New()
	if _, err := st.CreateLLMProvider(store.LLMProvider{Name: "p-a", Type: llm.TypeOpenAICompat, BaseURL: srvA.URL + "/v1", Model: "row-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLLMProvider(store.LLMProvider{Name: "p-b", Type: llm.TypeOpenAICompat, BaseURL: srvB.URL + "/v1", Model: "row-b"}); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Put(st, llm.APIKeySecretName("p-a"), []byte("k-a")); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Put(st, llm.APIKeySecretName("p-b"), []byte("k-b")); err != nil {
		t.Fatal(err)
	}
	agA, err := st.CreateAgent(store.Agent{AgentKey: "a", DisplayName: "A", LLMProvider: "p-a", Model: "model-a"})
	if err != nil {
		t.Fatal(err)
	}
	agB, err := st.CreateAgent(store.Agent{AgentKey: "b", DisplayName: "B", LLMProvider: "p-b", Model: "model-b"})
	if err != nil {
		t.Fatal(err)
	}
	agDef, err := st.CreateAgent(store.Agent{AgentKey: "def", DisplayName: "Default"})
	if err != nil {
		t.Fatal(err)
	}
	sessA, err := st.CreateSession(store.Session{AgentID: agA.ID})
	if err != nil {
		t.Fatal(err)
	}
	sessB, err := st.CreateSession(store.Session{AgentID: agB.ID})
	if err != nil {
		t.Fatal(err)
	}
	sessDef, err := st.CreateSession(store.Session{AgentID: agDef.ID})
	if err != nil {
		t.Fatal(err)
	}

	fallback := llm.NewRegistry().Preferred()
	rt := agent.New(st, nil, nil, nil, fallback)
	h := NewRouter(Options{Store: st, Runtime: rt, Provider: fallback, LLM: llm.NewRegistry()})
	postChat := func(sessionID string) int {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"session_id": sessionID, "message": "hi"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, req)
		return w.Code
	}
	if code := postChat(sessA.ID); code != http.StatusOK {
		t.Fatalf("chat A %d", code)
	}
	if code := postChat(sessB.ID); code != http.StatusOK {
		t.Fatalf("chat B %d", code)
	}
	if code := postChat(sessDef.ID); code != http.StatusOK {
		t.Fatalf("chat default %d", code)
	}
	if got := recA.seen(); len(got) != 1 || got[0] != "model-a" {
		t.Fatalf("server A models %v", got)
	}
	if got := recB.seen(); len(got) != 1 || got[0] != "model-b" {
		t.Fatalf("server B models %v", got)
	}
	if got := recEnv.seen(); len(got) != 1 || got[0] != "env-model" {
		t.Fatalf("env router9 should only see default agent, got %v", got)
	}
}

func TestAgents_UnknownLLMProvider400(t *testing.T) {
	_, h := newTestServer()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewBufferString(`{"agent_key":"x","llm_provider":"no-such"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create unknown provider %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewBufferString(`{"agent_key":"ok"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create empty provider %d %s", w.Code, w.Body.String())
	}
	var created store.Agent
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/agents/"+created.ID, bytes.NewBufferString(`{"llm_provider":"still-missing"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("patch unknown provider %d %s", w.Code, w.Body.String())
	}
}
