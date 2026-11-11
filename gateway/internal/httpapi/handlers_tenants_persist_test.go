// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/tenant"
)

func TestTenants_PersistRestartAndSaveFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenants.db")
	st, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := tenant.Load(st)
	if err != nil || reg == nil {
		t.Fatalf("load %v %v", reg, err)
	}
	h := GuardDeactivatedTenant(reg, NewRouter(Options{Store: st, Version: "t", Tenants: reg, Provider: llm.Echo{}}))

	w := tenantJSON(t, h, http.MethodPost, "/api/tenants", `{"slug":"minh-an","name":"Minh An"}`, "", "")
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	assertNoTenantSecrets(t, w.Body.String())
	w = tenantJSON(t, h, http.MethodPost, "/api/tenants/minh-an/members", `{"subject":"covan.phaply@minh-an.test","role":"owner"}`, "", "")
	if w.Code != http.StatusCreated {
		t.Fatalf("member %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, http.MethodPost, "/api/tenants", `{"slug":"phap-che","name":"Pháp chế"}`, "", "")
	if w.Code != http.StatusCreated {
		t.Fatalf("phap %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h, http.MethodPost, "/api/tenants/phap-che/status", `{"status":"deactivated","confirm":"phap-che"}`, "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	reg2, err := tenant.Load(st2)
	if err != nil {
		t.Fatal(err)
	}
	h2 := GuardDeactivatedTenant(reg2, NewRouter(Options{Store: st2, Version: "t", Tenants: reg2, Provider: llm.Echo{}}))
	w = tenantJSON(t, h2, http.MethodGet, "/api/tenants/minh-an", "", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}
	assertNoTenantSecrets(t, w.Body.String())
	var row map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	if row["name"] != "Minh An" || row["status"] != "active" || row["master"] != false {
		t.Fatalf("row %#v", row)
	}
	members, _ := row["members"].([]any)
	if len(members) != 1 {
		t.Fatalf("members %#v", row["members"])
	}
	mem, _ := members[0].(map[string]any)
	if mem["subject"] != "covan.phaply@minh-an.test" || mem["role"] != "owner" {
		t.Fatalf("member %#v", mem)
	}
	w = tenantJSON(t, h2, http.MethodGet, "/api/tenants/phap-che", "", "", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"deactivated"`) && !strings.Contains(w.Body.String(), `"status": "deactivated"`) {
		t.Fatalf("phap %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h2, http.MethodPost, "/api/tenants/default/status", `{"status":"deactivated","confirm":"default"}`, "", "")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "cannot deactivate master") {
		t.Fatalf("master %d %s", w.Code, w.Body.String())
	}

	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-minh-an")
	w = tenantJSON(t, h2, http.MethodPost, "/api/agents", `{"agent_key":"minh-an-sales","display_name":"Cố vấn Minh An"}`, "admin-minh-an", "phap-che")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "tenant deactivated") {
		t.Fatalf("deactivated write %d %s", w.Code, w.Body.String())
	}
	w = tenantJSON(t, h2, http.MethodPost, "/api/agents", `{"agent_key":"minh-an-sales","display_name":"Cố vấn Minh An"}`, "admin-minh-an", "kinh-doanh")
	if w.Code != http.StatusCreated {
		t.Fatalf("unregistered write %d %s", w.Code, w.Body.String())
	}

	w = tenantJSON(t, h, http.MethodPost, "/api/tenants", `{"slug":"kinh-doanh","name":"Kinh doanh"}`, "", "")
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "tenant registry save failed") {
		t.Fatalf("save %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "covan.phaply") || strings.Contains(w.Body.String(), "Kinh doanh") {
		t.Fatalf("body leaked %s", w.Body.String())
	}
	w = tenantJSON(t, h, http.MethodGet, "/api/tenants/kinh-doanh", "", "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("memory create %d %s", w.Code, w.Body.String())
	}
}
