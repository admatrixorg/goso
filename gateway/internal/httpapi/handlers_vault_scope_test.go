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
	"github.com/mqglobal/goso/gateway/internal/vault"
)

type vaultKeyGate struct {
	token  string
	scopes []string
}

func (g vaultKeyGate) Accept(token string) (auth.Grant, bool) {
	if token != g.token {
		return auth.Grant{}, false
	}
	return auth.Grant{ID: "ak_vault", Prefix: "gk_vault", Scopes: g.scopes}, true
}

func TestVaultAPI_ForgedScopeAndSameTenantPermission(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_DIR", root)
	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "1")
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-140")
	st := store.New()
	inner := NewRouter(Options{Store: st, Version: "t"})
	h := auth.Require(auth.Config{Admin: "admin-140", View: "view-140", Keys: vaultKeyGate{token: "gk_write", scopes: []string{"write"}}})(inner)
	readH := auth.Require(auth.Config{Admin: "admin-140", Keys: vaultKeyGate{token: "gk_read", scopes: []string{"read"}}})(inner)

	put := func(handler http.Handler, token, tenant, raw string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/api/vault/docs", bytes.NewBufferString(raw))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if tenant != "" {
			req.Header.Set("X-Goso-Tenant", tenant)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}

	sales := put(h, "admin-140", "minh-an-sales", `{"title":"Chính sách chiết khấu Minh An","body":"sales-9-percent","tenant_id":"minh-an-legal","path":"agents/cfo/note.md","agent_id":"cfo","project_id":"mien-tay","customer_key":"dai-ly-mau"}`)
	if sales.Code != http.StatusForbidden {
		t.Fatalf("forged tenant was accepted %d %s", sales.Code, sales.Body.String())
	}
	sales = put(h, "admin-140", "minh-an-sales", `{"title":"Chính sách chiết khấu Minh An","body":"sales-9-percent","path":"../escape.md"}`)
	if sales.Code != http.StatusBadRequest {
		t.Fatalf("traversal %d %s", sales.Code, sales.Body.String())
	}
	sales = put(h, "admin-140", "minh-an-sales", `{"title":"Chính sách chiết khấu Minh An","body":"sales-9-percent","path":"agents/cfo/note.md","customer_key":"dai-ly-mau"}`)
	if sales.Code != http.StatusCreated {
		t.Fatalf("put sales %d %s", sales.Code, sales.Body.String())
	}
	var salesDoc store.VaultDoc
	if err := json.Unmarshal(sales.Body.Bytes(), &salesDoc); err != nil {
		t.Fatal(err)
	}
	if salesDoc.TenantID != "minh-an-sales" || strings.Contains(salesDoc.Path, "..") || strings.HasPrefix(salesDoc.Path, "agents/") {
		t.Fatalf("forged scope applied %#v", salesDoc)
	}
	legal := put(h, "admin-140", "minh-an-legal", `{"title":"Chính sách chiết khấu Minh An","body":"legal-8-percent"}`)
	if legal.Code != http.StatusCreated {
		t.Fatalf("put legal %d %s", legal.Code, legal.Body.String())
	}
	var legalDoc store.VaultDoc
	if err := json.Unmarshal(legal.Body.Bytes(), &legalDoc); err != nil {
		t.Fatal(err)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/vault/docs/"+legalDoc.ID, nil)
	get.Header.Set("Authorization", "Bearer admin-140")
	get.Header.Set("X-Goso-Tenant", "minh-an-sales")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "legal-8-percent") {
		t.Fatalf("cross read %d %s", w.Code, w.Body.String())
	}

	view := put(h, "view-140", "minh-an-sales", `{"title":"Nope","body":"x"}`)
	if view.Code != http.StatusForbidden {
		t.Fatalf("view token wrote %d %s", view.Code, view.Body.String())
	}
	reader := put(readH, "gk_read", "minh-an-legal", `{"title":"Nope","body":"x"}`)
	if reader.Code != http.StatusForbidden {
		t.Fatalf("read key wrote %d %s", reader.Code, reader.Body.String())
	}
	writer := put(h, "gk_write", "minh-an-legal", `{"title":"Header is not staff","body":"default-only"}`)
	if writer.Code != http.StatusCreated {
		t.Fatalf("write key %d %s", writer.Code, writer.Body.String())
	}
	var written store.VaultDoc
	if err := json.Unmarshal(writer.Body.Bytes(), &written); err != nil {
		t.Fatal(err)
	}
	if written.TenantID != store.DefaultTenant {
		t.Fatalf("write key header selected a tenant %#v", written)
	}
	if got, err := st.GetVaultDoc(legalDoc.ID); err != nil || got.Body != "legal-8-percent" {
		t.Fatalf("write key touched legal: %v %#v", err, got)
	}

	syncReq := httptest.NewRequest(http.MethodPost, "/api/vault/sync?tenant=minh-an-legal", nil)
	syncReq.Header.Set("Authorization", "Bearer admin-140")
	syncReq.Header.Set("X-Goso-Tenant", "minh-an-sales")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, syncReq)
	if w.Code != http.StatusOK {
		t.Fatalf("sync %d %s", w.Code, w.Body.String())
	}
	if got, err := st.GetVaultDoc(legalDoc.ID); err != nil || !strings.Contains(got.Body, "legal-8-percent") {
		t.Fatalf("sync query tenant changed legal: %v %#v", err, got)
	}

	inv := httptest.NewRequest(http.MethodGet, "/api/vault/inventory?apply=1", nil)
	inv.Header.Set("Authorization", "Bearer admin-140")
	inv.Header.Set("X-Goso-Tenant", "minh-an-sales")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, inv)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"dry_run":true`) || !strings.Contains(w.Body.String(), `"applied":false`) {
		t.Fatalf("inventory %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "legal-8-percent") || strings.Contains(w.Body.String(), legalDoc.ID) {
		t.Fatalf("inventory leaked legal %s", w.Body.String())
	}
	legacy := filepath.Join(root, "legacy-unclear.md")
	if err := os.WriteFile(legacy, []byte("keep-legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, inv)
	after, err := os.ReadFile(legacy)
	if err != nil || string(after) != "keep-legacy" {
		t.Fatalf("inventory mutated legacy: %v %q", err, after)
	}

	t.Setenv("GOSO_VAULT_SCOPED_SYNC", "")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, syncReq)
	if w.Code != http.StatusForbidden {
		t.Fatalf("flag off sync %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "chinh-sach.md")); !os.IsNotExist(err) {
		t.Fatalf("sync moved a file to the shared root: %v", err)
	}
	_ = vault.ErrSyncDisabled
}
