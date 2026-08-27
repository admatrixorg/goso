// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestAgentTools_ListAndPatchBuiltin(t *testing.T) {
	t.Setenv("GOSO_WEB_SEARCH", "")
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "0.1.0"})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents/missing/tools", nil))
	if w.Code != 404 {
		t.Fatalf("missing agent %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents", bytes.NewBufferString(`{"agent_key":"a1","display_name":"A1"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("agent %d %s", w.Code, w.Body.String())
	}
	var agent map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &agent)
	id := agent["id"].(string)

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents/"+id+"/tools", nil))
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Tools []struct {
			Name             string `json:"name"`
			Connector        string `json:"connector"`
			RequiresApproval bool   `json:"requires_approval"`
			Enabled          bool   `json:"enabled"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	var sawSearch, sawSandbox bool
	for _, tl := range listed.Tools {
		if tl.Name == "web_search" {
			sawSearch = true
			if tl.Connector != "builtin" || tl.Enabled || tl.RequiresApproval {
				t.Fatalf("web_search %+v", tl)
			}
		}
		if tl.Name == "sandbox" {
			sawSandbox = true
			if !tl.RequiresApproval || tl.Enabled {
				t.Fatalf("sandbox %+v", tl)
			}
		}
	}
	if !sawSearch || !sawSandbox {
		t.Fatalf("builtins missing %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/agents/"+id+"/tools/web_search", bytes.NewBufferString(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("patch %d %s", w.Code, w.Body.String())
	}
	if !st.GetToolFlag("web_search") {
		t.Fatal("flag not persisted")
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/tools/invoke", bytes.NewBufferString(`{"connector":"builtin","tool":"web_search","arguments":{"q":"x"}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "not_configured") {
		t.Fatalf("invoke without env %d %s", w.Code, w.Body.String())
	}
}

func TestPatchConnector_TokenNeverReturned(t *testing.T) {
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "0.1.0"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/connectors", bytes.NewBufferString(`{"name":"crm","transport":"http","endpoint":"http://127.0.0.1:9","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/connectors/crm", bytes.NewBufferString(`{"endpoint":"http://127.0.0.1:8","token":"super-secret-token-value","enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("patch %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "super-secret-token-value") {
		t.Fatal("token leaked in PATCH")
	}
	var rec map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &rec)
	if rec["token_set"] != true {
		t.Fatalf("token_set %+v", rec)
	}
	if rec["enabled"] != false {
		t.Fatalf("enabled %+v", rec)
	}
	if rec["endpoint"] != "http://127.0.0.1:8" {
		t.Fatalf("endpoint %+v", rec)
	}
	if rec["token"] != nil {
		t.Fatalf("token field %v", rec["token"])
	}
	cred, _ := rec["credential_ref"].(string)
	if cred != "" && cred != "***" {
		t.Fatalf("credential_ref not masked %q", cred)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/connectors/crm", nil))
	if strings.Contains(w.Body.String(), "super-secret-token-value") {
		t.Fatal("token leaked in GET")
	}
	_ = json.Unmarshal(w.Body.Bytes(), &rec)
	if rec["token_set"] != true {
		t.Fatalf("get token_set %+v", rec)
	}
}

func TestAgentTools_PatchConnectorBoundTool(t *testing.T) {
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "0.1.0"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents", bytes.NewBufferString(`{"agent_key":"a1","display_name":"A1"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	var agent map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &agent)
	id := agent["id"].(string)

	manifest := `{"schema_version":"1.0","tools":[{"name":"contact_search","description":"search","requires_approval":false,"input_schema":{"type":"object"}}]}`
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("GET /manifest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(manifest))
	})
	fake := httptest.NewServer(mux)
	defer fake.Close()

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/connectors", bytes.NewBufferString(`{"name":"zalocrm","transport":"http","endpoint":"`+fake.URL+`","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/agents/"+id+"/connectors", bytes.NewBufferString(`{"connector":"zalocrm"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("link %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/agents/"+id+"/tools/contact_search", bytes.NewBufferString(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("patch tool %d %s", w.Code, w.Body.String())
	}
	got, _ := st.GetConnector("zalocrm")
	if got.Enabled {
		t.Fatal("connector should be disabled")
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents/"+id+"/tools", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"contact_search"`) {
		t.Fatalf("disabled connector tools must still list %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("expected enabled false %s", w.Body.String())
	}
}

func TestPatchConnector_Missing(t *testing.T) {
	h := NewRouter(Options{Store: store.New(), Version: "0.1.0"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/connectors/nope", bytes.NewBufferString(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestPatchConnector_TokenRequiresMasterKey(t *testing.T) {
	t.Setenv("GOSO_MASTER_KEY", "")
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "0.1.0"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/connectors", bytes.NewBufferString(`{"name":"crm","transport":"http","endpoint":"http://127.0.0.1:9","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/connectors/crm", bytes.NewBufferString(`{"token":"super-secret-token-value"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "master key") {
		t.Fatalf("want 400 master key, got %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/connectors/crm", nil))
	if strings.Contains(w.Body.String(), `"token_set":true`) {
		t.Fatalf("must not fake token_set %s", w.Body.String())
	}
}

func TestPatchAgentTool_EnabledRequired(t *testing.T) {
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "0.1.0"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents", bytes.NewBufferString(`{"agent_key":"a1","display_name":"A1"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	var agent map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &agent)
	id := agent["id"].(string)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/agents/"+id+"/tools/web_search", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("omitted enabled %d %s", w.Code, w.Body.String())
	}
}
