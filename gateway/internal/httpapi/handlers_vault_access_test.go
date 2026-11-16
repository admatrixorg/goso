// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

func TestVaultAPI_AgentScope(t *testing.T) {
	t.Setenv("GOSO_VAULT_DIR", t.TempDir())
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-kn")
	t.Setenv("GOSO_VAULT_UPLOAD", "1")
	st := store.New()
	x, _ := st.CreateAgent(store.Agent{AgentKey: "minh-an-x", TenantID: "minh-an"})
	y, _ := st.CreateAgent(store.Agent{AgentKey: "minh-an-y", TenantID: "minh-an"})
	foreign, _ := st.CreateAgent(store.Agent{AgentKey: "khac", TenantID: "tenant-khac"})
	foreignTeam, err := st.CreateTeam(store.Team{Name: "Khac", LeadAgentID: foreign.ID, TenantID: "tenant-khac"})
	if err != nil {
		t.Fatal(err)
	}
	h := auth.Require(auth.Config{Admin: "admin-kn"})(NewRouter(Options{Store: st, Version: "t"}))
	do := func(method, path, raw string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goso-Tenant", "minh-an")
		req.Header.Set("Authorization", "Bearer admin-kn")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	for _, raw := range []string{
		`{"title":"Sai","body":"x","agent_id":"` + foreign.ID + `"}`,
		`{"title":"Sai","body":"x","scope":"team","team_id":"` + foreignTeam.ID + `"}`,
		`{"title":"Sai","body":"x","scope":"agent"}`,
		`{"title":"Sai","body":"x","scope":"galaxy"}`,
	} {
		if w := do(http.MethodPut, "/api/vault/docs", raw); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", raw, w.Code, w.Body.String())
		}
	}
	w := do(http.MethodPut, "/api/vault/docs", `{"title":"Kich ban rieng X","body":"kich ban chot don","agent_id":"minh-an-x"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("put %d %s", w.Code, w.Body.String())
	}
	var doc store.VaultDoc
	_ = json.Unmarshal(w.Body.Bytes(), &doc)
	if doc.Scope != store.VaultScopeAgent || doc.AgentID != x.ID {
		t.Fatalf("stored %#v", doc)
	}
	sees := func(agent string) bool {
		t.Helper()
		q := "/api/vault/search?q=" + url.QueryEscape("kich ban")
		if agent != "" {
			q += "&agent_id=" + agent
		}
		w := do(http.MethodGet, q, "")
		if w.Code != http.StatusOK {
			t.Fatalf("search %d %s", w.Code, w.Body.String())
		}
		inSearch := strings.Contains(w.Body.String(), doc.ID)
		get := do(http.MethodGet, "/api/vault/docs/"+doc.ID+"?agent_id="+agent, "")
		list := do(http.MethodGet, "/api/vault/docs?agent_id="+agent, "")
		if agent == "" {
			get = do(http.MethodGet, "/api/vault/docs/"+doc.ID, "")
			list = do(http.MethodGet, "/api/vault/docs", "")
		}
		if inSearch != (get.Code == http.StatusOK) || inSearch != strings.Contains(list.Body.String(), doc.ID) {
			t.Fatalf("agent %q: search=%v get=%d list=%s", agent, inSearch, get.Code, list.Body.String())
		}
		return inSearch
	}
	if !sees("") || !sees(x.ID) || sees(y.ID) || sees("minh-an-y") {
		t.Fatal("agent view wrong")
	}
	if w := do(http.MethodGet, "/api/vault/search?q=x&agent_id="+foreign.ID, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("foreign agent view %d", w.Code)
	}
	// Upload validates the owner before reading the body.
	if w := do(http.MethodPost, "/api/vault/upload?agent_id="+foreign.ID, "x"); w.Code != http.StatusBadRequest {
		t.Fatalf("upload foreign agent %d %s", w.Code, w.Body.String())
	}
}

func TestVaultAPI_EmbeddingStatus(t *testing.T) {
	t.Setenv("GOSO_DEV_MODE", "1")
	vault.SetIndexer(nil)
	h := NewRouter(Options{Store: store.New(), Version: "t"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/vault/embeddings", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"state":"`+vault.EmbedNotConfigured+`"`) {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/vault/embeddings/backfill", nil))
	if w.Code != http.StatusConflict {
		t.Fatalf("backfill without indexer %d", w.Code)
	}
	for _, p := range []string{"/api/memory/status", "/api/kg/status"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code == http.StatusOK && strings.Contains(w.Body.String(), `"embedding`) && !strings.Contains(w.Body.String(), vault.EmbedNotConfigured) {
			t.Fatalf("%s reports %s", p, w.Body.String())
		}
	}
}
