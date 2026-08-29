// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/webhook"
)

func tenantDo(h http.Handler, method, path, body, token, tenant string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if tenant != "" {
		req.Header.Set("X-Goso-Tenant", tenant)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestTenantIsolationAgentsSessionsWebhooks(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-071")
	t.Setenv("GOSO_VIEW_TOKEN", "")
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "t", Provider: llm.Echo{}})

	w := tenantDo(h, "POST", "/api/agents", `{"agent_key":"alpha","display_name":"Alpha"}`, "admin-071", "acme")
	if w.Code != 201 {
		t.Fatalf("create A %d %s", w.Code, w.Body.String())
	}
	var a map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	aid := a["id"].(string)
	if a["tenant_id"] != "acme" {
		t.Fatalf("agent tenant %v", a["tenant_id"])
	}

	w = tenantDo(h, "GET", "/api/agents", "", "admin-071", "beta")
	if w.Code != 200 {
		t.Fatalf("list B %d", w.Code)
	}
	var list map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	agents := list["agents"].([]any)
	if len(agents) != 0 {
		t.Fatalf("B saw A's agents: %v", list)
	}

	w = tenantDo(h, "GET", "/api/agents/"+aid, "", "admin-071", "beta")
	if w.Code != 404 {
		t.Fatalf("get B %d %s", w.Code, w.Body.String())
	}

	w = tenantDo(h, "GET", "/api/agents/"+aid, "", "admin-071", "acme")
	if w.Code != 200 {
		t.Fatalf("get A %d %s", w.Code, w.Body.String())
	}

	w = tenantDo(h, "POST", "/api/sessions", `{"agent_id":"`+aid+`","label":"s"}`, "admin-071", "acme")
	if w.Code != 201 {
		t.Fatalf("session A %d %s", w.Code, w.Body.String())
	}
	var sess map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	sid := sess["id"].(string)

	w = tenantDo(h, "GET", "/api/sessions", "", "admin-071", "beta")
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if sessions, _ := list["sessions"].([]any); len(sessions) != 0 {
		t.Fatalf("B saw sessions: %v", list)
	}

	w = tenantDo(h, "POST", "/api/chat", `{"session_id":"`+sid+`","message":"hi"}`, "admin-071", "beta")
	if w.Code != 404 {
		t.Fatalf("chat B %d %s", w.Code, w.Body.String())
	}

	w = tenantDo(h, "DELETE", "/api/sessions/"+sid, "", "admin-071", "beta")
	if w.Code != 404 {
		t.Fatalf("delete B %d %s", w.Code, w.Body.String())
	}
	w = tenantDo(h, "DELETE", "/api/sessions/"+sid, "", "admin-071", "acme")
	if w.Code != 200 {
		t.Fatalf("delete A %d %s", w.Code, w.Body.String())
	}

	w = tenantDo(h, "POST", "/api/webhooks", `{"name":"wh-a"}`, "admin-071", "acme")
	if w.Code != 201 {
		t.Fatalf("webhook A %d %s", w.Code, w.Body.String())
	}
	var created webhook.Created
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	w = tenantDo(h, "GET", "/api/webhooks", "", "admin-071", "beta")
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if hooks, _ := list["webhooks"].([]any); len(hooks) != 0 {
		t.Fatalf("B saw webhooks: %v", list)
	}
	w = tenantDo(h, "GET", "/api/webhooks/"+created.ID, "", "admin-071", "beta")
	if w.Code != 404 {
		t.Fatalf("get webhook B %d %s", w.Code, w.Body.String())
	}

	body := []byte(`{"input":"iso","mode":"async"}`)
	req := httptest.NewRequest("POST", "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatalf("async %d %s", w.Code, w.Body.String())
	}
	var accepted map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &accepted)
	jid, _ := accepted["id"].(string)
	if jid == "" {
		t.Fatalf("job id %v", accepted)
	}
	w = tenantDo(h, "GET", "/api/webhooks/jobs/"+jid, "", "admin-071", "beta")
	if w.Code != 404 {
		t.Fatalf("job B %d %s", w.Code, w.Body.String())
	}
	w = tenantDo(h, "GET", "/api/webhooks/jobs/"+jid, "", "admin-071", "acme")
	if w.Code != 200 {
		t.Fatalf("job A %d %s", w.Code, w.Body.String())
	}
}

func TestTenantHeaderIgnoredWhenDisabled(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-071")
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "t"})
	w := tenantDo(h, "POST", "/api/agents", `{"agent_key":"solo","display_name":"S"}`, "admin-071", "acme")
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var a map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	if a["tenant_id"] != "default" {
		t.Fatalf("header honored while disabled: %v", a["tenant_id"])
	}
	w = tenantDo(h, "GET", "/api/agents", "", "", "beta")
	if w.Code != 200 {
		t.Fatalf("list %d", w.Code)
	}
	var list map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if agents := list["agents"].([]any); len(agents) != 1 {
		t.Fatalf("default list %v", list)
	}
}

func TestTenantEmptyInvalidDefault(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-071")
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "t"})
	w := tenantDo(h, "POST", "/api/agents", `{"agent_key":"d1","display_name":"D"}`, "admin-071", "")
	if w.Code != 201 {
		t.Fatalf("empty %d %s", w.Code, w.Body.String())
	}
	var a map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	if a["tenant_id"] != "default" {
		t.Fatalf("empty → %v", a["tenant_id"])
	}
	w = tenantDo(h, "POST", "/api/agents", `{"agent_key":"d2","display_name":"D2"}`, "admin-071", "not valid!")
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	if a["tenant_id"] != "default" {
		t.Fatalf("invalid → %v", a["tenant_id"])
	}
}

func TestTenantNonDefaultRequiresAdmin(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-071")
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "t"})
	w := tenantDo(h, "POST", "/api/agents", `{"agent_key":"nope","display_name":"N"}`, "", "acme")
	if w.Code != 201 {
		t.Fatalf("no token %d %s", w.Code, w.Body.String())
	}
	var a map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	if a["tenant_id"] != "default" {
		t.Fatalf("non-admin non-default → %v", a["tenant_id"])
	}
}

func TestTenantViewTokenCannotSwitch(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-071")
	t.Setenv("GOSO_VIEW_TOKEN", "view-071")
	st := store.New()
	inner := NewRouter(Options{Store: st, Version: "t"})
	h := auth.RequireTokens("admin-071", "view-071", []string{"/healthz"})(inner)

	w := tenantDo(h, "POST", "/api/agents", `{"agent_key":"va","display_name":"VA"}`, "admin-071", "acme")
	if w.Code != 201 {
		t.Fatalf("admin create %d %s", w.Code, w.Body.String())
	}
	w = tenantDo(h, "GET", "/api/agents", "", "view-071", "acme")
	if w.Code != 200 {
		t.Fatalf("view GET %d %s", w.Code, w.Body.String())
	}
	var list map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if agents, _ := list["agents"].([]any); len(agents) != 0 {
		t.Fatalf("view token switched tenant: %v", list)
	}
	w = tenantDo(h, "POST", "/api/agents", `{"agent_key":"vb"}`, "view-071", "acme")
	if w.Code != http.StatusForbidden {
		t.Fatalf("view POST %d (want 403)", w.Code)
	}
}

func TestTenantEndpoint(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "")
	h := NewRouter(Options{Store: store.New(), Version: "t"})
	w := tenantDo(h, "GET", "/api/tenant", "", "", "")
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["tenant"] != "default" || body["multi_tenant"] != false {
		t.Fatalf("body %v", body)
	}
}
