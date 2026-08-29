// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/apikey"
	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func apiKeysServer(t *testing.T) (*apikey.Registry, *auditlog.Store, http.Handler) {
	t.Helper()
	reg := apikey.New()
	al := auditlog.New(64)
	h := NewRouter(Options{Store: store.New(), Version: "t", APIKeys: reg, Audit: al})
	return reg, al, h
}

func apiKeyJSON(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
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
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func assertNoAPIKeySecrets(t *testing.T, body, secret string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, n := range []string{`"secret"`, `"token"`, `"api_key"`, `"hash"`, `"key_hash"`, `"authorization"`} {
		if strings.Contains(lower, n) {
			t.Fatalf("secret field in body: %s", body)
		}
	}
	if secret != "" && strings.Contains(body, secret) {
		t.Fatalf("plaintext secret in body: %s", body)
	}
	if strings.Contains(body, "sk-") || strings.Contains(body, "gsk_") {
		t.Fatalf("token shape in body: %s", body)
	}
}

func TestAPIKeys_CreateOnceGETOmitsSecret(t *testing.T) {
	_, al, h := apiKeysServer(t)
	w := apiKeyJSON(t, h, "POST", "/api/api-keys", `{"name":"ops","scopes":["read","write"]}`, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Prefix string   `json:"prefix"`
		Secret string   `json:"secret"`
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Secret == "" || !strings.HasPrefix(created.Secret, created.Prefix) || created.ID == "" {
		t.Fatalf("created %#v", created)
	}

	w = apiKeyJSON(t, h, "GET", "/api/api-keys", "", "")
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	assertNoAPIKeySecrets(t, w.Body.String(), created.Secret)
	if !strings.Contains(w.Body.String(), `"prefix":"`+created.Prefix) {
		t.Fatalf("list missing prefix %s", w.Body.String())
	}

	w = apiKeyJSON(t, h, "GET", "/api/api-keys/"+created.ID, "", "")
	if w.Code != 200 {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}
	assertNoAPIKeySecrets(t, w.Body.String(), created.Secret)

	assertSameGET(t, h, "/api/api-keys", "/v1/api-keys")
	assertSameGET(t, h, "/api/api-keys/"+created.ID, "/v1/api-keys/"+created.ID)

	w = apiKeyJSON(t, h, "POST", "/api/api-keys/"+created.ID+"/revoke", `{"confirm":"ops"}`, "")
	if w.Code != 200 {
		t.Fatalf("revoke %d %s", w.Code, w.Body.String())
	}
	assertNoAPIKeySecrets(t, w.Body.String(), created.Secret)
	if !strings.Contains(w.Body.String(), `"status":"revoked"`) {
		t.Fatalf("revoked %s", w.Body.String())
	}

	page := al.Query(auditlog.Query{Entity: "api_key"})
	if page.Total < 2 {
		t.Fatalf("audit %d %#v", page.Total, page.Records)
	}
	for _, rec := range page.Records {
		raw, _ := json.Marshal(rec)
		if strings.Contains(string(raw), created.Secret) || rec.After["secret"] != nil || rec.After["token"] != nil {
			t.Fatalf("audit secret %#v", rec)
		}
	}
}

func TestAPIKeys_SearchExpiryAndRejects(t *testing.T) {
	_, _, h := apiKeysServer(t)
	exp := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	w := apiKeyJSON(t, h, "POST", "/api/api-keys", `{"name":"ci","tenant_id":"acme","scopes":["admin"],"expires_at":"`+exp+`"}`, "")
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	w = apiKeyJSON(t, h, "GET", "/api/api-keys?q=acme", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"ci"`) {
		t.Fatalf("search %d %s", w.Code, w.Body.String())
	}
	w = apiKeyJSON(t, h, "POST", "/api/api-keys", `{"name":"sk-live-abcdefgh","scopes":["read"]}`, "")
	if w.Code != 400 {
		t.Fatalf("secret name %d %s", w.Code, w.Body.String())
	}
	w = apiKeyJSON(t, h, "POST", "/api/api-keys", `{"name":"gk_abababababababababababab","scopes":["read"]}`, "")
	if w.Code != 400 {
		t.Fatalf("own-format secret name %d %s", w.Code, w.Body.String())
	}
	w = apiKeyJSON(t, h, "POST", "/api/api-keys", `{"name":"x","scopes":["root"]}`, "")
	if w.Code != 400 {
		t.Fatalf("unknown scope %d %s", w.Code, w.Body.String())
	}
	w = apiKeyJSON(t, h, "POST", "/api/api-keys/ak_1/revoke", `{}`, "")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirm is required") {
		t.Fatalf("no confirm %d %s", w.Code, w.Body.String())
	}
}

func TestAPIKeys_ViewTokenGETOnly(t *testing.T) {
	reg := apikey.New()
	inner := NewRouter(Options{Store: store.New(), Version: "t", APIKeys: reg})
	h := auth.RequireTokens("admin-113", "view-113", []string{"/healthz"})(inner)

	w := apiKeyJSON(t, h, "GET", "/api/api-keys", "", "view-113")
	if w.Code != 200 {
		t.Fatalf("view GET list %d %s", w.Code, w.Body.String())
	}
	w = apiKeyJSON(t, h, "GET", "/v1/api-keys", "", "view-113")
	if w.Code != 200 {
		t.Fatalf("view GET v1 %d %s", w.Code, w.Body.String())
	}
	w = apiKeyJSON(t, h, "POST", "/api/api-keys", `{"name":"x","scopes":["read"]}`, "view-113")
	if w.Code != http.StatusForbidden {
		t.Fatalf("view POST %d %s", w.Code, w.Body.String())
	}
}

func TestAPIKeys_IssuedKeyAuthAndUsage(t *testing.T) {
	reg := apikey.New()
	inner := NewRouter(Options{Store: store.New(), Version: "t", APIKeys: reg})
	h := auth.Require(auth.Config{Admin: "admin-113", Keys: reg})(inner)

	w := apiKeyJSON(t, h, "POST", "/api/api-keys", `{"name":"reader","scopes":["read"]}`, "admin-113")
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	w = apiKeyJSON(t, h, "GET", "/api/api-keys", "", created.Secret)
	if w.Code != 200 {
		t.Fatalf("read GET %d %s", w.Code, w.Body.String())
	}
	assertNoAPIKeySecrets(t, w.Body.String(), created.Secret)
	w = apiKeyJSON(t, h, "POST", "/api/agents", `{"agent_key":"a","display_name":"A"}`, created.Secret)
	if w.Code != http.StatusForbidden {
		t.Fatalf("read POST %d %s", w.Code, w.Body.String())
	}

	w = apiKeyJSON(t, h, "POST", "/api/api-keys", `{"name":"writer","scopes":["write"]}`, "admin-113")
	var writer struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &writer)
	w = apiKeyJSON(t, h, "POST", "/api/agents", `{"agent_key":"b","display_name":"B"}`, writer.Secret)
	if w.Code != 201 {
		t.Fatalf("write POST %d %s", w.Code, w.Body.String())
	}
	w = apiKeyJSON(t, h, "POST", "/api/api-keys", `{"name":"nope","scopes":["read"]}`, writer.Secret)
	if w.Code != http.StatusForbidden {
		t.Fatalf("write create keys %d %s", w.Code, w.Body.String())
	}

	got, err := reg.Get(created.ID)
	if err != nil || got.UseCount < 1 {
		t.Fatalf("usage %v %#v", err, got)
	}
}
