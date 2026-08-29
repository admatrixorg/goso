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
	"github.com/mqglobal/goso/gateway/internal/pkgmgr"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func testPackages() *pkgmgr.Manager {
	return pkgmgr.New(func() []pkgmgr.Runtime {
		return []pkgmgr.Runtime{
			{Name: "python", Ecosystem: "python", Present: true, Version: "3.12.1", Compatible: true},
			{Name: "node", Ecosystem: "node", Present: true, Version: "22.1.0", Compatible: true},
			{Name: "git", Ecosystem: "github", Present: true, Version: "2.45.0", Compatible: true},
			{Name: "go", Present: true, Version: "1.22.5", Compatible: true},
		}
	})
}

func packagesServer(t *testing.T) (*pkgmgr.Manager, *auditlog.Store, http.Handler) {
	t.Helper()
	m := testPackages()
	al := auditlog.New(64)
	h := NewRouter(Options{Store: store.New(), Version: "t", Packages: m, Audit: al})
	return m, al, h
}

func pkgJSON(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
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

func assertNoPackageSecrets(t *testing.T, body, token string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, n := range []string{`"secret"`, `"token"`, `"api_key"`, `"hash"`, `"key_hash"`, `"authorization"`, `"password"`} {
		if strings.Contains(lower, n) {
			t.Fatalf("secret field in body: %s", body)
		}
	}
	if token != "" && strings.Contains(body, token) {
		t.Fatalf("plaintext token in body: %s", body)
	}
	if strings.Contains(body, "sk-") || strings.Contains(body, "gsk_") || strings.Contains(body, "ghp_") {
		t.Fatalf("token shape in body: %s", body)
	}
}

func TestPackages_InstallConfirmAllowlistGETOmitsCLI(t *testing.T) {
	_, al, h := packagesServer(t)
	w := pkgJSON(t, h, "GET", "/api/packages", "", "")
	if w.Code != 200 {
		t.Fatalf("snap %d %s", w.Code, w.Body.String())
	}
	assertNoPackageSecrets(t, w.Body.String(), "")
	if !strings.Contains(w.Body.String(), `"packages":[]`) && !strings.Contains(w.Body.String(), `"packages": []`) {
		if !strings.Contains(w.Body.String(), `"packages":`) {
			t.Fatalf("missing packages %s", w.Body.String())
		}
	}

	w = pkgJSON(t, h, "POST", "/api/packages/allow", `{"ecosystem":"python","name":"httpx","pin":"0.27.2"}`, "")
	if w.Code != 201 {
		t.Fatalf("allow %d %s", w.Code, w.Body.String())
	}
	w = pkgJSON(t, h, "POST", "/api/packages/install", `{"ecosystem":"python","name":"httpx","version":"0.27.2","confirm":"httpx"}`, "")
	if w.Code != 201 {
		t.Fatalf("install %d %s", w.Code, w.Body.String())
	}
	assertNoPackageSecrets(t, w.Body.String(), "")

	tok := "ghp_fixture_not_live_aaaa"
	w = pkgJSON(t, h, "POST", "/api/packages/cli", `{"kind":"github","token":"`+tok+`"}`, "")
	if w.Code != 200 {
		t.Fatalf("cli %d %s", w.Code, w.Body.String())
	}
	assertNoPackageSecrets(t, w.Body.String(), tok)
	if !strings.Contains(w.Body.String(), `"set":true`) {
		t.Fatalf("set meta %s", w.Body.String())
	}

	w = pkgJSON(t, h, "GET", "/api/packages", "", "")
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	assertNoPackageSecrets(t, w.Body.String(), tok)
	if !strings.Contains(w.Body.String(), `"status":"installed"`) {
		t.Fatalf("installed %s", w.Body.String())
	}

	w = pkgJSON(t, h, "POST", "/api/packages/install", `{"ecosystem":"python","name":"httpx","version":"0.27.2","confirm":"httpx","token":"sneaky"}`, "")
	if w.Code != 400 {
		t.Fatalf("secret field %d %s", w.Code, w.Body.String())
	}

	assertSameGET(t, h, "/api/packages", "/v1/packages")

	page := al.Query(auditlog.Query{Entity: "package"})
	if page.Total < 1 {
		t.Fatalf("audit package %d", page.Total)
	}
	for _, rec := range page.Records {
		raw, _ := json.Marshal(rec)
		if strings.Contains(string(raw), tok) || rec.After["token"] != nil || rec.After["secret"] != nil {
			t.Fatalf("audit secret %#v", rec)
		}
	}
	cliPage := al.Query(auditlog.Query{Entity: "package_cli"})
	if cliPage.Total < 1 {
		t.Fatalf("audit cli %d", cliPage.Total)
	}
	for _, rec := range cliPage.Records {
		raw, _ := json.Marshal(rec)
		if strings.Contains(string(raw), tok) {
			t.Fatalf("cli audit token %#v", rec)
		}
	}
}

func TestPackages_PartialRecoverUninstall(t *testing.T) {
	m, _, h := packagesServer(t)
	w := pkgJSON(t, h, "POST", "/api/packages/allow", `{"ecosystem":"node","name":"left-pad","pin":"1.3.0"}`, "")
	if w.Code != 201 {
		t.Fatalf("allow %d %s", w.Code, w.Body.String())
	}
	m.FailAt = 2
	w = pkgJSON(t, h, "POST", "/api/packages/install", `{"ecosystem":"node","name":"left-pad","version":"1.3.0","confirm":"left-pad"}`, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"partial"`) {
		t.Fatalf("partial %d %s", w.Code, w.Body.String())
	}
	var wrap struct {
		Package struct {
			ID string `json:"id"`
		} `json:"package"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &wrap); err != nil || wrap.Package.ID == "" {
		t.Fatalf("id %v %s", err, w.Body.String())
	}
	w = pkgJSON(t, h, "POST", "/api/packages/"+wrap.Package.ID+"/recover", `{}`, "")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirm is required") {
		t.Fatalf("recover confirm %d %s", w.Code, w.Body.String())
	}
	w = pkgJSON(t, h, "POST", "/api/packages/"+wrap.Package.ID+"/recover", `{"confirm":"`+wrap.Package.ID+`"}`, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"installed"`) {
		t.Fatalf("recover %d %s", w.Code, w.Body.String())
	}
	w = pkgJSON(t, h, "POST", "/api/packages/"+wrap.Package.ID+"/uninstall", `{"confirm":"left-pad"}`, "")
	if w.Code != 200 {
		t.Fatalf("uninstall %d %s", w.Code, w.Body.String())
	}
	w = pkgJSON(t, h, "GET", "/api/packages/"+wrap.Package.ID, "", "")
	if w.Code != 404 {
		t.Fatalf("gone %d %s", w.Code, w.Body.String())
	}
}

func TestPackages_ViewTokenGETOnly(t *testing.T) {
	m := testPackages()
	inner := NewRouter(Options{Store: store.New(), Version: "t", Packages: m})
	h := auth.RequireTokens("admin-114", "view-114", []string{"/healthz"})(inner)

	w := pkgJSON(t, h, "GET", "/api/packages", "", "view-114")
	if w.Code != 200 {
		t.Fatalf("view GET %d %s", w.Code, w.Body.String())
	}
	w = pkgJSON(t, h, "GET", "/v1/packages", "", "view-114")
	if w.Code != 200 {
		t.Fatalf("view v1 %d %s", w.Code, w.Body.String())
	}
	w = pkgJSON(t, h, "POST", "/api/packages/install", `{"ecosystem":"python","name":"httpx","version":"0.27.2","confirm":"httpx"}`, "view-114")
	if w.Code != http.StatusForbidden {
		t.Fatalf("view install %d %s", w.Code, w.Body.String())
	}
	w = pkgJSON(t, h, "POST", "/api/packages/cli", `{"kind":"github","token":"aaaa"}`, "view-114")
	if w.Code != http.StatusForbidden {
		t.Fatalf("view cli %d %s", w.Code, w.Body.String())
	}
}

func TestPackages_WriteScopeCannotMutate(t *testing.T) {
	keys := apikey.New()
	m := testPackages()
	inner := NewRouter(Options{Store: store.New(), Version: "t", Packages: m, APIKeys: keys})
	h := auth.Require(auth.Config{Admin: "admin-114", Keys: keys})(inner)

	w := pkgJSON(t, h, "POST", "/api/api-keys", `{"name":"reader","scopes":["read"]}`, "admin-114")
	if w.Code != 201 {
		t.Fatalf("create read %d %s", w.Code, w.Body.String())
	}
	var reader struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &reader)
	w = pkgJSON(t, h, "GET", "/api/packages", "", reader.Secret)
	if w.Code != 200 {
		t.Fatalf("read GET %d %s", w.Code, w.Body.String())
	}
	w = pkgJSON(t, h, "POST", "/api/packages/allow", `{"ecosystem":"python","name":"httpx","pin":"0.27.2"}`, reader.Secret)
	if w.Code != http.StatusForbidden {
		t.Fatalf("read allow %d %s", w.Code, w.Body.String())
	}

	w = pkgJSON(t, h, "POST", "/api/api-keys", `{"name":"writer","scopes":["write"]}`, "admin-114")
	if w.Code != 201 {
		t.Fatalf("create write %d %s", w.Code, w.Body.String())
	}
	var writer struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &writer)
	w = pkgJSON(t, h, "POST", "/api/packages/allow", `{"ecosystem":"python","name":"httpx","pin":"0.27.2"}`, writer.Secret)
	if w.Code != http.StatusForbidden {
		t.Fatalf("write allow %d %s", w.Code, w.Body.String())
	}
}
