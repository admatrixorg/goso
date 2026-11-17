// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/builtin"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestToolSettingsRoutesTenantScoped(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-ts")
	t.Setenv("GOSO_MASTER_KEY", strings.Repeat("cd", 32))
	for _, k := range []string{"GOSO_VISION_BASE_URL", "GOSO_VISION_API_KEY", "GOSO_VISION_PROVIDER", "GOSO_IMAGE_URL", "GOSO_IMAGE_API_KEY", "GOSO_STT_URL", "GOSO_STT_API_KEY", "GOSO_VAULT_DOCLING"} {
		t.Setenv(k, "")
	}
	st := store.New()
	prev := builtin.ProviderStore
	builtin.ProviderStore = st
	t.Cleanup(func() { builtin.ProviderStore = prev })
	for _, p := range []store.LLMProvider{
		{Name: "gem-a", TenantID: "shop-a", Type: "openai-compat", BaseURL: "https://gemini.example.test/v1beta/openai", Model: "gemini-2.5-flash", Enabled: true},
		{Name: "router-a", TenantID: "shop-a", Type: "openai-compat", BaseURL: "https://router.example.test/api/v1", Model: "m", Enabled: false},
		{Name: "gem-b", TenantID: "shop-b", Type: "openai-compat", BaseURL: "https://gemini.example.test/v1beta/openai", Model: "gemini-2.5-flash", Enabled: true},
	} {
		if _, err := st.CreateLLMProvider(p); err != nil {
			t.Fatal(err)
		}
	}
	agent, _ := st.CreateAgent(store.Agent{AgentKey: "sale", TenantID: "shop-a"})
	al := auditlog.New(100)
	h := NewRouter(Options{Store: st, Version: "t", Audit: al})
	do := func(method, path, tenant, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer admin-ts")
		req.Header.Set("X-Goso-Tenant", tenant)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	decode := func(rec *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("decode %d %s", rec.Code, rec.Body.String())
		}
		return m
	}

	rec := do(http.MethodGet, "/api/tools/builtin/read_video/settings", "shop-a", "")
	if m := decode(rec); rec.Code != http.StatusOK || m["source"] != "none" || m["settings"] != nil {
		t.Fatalf("empty %d %v", rec.Code, m)
	}

	body := `{"providers":[{"provider":"gem-a","model":"gemini-2.5-flash","timeout":120,"params":{"api":"gemini"}},{"provider":"router-a","model":"google/gemini-2.5-flash"}]}`
	rec = do(http.MethodPut, "/api/tools/builtin/read_video/settings", "shop-a", body)
	m := decode(rec)
	if rec.Code != http.StatusOK || m["source"] != "tenant" || m["tool"] != "read_video" {
		t.Fatalf("put %d %v", rec.Code, m)
	}
	if w, _ := m["warnings"].([]any); len(w) != 1 || !strings.Contains(w[0].(string), "router-a") {
		t.Fatalf("disabled-provider warning %v", m["warnings"])
	}
	provs := m["settings"].(map[string]any)["providers"].([]any)
	if len(provs) != 2 || provs[0].(map[string]any)["provider"] != "gem-a" {
		t.Fatalf("settings %v", m["settings"])
	}

	// Alias: stt edits read_audio; /v1 prefix works.
	if rec := do(http.MethodPut, "/v1/tools/builtin/stt/settings", "shop-a", `{"providers":[{"provider":"gem-a"}]}`); rec.Code != http.StatusOK || decode(rec)["tool"] != "read_audio" {
		t.Fatalf("alias %d %s", rec.Code, rec.Body.String())
	}

	// Rejections.
	for name, c := range map[string]struct {
		path, body string
		code       int
	}{
		"foreign provider": {"/api/tools/builtin/read_image/settings", `{"providers":[{"provider":"gem-b"}]}`, http.StatusBadRequest},
		"unknown provider": {"/api/tools/builtin/read_image/settings", `{"providers":[{"provider":"nope"}]}`, http.StatusBadRequest},
		"key in params":    {"/api/tools/builtin/read_image/settings", `{"providers":[{"provider":"gem-a","params":{"api_key":"x"}}]}`, http.StatusBadRequest},
		"unknown field":    {"/api/tools/builtin/read_image/settings", `{"providers":[{"provider":"gem-a","base_url":"https://x.test"}]}`, http.StatusBadRequest},
		"bad json":         {"/api/tools/builtin/read_image/settings", `{`, http.StatusBadRequest},
		"not a chain tool": {"/api/tools/builtin/web_search/settings", `{"providers":[]}`, http.StatusNotFound},
	} {
		if rec := do(http.MethodPut, c.path, "shop-a", c.body); rec.Code != c.code {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}

	// Tenant b sees none of tenant a's settings.
	rec = do(http.MethodGet, "/api/tools/builtin/settings", "shop-b", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "gem-a") {
		t.Fatalf("shop-b list %d %s", rec.Code, rec.Body.String())
	}
	tools := decode(rec)["tools"].([]any)
	if len(tools) != len(builtin.ChainTools) {
		t.Fatalf("list size %d", len(tools))
	}
	if rec := do(http.MethodDelete, "/api/tools/builtin/read_video/settings", "shop-b", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("shop-b delete %d", rec.Code)
	}
	rec = do(http.MethodGet, "/api/tools/builtin/settings", "shop-a", "")
	if !strings.Contains(rec.Body.String(), `"read_video"`) || !strings.Contains(rec.Body.String(), "gem-a") {
		t.Fatalf("shop-a list %s", rec.Body.String())
	}

	// The agent tool list reports read_video as configured for shop-a only.
	rec = do(http.MethodGet, "/api/agents/"+agent.ID+"/tools", "shop-a", "")
	if !toolListed(t, rec, "read_video", true) {
		t.Fatalf("agent tools %s", rec.Body.String())
	}
	if !builtin.ConfiguredForTenant("shop-a", "read_video") || builtin.ConfiguredForTenant("shop-b", "read_video") {
		t.Fatal("configured must follow the tenant chain")
	}

	// Audit holds names only.
	raw, _ := json.Marshal(al.Query(auditlog.Query{Entity: "builtin_tool_settings"}))
	if !strings.Contains(string(raw), "builtin_tool_settings") || strings.Contains(string(raw), `"timeout"`) {
		t.Fatalf("audit %s", raw)
	}

	if rec := do(http.MethodDelete, "/api/tools/builtin/read_video/settings", "shop-a", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete %d", rec.Code)
	}
	if m := decode(do(http.MethodGet, "/api/tools/builtin/read_video/settings", "shop-a", "")); m["source"] != "none" {
		t.Fatalf("after delete %v", m)
	}
	if rec := do(http.MethodDelete, "/api/tools/builtin/read_video/settings", "shop-a", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete %d", rec.Code)
	}
}

func toolListed(t *testing.T, rec *httptest.ResponseRecorder, name string, configured bool) bool {
	t.Helper()
	var out struct {
		Tools []map[string]any `json:"tools"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		return false
	}
	for _, tool := range out.Tools {
		if tool["name"] == name {
			return tool["configured"] == configured
		}
	}
	return false
}
