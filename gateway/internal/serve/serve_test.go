// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package serve

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/webhook"
)

func TestNewHealthzAndAgent(t *testing.T) {
	t.Setenv("GOSO_DEV_MODE", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	t.Setenv("GOSO_E2E_SCRIPTED", "")
	st := store.New()
	h, status := New(st, "test")
	if !status.DevMode {
		t.Fatal("expected explicit GOSO_DEV_MODE passthrough")
	}
	if status.Provider == "" {
		t.Fatal("expected provider name")
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz status %d", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != true {
		t.Fatalf("healthz body %+v", body)
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents", strings.NewReader(`{"agent_key":"desk","display_name":"Desktop"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create agent status %d body %s", rr.Code, rr.Body.String())
	}
	list := st.ListAgents()
	if len(list) != 1 || list[0].AgentKey != "desk" {
		t.Fatalf("store not reused: %+v", list)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/stats", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("stats status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestNewRequiresToken(t *testing.T) {
	t.Setenv("GOSO_DEV_MODE", "")
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	st := store.New()
	h, status := New(st, "test")
	if status.Auth || status.DevMode {
		t.Fatalf("status %+v", status)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("empty token /api/agents %d body %s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/providers", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("empty token /v1/providers %d body %s", rr.Code, rr.Body.String())
	}

	t.Setenv("GOSO_ADMIN_TOKEN", "secret-016")
	h, status = New(st, "test")
	if !status.Auth {
		t.Fatal("expected auth on")
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer %d", rr.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer secret-016")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bearer /api/agents %d %s", rr.Code, rr.Body.String())
	}
}

func TestProvidersListsConfigured(t *testing.T) {
	t.Setenv("GOSO_DEV_MODE", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	t.Setenv("GOSO_E2E_SCRIPTED", "")
	t.Setenv("GOSO_ANTHROPIC_API_KEY", "")
	t.Setenv("GOSO_OPENAI_API_KEY", "")
	t.Setenv("GOSO_OPENROUTER_API_KEY", "")
	t.Setenv("GOSO_GROQ_API_KEY", "k-groq")
	t.Setenv("GOSO_DEEPSEEK_API_KEY", "")
	t.Setenv("GOSO_GEMINI_API_KEY", "")
	t.Setenv("GOSO_MISTRAL_API_KEY", "")
	t.Setenv("GOSO_XAI_API_KEY", "")
	t.Setenv("GOSO_MINIMAX_API_KEY", "")
	t.Setenv("GOSO_DASHSCOPE_API_KEY", "")
	t.Setenv("GOSO_ROUTER9_BASE_URL", "")
	t.Setenv("GOSO_ROUTER9_API_KEY", "")
	t.Setenv("GOSO_LLM_PROVIDER", "")
	h, status := New(store.New(), "test")
	if status.Provider != "groq" {
		t.Fatalf("default provider %s", status.Provider)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/providers", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	apiBody := rr.Body.String()
	var body struct {
		Providers []string `json:"providers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, n := range body.Providers {
		got[n] = true
	}
	if !got["echo"] || !got["groq"] || got["openai"] || got["anthropic"] {
		t.Fatalf("providers %+v", body.Providers)
	}

	v1 := httptest.NewRecorder()
	h.ServeHTTP(v1, httptest.NewRequest(http.MethodGet, "/v1/providers", nil))
	if v1.Code != rr.Code || v1.Body.String() != apiBody {
		t.Fatalf("GET /v1/providers %d %s vs /api/providers %d %s", v1.Code, v1.Body.String(), rr.Code, apiBody)
	}
}

func TestWebhookLLMBypassesAdmin(t *testing.T) {
	t.Setenv("GOSO_DEV_MODE", "")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-040")
	t.Setenv("GOSO_E2E_SCRIPTED", "")
	h, _ := New(store.New(), "test")

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks", nil)
	req.Header.Set("Authorization", "Bearer admin-040")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create webhook %d %s", rr.Code, rr.Body.String())
	}
	var created webhook.Created
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	body := bytes.NewReader([]byte(`{"input":"ping","mode":"sync"}`))
	req = httptest.NewRequest(http.MethodPost, "/api/webhooks/llm", body)
	req.Header.Set("Authorization", "Bearer "+created.Token)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("llm webhook %d %s", rr.Code, rr.Body.String())
	}
}

func TestViewToken_GETOnly(t *testing.T) {
	t.Setenv("GOSO_DEV_MODE", "")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-041")
	t.Setenv("GOSO_VIEW_TOKEN", "view-041")
	t.Setenv("GOSO_E2E_SCRIPTED", "")
	h, _ := New(store.New(), "test")

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer view-041")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("view GET agents %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer view-041")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("view GET sessions %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"session_id":"x","message":"hi"}`))
	req.Header.Set("Authorization", "Bearer view-041")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view POST chat %d %s", rr.Code, rr.Body.String())
	}
}

func TestMaxBytesReader_API(t *testing.T) {
	t.Setenv("GOSO_DEV_MODE", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	t.Setenv("GOSO_E2E_SCRIPTED", "")
	h, _ := New(store.New(), "test")
	body := bytes.Repeat([]byte("a"), security.MaxAPIBody+8)
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code == http.StatusCreated {
		t.Fatalf("oversized body must not create: %d %s", rr.Code, rr.Body.String())
	}
}

func TestChannelsListsSeven(t *testing.T) {
	t.Setenv("GOSO_DEV_MODE", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	t.Setenv("GOSO_E2E_SCRIPTED", "")
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("GOSO_DISCORD_BOT_TOKEN", "")
	t.Setenv("GOSO_SLACK_BOT_TOKEN", "")
	t.Setenv("GOSO_FEISHU_APP_SECRET", "")
	t.Setenv("GOSO_WHATSAPP_ACCESS_TOKEN", "")
	h, _ := New(store.New(), "test")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/channels", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Channels []struct {
			Name string `json:"name"`
		} `json:"channels"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Channels) != 7 {
		t.Fatalf("channels %+v", body.Channels)
	}
}

func TestLoadConnectors_AppliesStoredToken(t *testing.T) {
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	t.Setenv("GOSO_DEV_MODE", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	t.Setenv("GOSO_E2E_SCRIPTED", "")
	t.Setenv("GOSO_ANTHROPIC_API_KEY", "")
	t.Setenv("GOSO_OPENAI_API_KEY", "")
	t.Setenv("GOSO_GROQ_API_KEY", "")
	t.Setenv("GOSO_ROUTER9_BASE_URL", "")
	t.Setenv("GOSO_LLM_PROVIDER", "")

	var sawAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("GET /manifest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":"1.0","tools":[{"name":"ping","description":"p","requires_approval":false,"input_schema":{"type":"object"}}]}`))
	})
	mux.HandleFunc("POST /tools/ping", func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	fake := httptest.NewServer(mux)
	defer fake.Close()

	st := store.New()
	h, _ := New(st, "test")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/connectors", strings.NewReader(`{"name":"crm","transport":"http","endpoint":"`+fake.URL+`","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/connectors/crm", strings.NewReader(`{"token":"reload-token-value"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch %d %s", rr.Code, rr.Body.String())
	}

	h2, _ := New(st, "test")
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/tools/invoke", strings.NewReader(`{"connector":"crm","tool":"ping","arguments":{}}`))
	req.Header.Set("Content-Type", "application/json")
	h2.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("invoke %d %s", rr.Code, rr.Body.String())
	}
	if sawAuth != "Bearer reload-token-value" {
		t.Fatalf("auth after reload %q", sawAuth)
	}
}
