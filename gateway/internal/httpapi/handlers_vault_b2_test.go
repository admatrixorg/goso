// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestB2_InventoryDenyLeaksNoMetadata(t *testing.T) {
	const marker = "LEGACY-ROW-8841"
	st := store.New()
	if _, err := st.PutVaultDoc(store.VaultDoc{
		Title: marker, Path: "chinh-sach.md", TenantID: store.DefaultTenant,
		Body: marker, SHA256: "s", OwnerExplicit: true,
	}); err != nil {
		t.Fatal(err)
	}
	inner := NewRouter(Options{Store: st, Version: "t"})
	admin := auth.Require(auth.Config{
		Admin: "admin-b2",
		View:  "view-b2",
		Keys:  vaultKeyGate{token: "gk_write", scopes: []string{"write"}},
	})(inner)
	readH := auth.Require(auth.Config{
		Admin: "admin-b2",
		Keys:  vaultKeyGate{token: "gk_read", scopes: []string{"read"}},
	})(inner)

	call := func(h http.Handler, method, token string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/vault/inventory", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	assertDenied := func(name string, w *httptest.ResponseRecorder) {
		t.Helper()
		if w.Code == http.StatusOK || strings.Contains(w.Body.String(), marker) || strings.Contains(w.Body.String(), "chinh-sach") {
			t.Fatalf("%s leaked %d %s", name, w.Code, w.Body.String())
		}
	}
	assertDenied("unauth GET", call(admin, http.MethodGet, ""))
	assertDenied("unauth POST", call(admin, http.MethodPost, ""))
	assertDenied("read GET", call(readH, http.MethodGet, "gk_read"))
	assertDenied("read POST", call(readH, http.MethodPost, "gk_read"))
	assertDenied("write GET", call(admin, http.MethodGet, "gk_write"))
	assertDenied("write POST", call(admin, http.MethodPost, "gk_write"))
	assertDenied("view GET", call(admin, http.MethodGet, "view-b2"))
	assertDenied("view POST", call(admin, http.MethodPost, "view-b2"))
}

func TestB4_TwoHTTPRequestsSameTenant(t *testing.T) {
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	t.Setenv("GOSO_VAULT_DIR", t.TempDir())
	_, h := newTestServer()
	put := httptest.NewRequest(http.MethodPut, "/api/vault/docs", strings.NewReader(`{"title":"Giữ","body":"stay"}`))
	put.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, put)
	if w.Code != http.StatusCreated {
		t.Fatalf("seed %d %s", w.Code, w.Body.String())
	}
	// The vault package barrier is exercised by TestB4_TwoServicesSameTenantDoNotDropPut.
	// This request pair shares one vault directory and one store, which is the HTTP shape of that race.
	entered := make(chan struct{})
	release := make(chan struct{})
	// Imported in the vault test; HTTP coverage uses two concurrent requests below.
	_ = entered
	_ = release
	syncDone := make(chan int, 1)
	putDone := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/vault/sync", nil))
		syncDone <- rec.Code
	}()
	go func() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/vault/docs", strings.NewReader(`{"title":"Mới","body":"FRESH-HTTP-441"}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rec, req)
		putDone <- rec.Code
	}()
	if code := <-syncDone; code != http.StatusOK {
		t.Fatalf("sync %d", code)
	}
	if code := <-putDone; code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("put %d", code)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/vault/search?q=FRESH-HTTP-441", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "FRESH-HTTP-441") {
		t.Fatalf("http put lost %d %s", rec.Code, rec.Body.String())
	}
}
