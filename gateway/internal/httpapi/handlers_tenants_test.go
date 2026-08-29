// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/tenant"
)

func tenantsServer(t *testing.T) (*tenant.Registry, *auditlog.Store, http.Handler) {
	t.Helper()
	reg := tenant.New()
	al := auditlog.New(64)
	inner := NewRouter(Options{Store: store.New(), Version: "t", Tenants: reg, Audit: al, Provider: llm.Echo{}})
	return reg, al, GuardDeactivatedTenant(reg, inner)
}

func tenantJSON(t *testing.T, h http.Handler, method, path, body, token, tid string) *httptest.ResponseRecorder {
	t.Helper()
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
	if tid != "" {
		req.Header.Set("X-Goso-Tenant", tid)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func assertNoTenantSecrets(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, n := range []string{`"token"`, `"api_key"`, `"password"`, `"secret"`, `"authorization"`, `"bearer `} {
		if strings.Contains(lower, n) {
			t.Fatalf("secret field in body: %s", body)
		}
	}
	if strings.Contains(body, "sk-") || strings.Contains(body, "gsk_") {
		t.Fatalf("token shape in body: %s", body)
	}
}

func TestTenants_ListMasterContextAndV1(t *testing.T) {
	_, _, h := tenantsServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tenants", nil))
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	assertNoTenantSecrets(t, w.Body.String())
	var listed struct {
		Tenants []map[string]any `json:"tenants"`
		Current map[string]any   `json:"current"`
		Master  map[string]any   `json:"master"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Tenants) != 1 || listed.Tenants[0]["id"] != "default" {
		t.Fatalf("list %#v", listed)
	}
	if listed.Current["id"] != "default" || listed.Master["id"] != "default" {
		t.Fatalf("context %#v", listed)
	}
	assertSameGET(t, h, "/api/tenants", "/v1/tenants")
	assertSameGET(t, h, "/api/tenant", "/v1/tenant")
}

func TestTenants_CreateStatusMembersAudit(t *testing.T) {
	_, al, h := tenantsServer(t)
	w := tenantJSON(t, h, "POST", "/api/tenants", `{"slug":"acme","name":"Acme"}`, "", "")
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	assertNoTenantSecrets(t, w.Body.String())
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created["id"] != "acme" || created["status"] != "active" {
		t.Fatalf("created %#v", created)
	}

	w = tenantJSON(t, h, "GET", "/api/tenants?q=acme", "", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"acme"`) {
		t.Fatalf("search %d %s", w.Code, w.Body.String())
	}

	w = tenantJSON(t, h, "POST", "/api/tenants/acme/members", `{"subject":"ops@acme.test","role":"admin"}`, "", "")
	if w.Code != 201 {
		t.Fatalf("member %d %s", w.Code, w.Body.String())
	}
	assertNoTenantSecrets(t, w.Body.String())
	var detail struct {
		Members []map[string]any `json:"members"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &detail)
	if len(detail.Members) != 1 {
		t.Fatalf("members %#v", detail.Members)
	}
	mid, _ := detail.Members[0]["id"].(string)

	w = tenantJSON(t, h, "PATCH", "/api/tenants/acme/members/"+mid, `{"role":"viewer"}`, "", "")
	if w.Code != 200 {
		t.Fatalf("role %d %s", w.Code, w.Body.String())
	}

	w = tenantJSON(t, h, "POST", "/api/tenants/acme/status", `{"status":"deactivated"}`, "", "")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirm is required") {
		t.Fatalf("no confirm %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, "POST", "/api/tenants/acme/status", `{"status":"deactivated","confirm":"nope"}`, "", "")
	if w.Code != 400 {
		t.Fatalf("mismatch %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, "POST", "/api/tenants/default/status", `{"status":"deactivated","confirm":"default"}`, "", "")
	if w.Code != 409 {
		t.Fatalf("master %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, "POST", "/api/tenants/acme/status", `{"status":"deactivated","confirm":"acme"}`, "", "")
	if w.Code != 200 {
		t.Fatalf("deactivate %d %s", w.Code, w.Body.String())
	}

	w = tenantJSON(t, h, "DELETE", "/api/tenants/acme/members/"+mid, `{"confirm":"ops@acme.test"}`, "", "")
	if w.Code != 200 {
		t.Fatalf("remove %d %s", w.Code, w.Body.String())
	}

	page := al.Query(auditlog.Query{Entity: "tenant"})
	if page.Total < 4 {
		t.Fatalf("audit %d %#v", page.Total, page.Records)
	}
	for _, rec := range page.Records {
		if rec.After["token"] != nil || rec.After["api_key"] != nil {
			t.Fatalf("audit secret %#v", rec)
		}
	}

	w = tenantJSON(t, h, "POST", "/api/tenants", `{"slug":"x","name":"sk-live-abcdefgh"}`, "", "")
	if w.Code != 400 {
		t.Fatalf("secret name %d %s", w.Code, w.Body.String())
	}
}

func TestTenants_DeactivatedBlocksWritesStillIsolated(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-112")
	reg := tenant.New()
	h := GuardDeactivatedTenant(reg, NewRouter(Options{Store: store.New(), Version: "t", Tenants: reg, Provider: llm.Echo{}}))

	w := tenantJSON(t, h, "POST", "/api/tenants", `{"slug":"acme","name":"Acme"}`, "admin-112", "")
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, "POST", "/api/agents", `{"agent_key":"alpha","display_name":"A"}`, "admin-112", "acme")
	if w.Code != 201 {
		t.Fatalf("agent before %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, "POST", "/api/tenants/acme/status", `{"status":"deactivated","confirm":"Acme"}`, "admin-112", "")
	if w.Code != 200 {
		t.Fatalf("deactivate %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, "POST", "/api/agents", `{"agent_key":"beta","display_name":"B"}`, "admin-112", "acme")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "tenant deactivated") {
		t.Fatalf("write %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, "GET", "/api/agents", "", "admin-112", "acme")
	if w.Code != 200 {
		t.Fatalf("get isolated %d %s", w.Code, w.Body.String())
	}
	var list map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	agents, _ := list["agents"].([]any)
	if len(agents) != 1 {
		t.Fatalf("acme agents %#v", list)
	}
	w = tenantJSON(t, h, "GET", "/api/agents", "", "admin-112", "beta")
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if agents, _ = list["agents"].([]any); len(agents) != 0 {
		t.Fatalf("beta saw acme %#v", list)
	}
}

func TestTenants_ViewTokenGETOnly(t *testing.T) {
	reg := tenant.New()
	inner := GuardDeactivatedTenant(reg, NewRouter(Options{Store: store.New(), Version: "t", Tenants: reg}))
	h := auth.RequireTokens("admin-112", "view-112", []string{"/healthz"})(inner)

	w := tenantJSON(t, h, "GET", "/api/tenants", "", "view-112", "")
	if w.Code != 200 {
		t.Fatalf("view GET list %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, "GET", "/api/tenant", "", "view-112", "")
	if w.Code != 200 {
		t.Fatalf("view GET context %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, "GET", "/v1/tenants", "", "view-112", "")
	if w.Code != 200 {
		t.Fatalf("view GET v1 %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, "POST", "/api/tenants", `{"slug":"x","name":"X"}`, "view-112", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("view POST %d %s", w.Code, w.Body.String())
	}
}

func TestTenants_InvalidPathDoesNotMutateMaster(t *testing.T) {
	_, _, h := tenantsServer(t)
	w := tenantJSON(t, h, "GET", "/api/tenants/!!!", "", "", "")
	if w.Code != 404 {
		t.Fatalf("get invalid %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, "POST", "/api/tenants/%20/members", `{"subject":"ops@acme.test","role":"admin"}`, "", "")
	if w.Code != 404 {
		t.Fatalf("member invalid %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, "GET", "/api/tenants/default", "", "", "")
	if w.Code != 200 {
		t.Fatalf("master %d %s", w.Code, w.Body.String())
	}
	var detail struct {
		Members []any `json:"members"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &detail)
	if len(detail.Members) != 0 {
		t.Fatalf("master members %#v", detail.Members)
	}
}

func TestTenantEndpointIncludesMaster(t *testing.T) {
	_, _, h := tenantsServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tenant", nil))
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	assertNoTenantSecrets(t, w.Body.String())
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["tenant"] != "default" || body["master_id"] != "default" || body["master"] != true {
		t.Fatalf("body %v", body)
	}
}
