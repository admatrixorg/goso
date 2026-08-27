// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestVaultAPI_PutLinksSearchSync(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_DIR", root)
	st, h := newTestServer()

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/vault/docs", bytes.NewBufferString(`{"title":"Alpha","body":"see [[Beta]] pineapple"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("PUT alpha %d %s", w.Code, w.Body.String())
	}
	var alpha store.VaultDoc
	if err := json.Unmarshal(w.Body.Bytes(), &alpha); err != nil || alpha.ID == "" {
		t.Fatalf("alpha %v %s", err, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PUT", "/api/vault/docs", bytes.NewBufferString(`{"title":"Beta","body":"see [[Alpha]] mango"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("PUT beta %d %s", w.Code, w.Body.String())
	}
	var beta store.VaultDoc
	_ = json.Unmarshal(w.Body.Bytes(), &beta)

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/vault/docs/"+alpha.ID+"/links", nil))
	if w.Code != 200 {
		t.Fatalf("links %d %s", w.Code, w.Body.String())
	}
	var edges struct {
		Outbound []store.VaultLink `json:"outbound"`
		Inbound  []store.VaultLink `json:"inbound"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &edges)
	if len(edges.Outbound) != 1 || edges.Outbound[0].ToID != beta.ID || len(edges.Inbound) != 1 {
		t.Fatalf("edges %#v", edges)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/vault/search?q=pineapple", nil))
	if w.Code != 200 {
		t.Fatalf("search %d %s", w.Code, w.Body.String())
	}
	var hits []store.VaultSearchHit
	if err := json.Unmarshal(w.Body.Bytes(), &hits); err != nil || len(hits) == 0 {
		t.Fatalf("hits %v %s", err, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/vault/search?q=no-such-token-xyz", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("empty search %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/vault/search", nil))
	if w.Code != 400 {
		t.Fatalf("empty q %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/vault/docs", nil))
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/vault/docs/"+alpha.ID, nil))
	if w.Code != 200 {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}

	if err := os.WriteFile(filepath.Join(root, "gamma.md"), []byte("# Gamma\nsee [[Alpha]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/vault/sync", nil))
	if w.Code != 200 {
		t.Fatalf("sync %d %s", w.Code, w.Body.String())
	}
	got, err := st.FindVaultDocByTitle("Gamma")
	if err != nil || got == nil {
		t.Fatalf("gamma after sync %v %#v", err, got)
	}
	_, ib, err := st.ListVaultDocLinks(alpha.ID)
	if err != nil || len(ib) < 2 {
		t.Fatalf("alpha inbound after gamma %v %#v", err, ib)
	}
}

func TestVaultAPI_BearerAuth(t *testing.T) {
	t.Setenv("GOSO_VAULT_DIR", t.TempDir())
	st := store.New()
	mux := Router(st, "0.1.0")
	h := auth.RequireToken("secret", []string{"/healthz"})(mux)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/vault/docs", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth list %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/vault/search?q=hi", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth search %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/vault/sync", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth sync %d", w.Code)
	}

	req := httptest.NewRequest("GET", "/api/vault/search?q=hi", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("auth search %d %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("auth empty %q", w.Body.String())
	}
}

func TestVaultAPI_NotFound(t *testing.T) {
	t.Setenv("GOSO_VAULT_DIR", t.TempDir())
	_, h := newTestServer()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/vault/docs/missing", nil))
	if w.Code != 404 {
		t.Fatalf("get missing %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/vault/docs/missing/links", nil))
	if w.Code != 404 {
		t.Fatalf("links missing %d", w.Code)
	}
	w = httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/vault/docs", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("empty title %d", w.Code)
	}
}
