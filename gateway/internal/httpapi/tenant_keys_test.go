// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/apikey"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/tenant"
)

func tenantKeyServer(t *testing.T) (http.Handler, *apikey.Registry, string, string) {
	t.Helper()
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-a1")
	st := store.New()
	if _, err := st.CreateAgent(store.Agent{AgentKey: "sale-minh-an", DisplayName: "Sale Minh An", TenantID: "minh-an"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(store.Agent{AgentKey: "sale-demo", DisplayName: "Sale Demo", TenantID: "demo-shop"}); err != nil {
		t.Fatal(err)
	}
	tenants := tenant.New()
	for _, id := range []string{"minh-an", "demo-shop"} {
		if _, err := tenants.Create(id, id); err != nil {
			t.Fatal(err)
		}
	}
	reg := apikey.New()
	inner := NewRouter(Options{Store: st, Version: "t", APIKeys: reg, Tenants: tenants})
	h := auth.Require(auth.Config{Admin: "admin-a1", Keys: reg})(inner)
	mk := func(name, tid string, scopes ...string) string {
		body, _ := json.Marshal(map[string]any{"name": name, "tenant_id": tid, "scopes": scopes})
		w := apiKeyJSON(t, h, "POST", "/api/api-keys", string(body), "admin-a1")
		if w.Code != http.StatusCreated {
			t.Fatalf("create key %d %s", w.Code, w.Body.String())
		}
		var c struct {
			Secret string `json:"secret"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &c)
		return c.Secret
	}
	return h, reg, mk("minh-an-reader", "minh-an", "read"), mk("minh-an-admin", "minh-an", "admin")
}

func withTenant(t *testing.T, h http.Handler, method, path, token, tid string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if tid != "" {
		req.Header.Set(tenant.Header, tid)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// A key sees only its own tenant's agents, with or without the header.
func TestTenantKeySeesOnlyItsTenant(t *testing.T) {
	h, _, reader, _ := tenantKeyServer(t)
	for _, hdr := range []string{"", "minh-an"} {
		w := withTenant(t, h, "GET", "/api/agents", reader, hdr)
		if w.Code != http.StatusOK {
			t.Fatalf("header %q: %d %s", hdr, w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, "sale-minh-an") || strings.Contains(body, "sale-demo") {
			t.Fatalf("header %q: wrong agents %s", hdr, body)
		}
	}
}

// Naming another tenant in the header is refused, not silently rebound.
func TestTenantKeyCannotSwitchTenant(t *testing.T) {
	h, _, reader, admin := tenantKeyServer(t)
	for _, tok := range []string{reader, admin} {
		for _, hdr := range []string{"demo-shop", "default", "bad tenant!"} {
			w := withTenant(t, h, "GET", "/api/agents", tok, hdr)
			if w.Code != http.StatusForbidden {
				t.Fatalf("header %q: %d %s", hdr, w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "sale-demo") {
				t.Fatal("leaked the other tenant")
			}
		}
	}
	// The operator token still switches tenants as before.
	w := withTenant(t, h, "GET", "/api/agents", "admin-a1", "demo-shop")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "sale-demo") {
		t.Fatalf("operator switch %d %s", w.Code, w.Body.String())
	}
}

// A tenant key, even with admin scope, cannot reach gateway-wide paths:
// every key, every tenant, packages, backup.
func TestTenantKeyCannotReachGatewayWidePaths(t *testing.T) {
	h, _, _, admin := tenantKeyServer(t)
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/api-keys"},
		{"POST", "/api/api-keys"},
		{"GET", "/api/tenants"},
		{"POST", "/api/tenants"},
		{"POST", "/api/system/backup"},
		{"POST", "/api/pairing"},
	} {
		w := withTenant(t, h, c.method, c.path, admin, "")
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s: %d %s", c.method, c.path, w.Code, w.Body.String())
		}
	}
	// Inside its tenant, admin scope still writes.
	req := httptest.NewRequest("POST", "/api/agents", strings.NewReader(`{"agent_key":"cskh-minh-an","display_name":"CSKH"}`))
	req.Header.Set("Authorization", "Bearer "+admin)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"tenant_id":"minh-an"`) {
		t.Fatalf("tenant write %d %s", w.Code, w.Body.String())
	}
}

func TestAPIKeyNeedsExistingTenant(t *testing.T) {
	h, _, _, _ := tenantKeyServer(t)
	w := apiKeyJSON(t, h, "POST", "/api/api-keys", `{"name":"x","tenant_id":"khong-ton-tai","scopes":["read"]}`, "admin-a1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown tenant %d %s", w.Code, w.Body.String())
	}
}

// Keys survive a restart: a new registry loaded from the same database
// accepts the key, keeps its tenant, and keeps a revoked key revoked.
func TestAPIKeysSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goso.db")
	st, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := apikey.Load(st)
	if err != nil || reg == nil {
		t.Fatalf("load %v %v", reg, err)
	}
	keep, err := reg.Create(apikey.Input{Name: "giu", TenantID: "minh-an", Scopes: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	gone, err := reg.Create(apikey.Input{Name: "thu-hoi", TenantID: "minh-an", Scopes: []string{"write"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Revoke(gone.ID, gone.ID); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	st2, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	again, err := apikey.Load(st2)
	if err != nil || again == nil {
		t.Fatalf("reload %v %v", again, err)
	}
	g, ok := again.Accept(keep.Secret)
	if !ok || g.TenantID != "minh-an" || !g.Has("read") {
		t.Fatalf("kept key after restart: %v %+v", ok, g)
	}
	if _, ok := again.Accept(gone.Secret); ok {
		t.Fatal("revoked key works after restart")
	}
	if p, err := again.Get(gone.ID); err != nil || p.Status != apikey.StatusRevoked {
		t.Fatalf("revoked row %v %+v", err, p)
	}
	next, err := again.Create(apikey.Input{Name: "moi", Scopes: []string{"read"}})
	if err != nil || next.ID == keep.ID || next.ID == gone.ID {
		t.Fatalf("new id collides: %v %+v", err, next)
	}
	for _, p := range again.List("") {
		raw, _ := json.Marshal(p)
		if strings.Contains(string(raw), keep.Secret) {
			t.Fatal("secret stored")
		}
	}
}
