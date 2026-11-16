// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestHookRoutesTenantScopedAndSecretHidden(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-hooks")
	t.Setenv("GOSO_SSRF", "0")
	t.Setenv("GOSO_MASTER_KEY", strings.Repeat("cd", 32))
	st := store.New()
	sale, _ := st.CreateAgent(store.Agent{AgentKey: "sale", TenantID: "minh-an"})
	other, _ := st.CreateAgent(store.Agent{AgentKey: "sale", TenantID: "demo-shop"})
	al := auditlog.New(100)
	h := NewRouter(Options{Store: st, Version: "t", Audit: al})

	do := func(method, path, tenant, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer admin-hooks")
		req.Header.Set("X-Goso-Tenant", tenant)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := do(http.MethodPost, "/api/hooks", "minh-an", `{"event":"PostToolUse","matcher":"zalocrm__*","handler_type":"http","config":{"url":"https://hooks.example.test/goso?token=abc"},"secret":"demo-signing-secret","agent_id":"`+sale.ID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "demo-signing-secret") || strings.Contains(rec.Body.String(), "secret_enc") {
		t.Fatalf("secret echoed: %s", rec.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	id, _ := created["id"].(string)
	if id == "" || created["has_secret"] != true || created["tenant_id"] != "minh-an" || created["enabled"] != true {
		t.Fatalf("created %v", created)
	}

	// /v1 alias, same tenant: listed, secret hidden.
	rec = do(http.MethodGet, "/v1/hooks", "minh-an", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), id) || strings.Contains(rec.Body.String(), "demo-signing-secret") {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	// Other tenant sees nothing and cannot read, change or delete it.
	rec = do(http.MethodGet, "/api/hooks", "demo-shop", "")
	if strings.Contains(rec.Body.String(), id) {
		t.Fatalf("other tenant listed it: %s", rec.Body.String())
	}
	for _, m := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
		if rec := do(m, "/api/hooks/"+id, "demo-shop", `{"enabled":false}`); rec.Code != http.StatusNotFound {
			t.Fatalf("other tenant %s %d", m, rec.Code)
		}
	}
	// Agent of another tenant is refused.
	if rec := do(http.MethodPost, "/api/hooks", "minh-an", `{"event":"Stop","handler_type":"http","config":{"url":"https://hooks.example.test"},"agent_id":"`+other.ID+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("foreign agent %d", rec.Code)
	}
	// Validation.
	for _, body := range []string{
		`{"event":"Stop","handler_type":"block"}`,
		`{"event":"Nope","handler_type":"http","config":{"url":"https://hooks.example.test"}}`,
		`{"event":"PreToolUse","handler_type":"http","config":{"url":"ftp://hooks.example.test"}}`,
		`{"event":"Stop","matcher":"x*","handler_type":"http","config":{"url":"https://hooks.example.test"}}`,
		`{"event":"PreToolUse","matcher":"[","handler_type":"block"}`,
		`{"event":"PreToolUse","handler_type":"shell"}`,
	} {
		if rec := do(http.MethodPost, "/api/hooks", "minh-an", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("accepted %s: %d %s", body, rec.Code, rec.Body.String())
		}
	}

	// Patch keeps the secret unless told otherwise.
	rec = do(http.MethodPatch, "/api/hooks/"+id, "minh-an", `{"enabled":false}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"has_secret":true`) || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("patch %d %s", rec.Code, rec.Body.String())
	}
	rec = do(http.MethodPatch, "/api/hooks/"+id, "minh-an", `{"secret":""}`)
	if !strings.Contains(rec.Body.String(), `"has_secret":false`) {
		t.Fatalf("clear secret %s", rec.Body.String())
	}
	if rec := do(http.MethodDelete, "/api/hooks/"+id, "minh-an", ""); rec.Code != http.StatusOK {
		t.Fatalf("delete %d", rec.Code)
	}
	if rec := do(http.MethodGet, "/api/hooks/"+id, "minh-an", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("after delete %d", rec.Code)
	}

	page := al.Query(auditlog.Query{Entity: "hook"})
	actions := map[string]int{}
	for _, r := range page.Records {
		actions[r.Action]++
		raw, _ := json.Marshal(r)
		if strings.Contains(string(raw), "demo-signing-secret") || strings.Contains(string(raw), "token=abc") {
			t.Fatalf("audit leaked: %s", raw)
		}
	}
	if actions["create"] != 1 || actions["update"] != 2 || actions["delete"] != 1 {
		t.Fatalf("audit actions %v", actions)
	}
}

func TestHookRoutesSSRFAndMasterKey(t *testing.T) {
	t.Setenv("GOSO_SSRF", "1")
	t.Setenv("GOSO_MASTER_KEY", "")
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "t"})
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/hooks", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for _, u := range []string{"http://127.0.0.1:9000/x", "http://169.254.169.254/latest", "http://localhost/x"} {
		if rec := post(`{"event":"Stop","handler_type":"http","config":{"url":"` + u + `"}}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "not allowed") {
			t.Fatalf("ssrf %s: %d %s", u, rec.Code, rec.Body.String())
		}
	}
	if rec := post(`{"event":"PreToolUse","handler_type":"block","config":{"message":"tạm khoá"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("block hook %d %s", rec.Code, rec.Body.String())
	}
	if rec := post(`{"event":"Stop","handler_type":"http","config":{"url":"https://93.184.216.34/x"},"secret":"s"}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "GOSO_MASTER_KEY") {
		t.Fatalf("secret without master key %d %s", rec.Code, rec.Body.String())
	}
}

func TestTenantAgentLinksRoute(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-links")
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "sale", TenantID: "minh-an"})
	b, _ := st.CreateAgent(store.Agent{AgentKey: "kho", TenantID: "minh-an"})
	c, _ := st.CreateAgent(store.Agent{AgentKey: "kho", TenantID: "demo-shop"})
	_ = st.AddAgentLink(a.ID, b.ID)
	h := NewRouter(Options{Store: st, Version: "t"})
	do := func(method, path, tenant, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer admin-links")
		req.Header.Set("X-Goso-Tenant", tenant)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec := do(http.MethodGet, "/api/agent-links", "minh-an", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), b.ID) {
		t.Fatalf("tenant links %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/api/agent-links", "demo-shop", ""); strings.Contains(rec.Body.String(), a.ID) {
		t.Fatalf("other tenant sees links %s", rec.Body.String())
	}
	if rec := do(http.MethodPost, "/api/agents/"+a.ID+"/links", "minh-an", `{"to_agent_id":"`+c.ID+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("cross-tenant link %d %s", rec.Code, rec.Body.String())
	}
	if st.HasAgentLink(a.ID, c.ID) {
		t.Fatal("cross-tenant link stored")
	}
}
