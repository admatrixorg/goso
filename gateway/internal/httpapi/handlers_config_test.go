// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/config"
)

func TestConfigGETOmitsAuthToken(t *testing.T) {
	t.Cleanup(config.ResetOverlay)
	config.ResetOverlay()
	secret := "live-admin-token-should-never-return"
	t.Setenv("GOSO_ADMIN_TOKEN", secret)
	t.Setenv("GOSO_VIEW_TOKEN", "")
	t.Setenv("GOSO_MASTER_KEY", "")
	t.Setenv("GOSO_DATABASE_URL", "")
	_, h := newTestServer()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if w.Code != 200 {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, secret) {
		t.Fatalf("GET leaked token: %s", body)
	}
	if config.ContainsSecret(w.Body.Bytes()) {
		t.Fatalf("ContainsSecret: %s", body)
	}
	var snap config.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Auth.TokenSet.Value != true {
		t.Fatalf("token_set %+v", snap.Auth.TokenSet)
	}
	if snap.Auth.TokenSet.Editable {
		t.Fatal("token_set must not be editable")
	}
}

func TestConfigPUTValidationConflictEnvOwned(t *testing.T) {
	t.Cleanup(config.ResetOverlay)
	config.ResetOverlay()
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	t.Setenv("GOSO_QUOTA_DAY", "")
	t.Setenv("GOSO_INJECTION", "")
	t.Setenv("GOSO_LOG_LEVEL", "")
	st, h := newTestServer()

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if w.Code != 200 {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewBufferString(`{"values":{"quota_day":"-3"}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("invalid quota want 400, got %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewBufferString(`{"values":{"token":"nope"}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("secret field want 400, got %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewBufferString(`{"values":{"quota_day":"11","log_level":"debug"}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("first put %d %s", w.Code, w.Body.String())
	}
	var first config.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.UpdatedAt == "" {
		t.Fatal("updated_at missing")
	}
	if first.Quota.DayLimit.Value != "11" {
		t.Fatalf("quota %+v", first.Quota.DayLimit)
	}
	if first.Server.LogLevel.Value != "debug" {
		t.Fatalf("log_level %+v", first.Server.LogLevel)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewBufferString(`{"values":{"quota_day":"12"}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Fatalf("stale put want 409, got %d %s", w.Code, w.Body.String())
	}

	t.Setenv("GOSO_INJECTION", "block")
	w = httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"updated_at": first.UpdatedAt, "values": map[string]string{"injection": "log"}})
	req = httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "env-owned") {
		t.Fatalf("env-owned want 409, got %d %s", w.Code, w.Body.String())
	}

	got, err := st.GetGatewaySettings()
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["quota_day"] != "11" {
		t.Fatalf("store %+v", got.Values)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/config", nil))
	if w.Code != 200 {
		t.Fatalf("v1 GET %d %s", w.Code, w.Body.String())
	}
}

func TestConfigGETGroups(t *testing.T) {
	t.Cleanup(config.ResetOverlay)
	config.ResetOverlay()
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	_, h := newTestServer()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var snap map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"server", "auth", "behavior", "quota", "tools", "integrations"} {
		if _, ok := snap[k]; !ok {
			t.Fatalf("missing group %s in %s", k, w.Body.String())
		}
	}
}
