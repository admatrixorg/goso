// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/apikey"
	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/impexp"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func impexpServer(t *testing.T) (store.StoreIface, *auditlog.Store, http.Handler) {
	t.Helper()
	st := store.New()
	al := auditlog.New(64)
	h := NewRouter(Options{Store: st, Version: "t", Audit: al})
	return st, al, h
}

func peJSON(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
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

func assertNoPortableSecrets(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, n := range []string{`"token":`, `"secret":`, `"api_key":`, `"password":`, `"authorization":`} {
		if strings.Contains(lower, n) {
			t.Fatalf("secret field in body: %s", body)
		}
	}
	if strings.Contains(body, "sk-") || strings.Contains(body, "gsk_") {
		t.Fatalf("token shape in body: %s", body)
	}
}

func TestImportExport_CatalogExportImportRollback(t *testing.T) {
	st, al, h := impexpServer(t)
	ag, err := st.CreateAgent(store.Agent{AgentKey: "bot", DisplayName: "Bot"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateTeam(store.Team{Name: "Ops", LeadAgentID: ag.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConnector(store.ConnectorRecord{Name: "crm", Transport: "http", Endpoint: "http://127.0.0.1:9", CredentialRef: "secret:connector/crm/token"}); err != nil {
		t.Fatal(err)
	}

	w := peJSON(t, h, "GET", "/api/import-export", "", "")
	if w.Code != 200 {
		t.Fatalf("catalog %d %s", w.Code, w.Body.String())
	}
	assertNoPortableSecrets(t, w.Body.String())

	w = peJSON(t, h, "GET", "/v1/import-export", "", "")
	if w.Code != 200 {
		t.Fatalf("v1 catalog %d %s", w.Code, w.Body.String())
	}

	sel := `{"team_ids":[],"agent_ids":["` + ag.ID + `"],"skill_names":[],"mcp_names":["crm"]}`
	w = peJSON(t, h, "POST", "/api/import-export/export", sel, "")
	if w.Code != 200 {
		t.Fatalf("export %d %s", w.Code, w.Body.String())
	}
	assertNoPortableSecrets(t, w.Body.String())
	var exp impexp.Job
	if err := json.Unmarshal(w.Body.Bytes(), &exp); err != nil || exp.Archive == nil {
		t.Fatalf("export job %v %s", err, w.Body.String())
	}

	arch, _ := json.Marshal(map[string]any{"archive": exp.Archive})
	w = peJSON(t, h, "POST", "/api/import-export/preview", string(arch), "")
	if w.Code != 200 {
		t.Fatalf("preview %d %s", w.Code, w.Body.String())
	}
	assertNoPortableSecrets(t, w.Body.String())

	dst := store.New()
	h2 := NewRouter(Options{Store: dst, Version: "t", Audit: auditlog.New(32)})
	impBody, _ := json.Marshal(map[string]any{"archive": exp.Archive, "conflict": "skip", "dry_run": true})
	w = peJSON(t, h2, "POST", "/api/import-export/import", string(impBody), "")
	if w.Code != 200 {
		t.Fatalf("dry %d %s", w.Code, w.Body.String())
	}
	if len(dst.ListAgents()) != 0 {
		t.Fatal("dry run wrote")
	}

	impBody, _ = json.Marshal(map[string]any{"archive": exp.Archive, "conflict": "skip", "dry_run": false})
	w = peJSON(t, h2, "POST", "/api/import-export/import", string(impBody), "")
	if w.Code != 200 {
		t.Fatalf("import %d %s", w.Code, w.Body.String())
	}
	assertNoPortableSecrets(t, w.Body.String())
	var imp impexp.Job
	if err := json.Unmarshal(w.Body.Bytes(), &imp); err != nil {
		t.Fatal(err)
	}
	if len(dst.ListAgents()) != 1 {
		t.Fatalf("imported agents %d", len(dst.ListAgents()))
	}

	w = peJSON(t, h2, "GET", "/api/import-export/"+imp.ID, "", "")
	if w.Code != 200 {
		t.Fatalf("job get %d %s", w.Code, w.Body.String())
	}
	assertNoPortableSecrets(t, w.Body.String())

	w = peJSON(t, h2, "POST", "/api/import-export/"+imp.ID+"/rollback", "{}", "")
	if w.Code != 200 {
		t.Fatalf("rollback %d %s", w.Code, w.Body.String())
	}
	if len(dst.ListAgents()) != 0 {
		t.Fatal("rollback leftover")
	}

	page := al.Query(auditlog.Query{Entity: "portable", Limit: 10})
	if page.Total == 0 {
		t.Fatal("audit export")
	}
}

func TestImportExport_RejectsBadSchemaAndSmuggledToken(t *testing.T) {
	_, _, h := impexpServer(t)
	w := peJSON(t, h, "POST", "/api/import-export/import", `{"archive":{"schema":"nope","schema_version":1},"dry_run":true}`, "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad schema %d %s", w.Code, w.Body.String())
	}
	smuggle := `{"archive":{"schema":"goso.portable/v1","schema_version":1,"mcp":[{"name":"crm","transport":"http","token":"sk-live-fixture-not-vendor-zzzz"}]},"dry_run":true}`
	w = peJSON(t, h, "POST", "/api/import-export/import", smuggle, "")
	if w.Code != 200 {
		t.Fatalf("smuggle import %d %s", w.Code, w.Body.String())
	}
	assertNoPortableSecrets(t, w.Body.String())
}

func TestImportExport_V1Alias(t *testing.T) {
	_, _, h := impexpServer(t)
	wAPI := peJSON(t, h, "GET", "/api/import-export", "", "")
	wV1 := peJSON(t, h, "GET", "/v1/import-export", "", "")
	if wAPI.Code != 200 || wV1.Code != 200 {
		t.Fatalf("alias api=%d v1=%d", wAPI.Code, wV1.Code)
	}
	var a, b map[string]any
	if err := json.Unmarshal(wAPI.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wV1.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	delete(a, "generated_at")
	delete(b, "generated_at")
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	if string(ab) != string(bb) {
		t.Fatalf("alias body api=%s v1=%s", ab, bb)
	}
}

func TestImportExport_ViewTokenGETOnly(t *testing.T) {
	_, _, inner := impexpServer(t)
	h := auth.RequireTokens("admin-116", "view-116", []string{"/healthz"})(inner)
	w := peJSON(t, h, "GET", "/api/import-export", "", "view-116")
	if w.Code != 200 {
		t.Fatalf("view GET %d %s", w.Code, w.Body.String())
	}
	w = peJSON(t, h, "GET", "/v1/import-export", "", "view-116")
	if w.Code != 200 {
		t.Fatalf("view v1 %d %s", w.Code, w.Body.String())
	}
	w = peJSON(t, h, "POST", "/api/import-export/export", `{"agent_ids":["x"]}`, "view-116")
	if w.Code != http.StatusForbidden {
		t.Fatalf("view POST %d %s", w.Code, w.Body.String())
	}
}

func TestImportExport_IssuedScopes(t *testing.T) {
	keys := apikey.New()
	st := store.New()
	inner := NewRouter(Options{Store: st, Version: "t", APIKeys: keys})
	h := auth.Require(auth.Config{Admin: "admin-116", Keys: keys})(inner)

	w := peJSON(t, h, "POST", "/api/api-keys", `{"name":"reader","scopes":["read"]}`, "admin-116")
	if w.Code != 201 {
		t.Fatalf("create read %d %s", w.Code, w.Body.String())
	}
	var reader struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &reader)
	w = peJSON(t, h, "GET", "/api/import-export", "", reader.Secret)
	if w.Code != 200 {
		t.Fatalf("read GET %d %s", w.Code, w.Body.String())
	}
	w = peJSON(t, h, "POST", "/api/import-export/export", `{"agent_ids":["x"]}`, reader.Secret)
	if w.Code != http.StatusForbidden {
		t.Fatalf("read POST %d %s", w.Code, w.Body.String())
	}

	w = peJSON(t, h, "POST", "/api/api-keys", `{"name":"writer","scopes":["write"]}`, "admin-116")
	var writer struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &writer)
	w = peJSON(t, h, "POST", "/api/import-export/preview", `{"archive":{"schema":"goso.portable/v1","schema_version":1}}`, writer.Secret)
	if w.Code != 200 {
		t.Fatalf("write preview %d %s", w.Code, w.Body.String())
	}
}
