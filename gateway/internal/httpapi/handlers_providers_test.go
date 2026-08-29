// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func clearLLMEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GOSO_ANTHROPIC_API_KEY", "")
	t.Setenv("GOSO_ANTHROPIC_MODEL", "")
	t.Setenv("GOSO_OPENAI_API_KEY", "")
	t.Setenv("GOSO_OPENAI_MODEL", "")
	t.Setenv("GOSO_LLM_PROVIDER", "")
	t.Setenv("GOSO_MASTER_KEY", "")
	for _, spec := range llm.OpenAICompatProviders() {
		t.Setenv(spec.EnvKey, "")
		t.Setenv(spec.EnvModel, "")
		if spec.EnvURL != "" {
			t.Setenv(spec.EnvURL, "")
		}
	}
}

func providerRouter(t *testing.T, st store.StoreIface) http.Handler {
	t.Helper()
	return NewRouter(Options{Store: st, Version: "0.1.0", LLM: llm.NewRegistry()})
}

func TestProviders_EmptyEnvIncludesEcho(t *testing.T) {
	clearLLMEnv(t)
	h := providerRouter(t, store.New())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/providers", nil))
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Providers []llm.ProviderInfo `json:"providers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Providers) != 1 || body.Providers[0].Name != "echo" || body.Providers[0].Type != "echo" || body.Providers[0].Source != "env" {
		t.Fatalf("want echo only, got %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"api_key"`) {
		t.Fatal("leaked api_key")
	}
}

