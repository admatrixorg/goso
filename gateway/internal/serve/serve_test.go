// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package serve

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_WS_ORIGINS", "")
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
	var stats map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if _, ok := stats["last_heartbeat"]; ok {
		t.Fatalf("default off must omit last_heartbeat %#v", stats)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/system/heartbeat", strings.NewReader("{}")))
	if rr.Code != http.StatusOK {
		t.Fatalf("heartbeat %d %s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/stats", nil))
	if err := json.Unmarshal(rr.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	stamp, _ := stats["last_heartbeat"].(string)
	if stamp == "" {
		t.Fatalf("expected last_heartbeat after POST %#v", stats)
	}
}

func TestNewRequiresToken(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
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
	t.Setenv("GOSO_ENV", "demo")
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
		Providers []struct {
			Name   string `json:"name"`
			Type   string `json:"type"`
			KeySet bool   `json:"key_set"`
			Source string `json:"source"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, n := range body.Providers {
		got[n.Name] = true
		if n.Name == "groq" && (n.Type != "openai-compat" || !n.KeySet || n.Source != "env") {
			t.Fatalf("groq %+v", n)
		}
	}
	if !got["echo"] || !got["groq"] || got["openai"] || got["anthropic"] {
		t.Fatalf("providers %+v", body.Providers)
	}
	if strings.Contains(apiBody, `"api_key"`) {
		t.Fatal("leaked api_key")
	}

	v1 := httptest.NewRecorder()
	h.ServeHTTP(v1, httptest.NewRequest(http.MethodGet, "/v1/providers", nil))
	if v1.Code != rr.Code || v1.Body.String() != apiBody {
		t.Fatalf("GET /v1/providers %d %s vs /api/providers %d %s", v1.Code, v1.Body.String(), rr.Code, apiBody)
	}
}

func TestWebhookLLMBypassesAdmin(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
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
	t.Setenv("GOSO_ENV", "demo")
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

	req = httptest.NewRequest(http.MethodGet, "/api/pending-messages", nil)
	req.Header.Set("Authorization", "Bearer view-041")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("view GET pending %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/pending-messages/pg_1/compact", strings.NewReader(`{"confirm":"x"}`))
	req.Header.Set("Authorization", "Bearer view-041")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view POST compact %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/contacts", nil)
	req.Header.Set("Authorization", "Bearer view-041")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("view GET contacts %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/contacts/ct_1/merge", strings.NewReader(`{"source_id":"ct_2","confirm":"x"}`))
	req.Header.Set("Authorization", "Bearer view-041")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view POST merge %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	req.Header.Set("Authorization", "Bearer view-041")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("view GET nodes %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/nodes/nd_1/approve", strings.NewReader(`{"confirm":"x"}`))
	req.Header.Set("Authorization", "Bearer view-041")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view POST approve %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/nodes/nd_1/revoke", strings.NewReader(`{"confirm":"x"}`))
	req.Header.Set("Authorization", "Bearer view-041")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view POST revoke %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/workstations", nil)
	req.Header.Set("Authorization", "Bearer view-041")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("view GET workstations %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/workstations", strings.NewReader(`{"display":"x"}`))
	req.Header.Set("Authorization", "Bearer view-041")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view POST workstations %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/workstations/ws_1/delete", strings.NewReader(`{"confirm":"x"}`))
	req.Header.Set("Authorization", "Bearer view-041")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view POST workstation delete %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/storage", nil)
	req.Header.Set("Authorization", "Bearer view-041")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("view GET storage %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/storage/delete", strings.NewReader(`{"path":"x","confirm":"x"}`))
	req.Header.Set("Authorization", "Bearer view-041")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view POST storage delete %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/storage/upload", strings.NewReader("x"))
	req.Header.Set("Authorization", "Bearer view-041")
	req.Header.Set("Content-Type", "multipart/form-data")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view POST storage upload %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/activity", nil)
	req.Header.Set("Authorization", "Bearer view-041")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("view GET activity %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/activity", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer view-041")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view POST activity %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/system/backup", nil)
	req.Header.Set("Authorization", "Bearer view-041")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view GET backup %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/system/backup", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer view-041")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view POST backup %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/system/backup", strings.NewReader("{}"))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no token POST backup %d %s", rr.Code, rr.Body.String())
	}

	for _, path := range []string{"/api/kg/entities", "/api/skills", "/api/agents/x/evolution/tick"} {
		req = httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer view-041")
		req.Header.Set("Content-Type", "application/json")
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("view POST %s %d %s", path, rr.Code, rr.Body.String())
		}
	}
}

func TestPairing_ExchangeViewGrant(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_DEV_MODE", "")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-077")
	t.Setenv("GOSO_VIEW_TOKEN", "view-077")
	t.Setenv("GOSO_E2E_SCRIPTED", "")
	h, _ := New(store.New(), "test")

	req := httptest.NewRequest(http.MethodPost, "/api/pairing", nil)
	req.Header.Set("Authorization", "Bearer admin-077")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create pairing %d %s", rr.Code, rr.Body.String())
	}
	var created struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	ex := httptest.NewRequest(http.MethodPost, "/api/pairing/exchange", strings.NewReader(`{"code":"`+created.Code+`"}`))
	ex.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, ex)
	if rr.Code != http.StatusOK {
		t.Fatalf("exchange %d %s", rr.Code, rr.Body.String())
	}
	var grant struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&grant); err != nil {
		t.Fatal(err)
	}
	if grant.Token != "view-077" {
		t.Fatalf("token %q", grant.Token)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	get.Header.Set("Authorization", "Bearer "+grant.Token)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("view GET %d", rr.Code)
	}

	post := httptest.NewRequest(http.MethodPost, "/api/system/backup", strings.NewReader("{}"))
	post.Header.Set("Authorization", "Bearer "+grant.Token)
	post.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("view POST backup after exchange %d", rr.Code)
	}
}

func TestMaxBytesReader_API(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
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
	t.Setenv("GOSO_ENV", "demo")
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
	t.Setenv("GOSO_ENV", "demo")
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

func TestNew_ProductionRequiresWSOrigins(t *testing.T) {
	t.Setenv("GOSO_ENV", "production")
	t.Setenv("GOSO_WS_ORIGINS", "")
	t.Setenv("GOSO_DEV_MODE", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	old := fatalf
	called := false
	fatalf = func(format string, args ...any) {
		called = true
		panic(fmt.Sprintf(format, args...))
	}
	t.Cleanup(func() { fatalf = old })
	defer func() {
		_ = recover()
		if !called {
			t.Fatal("expected production refuse")
		}
	}()
	New(store.New(), "test")
	t.Fatal("New returned")
}

func TestNew_DemoAllowsEmptyWSOrigins(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_WS_ORIGINS", "")
	t.Setenv("GOSO_DEV_MODE", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	h, _ := New(store.New(), "test")
	if h == nil {
		t.Fatal("demo should boot")
	}
}

func TestNew_ProductionWithOrigins(t *testing.T) {
	t.Setenv("GOSO_ENV", "production")
	t.Setenv("GOSO_WS_ORIGINS", "https://app.example")
	t.Setenv("GOSO_DEV_MODE", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	h, _ := New(store.New(), "test")
	if h == nil {
		t.Fatal("production with origins should boot")
	}
}