func TestProviders_Router9WhenURLSet(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("GOSO_ROUTER9_BASE_URL", "http://127.0.0.1:20127/v1")
	h := providerRouter(t, store.New())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/providers", nil))
	var body struct {
		Providers []llm.ProviderInfo `json:"providers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := map[string]llm.ProviderInfo{}
	for _, p := range body.Providers {
		got[p.Name] = p
	}
	if _, ok := got["echo"]; !ok {
		t.Fatal("echo")
	}
	r9, ok := got["router9"]
	if !ok || r9.Type != "router9" || r9.Source != "env" || r9.Model != "ocg/deepseek-v4-flash" {
		t.Fatalf("router9 %+v", r9)
	}
}

func TestProviders_CRUDTestConnection(t *testing.T) {
	clearLLMEnv(t)
	master, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", master)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer unit-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"bad key"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "gpt-unit"}, {"id": "gpt-alt"}},
		})
	}))
	defer srv.Close()

	st := store.New()
	h := providerRouter(t, st)

	create := `{"name":"acme","type":"openai-compat","base_url":"` + srv.URL + `","model":"gpt-unit","api_key":"unit-key"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/providers", bytes.NewBufferString(create))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "unit-key") || strings.Contains(w.Body.String(), `"api_key"`) {
		t.Fatal("create leaked secret")
	}
	var created llm.ProviderInfo
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Name != "acme" || !created.KeySet || created.Source != "sqlite" || !created.Enabled {
		t.Fatalf("created %+v", created)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/providers", nil))
	var listed struct {
		Providers []llm.ProviderInfo `json:"providers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	var acme *llm.ProviderInfo
	for i := range listed.Providers {
		if listed.Providers[i].Name == "acme" {
			acme = &listed.Providers[i]
		}
	}
	if acme == nil || !acme.KeySet || acme.Source != "sqlite" {
		t.Fatalf("list acme %+v %s", acme, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "unit-key") || strings.Contains(w.Body.String(), `"api_key"`) {
		t.Fatal("GET leaked secret")
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/providers/acme", bytes.NewBufferString(`{"model":"gpt-alt","api_key":""}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("patch %d %s", w.Code, w.Body.String())
	}
	var patched llm.ProviderInfo
	if err := json.Unmarshal(w.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Model != "gpt-alt" || !patched.KeySet {
		t.Fatalf("empty api_key must leave key set: %+v", patched)
	}
	gotKey, err := secrets.Get(st, llm.APIKeySecretName("acme"))
	if err != nil || string(gotKey) != "unit-key" {
		t.Fatalf("secret unchanged %v %q", err, gotKey)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/providers/acme/test", bytes.NewBufferString(`{"kind":"models"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("test %d %s", w.Code, w.Body.String())
	}
	var tr llm.TestResult
	if err := json.Unmarshal(w.Body.Bytes(), &tr); err != nil {
		t.Fatal(err)
	}
	if !tr.OK || len(tr.Models) != 2 || tr.Models[0] != "gpt-unit" {
		t.Fatalf("test result %+v", tr)
	}

	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer badSrv.Close()
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/providers/acme", bytes.NewBufferString(`{"base_url":"`+badSrv.URL+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("patch url %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/providers/acme/test", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &tr); err != nil {
		t.Fatal(err)
	}
	if tr.OK || tr.Error == "" {
		t.Fatalf("401 must be ok:false, got %+v", tr)
	}
}

func TestProviders_NoMasterKeyWithAPIKey400(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("GOSO_MASTER_KEY", "")
	h := providerRouter(t, store.New())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/providers", bytes.NewBufferString(
		`{"name":"locked","type":"openai-compat","api_key":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "master key required") {
		t.Fatalf("want 400 master key required, got %d %s", w.Code, w.Body.String())
	}
}

func TestProviders_PatchMissing404(t *testing.T) {
	clearLLMEnv(t)
	h := providerRouter(t, store.New())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/providers/nope", bytes.NewBufferString(`{"model":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("patch missing %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/providers/nope/test", bytes.NewBufferString(`{}`)))
	if w.Code != 404 {
		t.Fatalf("test missing %d %s", w.Code, w.Body.String())
	}
}

func TestProviders_EnvWinsOverSQLite(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("GOSO_ROUTER9_BASE_URL", "http://127.0.0.1:20127/v1")
	st := store.New()
	if _, err := st.CreateLLMProvider(store.LLMProvider{Name: "router9", Type: "openai-compat", Model: "hijack"}); err != nil {
		t.Fatal(err)
	}
	h := providerRouter(t, st)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/providers", nil))
	var body struct {
		Providers []llm.ProviderInfo `json:"providers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, p := range body.Providers {
		if p.Name == "router9" {
			if p.Source != "env" || p.Model == "hijack" {
				t.Fatalf("env must win: %+v", p)
			}
			return
		}
	}
	t.Fatal("router9 missing")
}

func TestProviders_PostEchoConflict(t *testing.T) {
	clearLLMEnv(t)
	h := providerRouter(t, store.New())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/providers", bytes.NewBufferString(`{"name":"echo","type":"echo"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Fatalf("echo exists %d %s", w.Code, w.Body.String())
	}
}

func TestProviders_DeleteKeyNeverLeakAndEnvWins(t *testing.T) {
	clearLLMEnv(t)
	master, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", master)
	t.Setenv("GOSO_ROUTER9_BASE_URL", "http://127.0.0.1:20127/v1")

	st := store.New()
	h := providerRouter(t, st)

	create := `{"name":"acme","type":"openai-compat","base_url":"http://127.0.0.1:9","model":"m","api_key":"unit-key"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/providers", bytes.NewBufferString(create))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/providers/acme", bytes.NewBufferString(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("disable %d %s", w.Code, w.Body.String())
	}
	var disabled llm.ProviderInfo
	if err := json.Unmarshal(w.Body.Bytes(), &disabled); err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled || !disabled.KeySet {
		t.Fatalf("disable should keep key: %+v", disabled)
	}
	if strings.Contains(w.Body.String(), "unit-key") || strings.Contains(w.Body.String(), `"api_key"`) {
		t.Fatal("PATCH leaked secret")
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/providers/acme/key", nil))
	if w.Code != 200 {
		t.Fatalf("delete key %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "unit-key") || strings.Contains(w.Body.String(), `"api_key"`) {
		t.Fatal("DELETE leaked secret")
	}
	var cleared struct {
		OK     bool `json:"ok"`
		KeySet bool `json:"key_set"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cleared); err != nil {
		t.Fatal(err)
	}
	if !cleared.OK || cleared.KeySet {
		t.Fatalf("cleared %+v %s", cleared, w.Body.String())
	}
	if _, err := secrets.Get(st, llm.APIKeySecretName("acme")); err == nil {
		t.Fatal("boxed key should be gone")
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/providers", nil))
	if strings.Contains(w.Body.String(), "unit-key") || strings.Contains(w.Body.String(), `"api_key"`) {
		t.Fatal("GET leaked secret after clear")
	}
	var listed struct {
		Providers []llm.ProviderInfo `json:"providers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	var acme, router9 *llm.ProviderInfo
	for i := range listed.Providers {
		switch listed.Providers[i].Name {
		case "acme":
			acme = &listed.Providers[i]
		case "router9":
			router9 = &listed.Providers[i]
		}
	}
	if acme == nil || acme.KeySet || acme.Source != "sqlite" || acme.Enabled {
		t.Fatalf("acme after clear %+v", acme)
	}
	if router9 == nil || router9.Source != "env" {
		t.Fatalf("env must win %+v", router9)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/providers/router9/key", nil))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "env overlay") {
		t.Fatalf("env delete %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/providers/missing/key", nil))
	if w.Code != 404 {
		t.Fatalf("missing delete %d %s", w.Code, w.Body.String())
	}
}

func TestProviders_TestErrorNeverLeaksKey(t *testing.T) {
	clearLLMEnv(t)
	master, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", master)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"nope","api_key":"unit-key","authorization":"Bearer unit-key"}`))
	}))
	defer srv.Close()
	st := store.New()
	h := providerRouter(t, st)
	create := `{"name":"leak","type":"openai-compat","base_url":"` + srv.URL + `","model":"m","api_key":"unit-key"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/providers", bytes.NewBufferString(create))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/providers/leak/test", bytes.NewBufferString(`{"kind":"models"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("test %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "unit-key") || strings.Contains(w.Body.String(), `"api_key":"unit-key"`) {
		t.Fatalf("test leaked secret: %s", w.Body.String())
	}
	var tr llm.TestResult
	if err := json.Unmarshal(w.Body.Bytes(), &tr); err != nil {
		t.Fatal(err)
	}
	if tr.OK || tr.Error == "" || tr.LatencyMS < 0 {
		t.Fatalf("want redacted failure %+v", tr)
	}
}
