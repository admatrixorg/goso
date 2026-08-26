// Copyright (c) 2026 MQ Global — GOSO Desktop. Clean-room implementation.

package host

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	goso "github.com/mqglobal/goso/gateway"
)

func TestDefaultDBPathEnvOverride(t *testing.T) {
	t.Setenv("GOSO_DB_PATH", "/tmp/custom-goso.db")
	if got := DefaultDBPath(); got != "/tmp/custom-goso.db" {
		t.Fatalf("DefaultDBPath=%q", got)
	}
}

func TestDefaultDBPathPlatform(t *testing.T) {
	t.Setenv("GOSO_DB_PATH", "")
	p := DefaultDBPath()
	if !strings.HasSuffix(p, "goso.db") {
		t.Fatalf("expected goso.db suffix, got %q", p)
	}
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(p, filepath.Join("Application Support", "GOSO")) {
			t.Fatalf("macOS path should be Application Support/GOSO, got %q", p)
		}
	case "windows":
		if !strings.Contains(filepath.ToSlash(p), "/GOSO/") {
			t.Fatalf("windows path should contain GOSO, got %q", p)
		}
	default:
		if !strings.Contains(p, "GOSO") {
			t.Fatalf("path should contain GOSO, got %q", p)
		}
	}
}

func TestDefaultTokenPath(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "admin.token")
	t.Setenv("GOSO_ADMIN_TOKEN_PATH", want)
	if got := DefaultTokenPath(); got != want {
		t.Fatalf("DefaultTokenPath=%q want %q", got, want)
	}
	t.Setenv("GOSO_ADMIN_TOKEN_PATH", "")
	t.Setenv("GOSO_DB_PATH", filepath.Join(dir, "goso.db"))
	if got := DefaultTokenPath(); got != want {
		t.Fatalf("token next to db: %q want %q", got, want)
	}
}

func TestStartSQLiteAndGatewayReuse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goso.db")
	t.Setenv("GOSO_DB_PATH", path)
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	t.Setenv("GOSO_DEV_MODE", "1")
	t.Setenv("GOSO_RATE_LIMIT", "0")
	t.Setenv("GOSO_ADMIN_TOKEN_PATH", filepath.Join(dir, "admin.token"))

	rt, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	if rt.DBPath != path {
		t.Fatalf("DBPath=%q want %q", rt.DBPath, path)
	}
	if rt.AdminToken() != "" {
		t.Fatal("dev mode should not mint a local token")
	}

	assets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ui"))
	})
	ts := httptest.NewServer(Middleware(rt.Handler)(assets))
	defer ts.Close()

	res, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("healthz %d", res.StatusCode)
	}
	var health map[string]any
	if err := json.NewDecoder(res.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	if health["ok"] != true {
		t.Fatalf("healthz %+v", health)
	}

	res, err = http.Post(ts.URL+"/api/agents", "application/json", strings.NewReader(`{"agent_key":"desk","display_name":"Desktop"}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create agent %d %s", res.StatusCode, body)
	}

	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}

	// Re-open the same SQLite file through the public gateway facade.
	h2, close2, _, err := goso.OpenLocal(path, Version)
	if err != nil {
		t.Fatal(err)
	}
	defer close2()
	rr := httptest.NewRecorder()
	h2.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("reopen list %d %s", rr.Code, rr.Body.String())
	}
	var listed struct {
		Agents []struct {
			AgentKey string `json:"agent_key"`
		} `json:"agents"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Agents) != 1 || listed.Agents[0].AgentKey != "desk" {
		t.Fatalf("sqlite reuse failed: %+v", listed.Agents)
	}

	res, err = http.Get(ts.URL + "/index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	ui, _ := io.ReadAll(res.Body)
	if string(ui) != "ui" {
		t.Fatalf("asset pass-through got %q", ui)
	}
}

func TestLocalTokenAuth(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "goso.db")
	tokFile := filepath.Join(dir, "admin.token")
	t.Setenv("GOSO_DB_PATH", db)
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	t.Setenv("GOSO_DEV_MODE", "")
	t.Setenv("GOSO_RATE_LIMIT", "0")
	t.Setenv("GOSO_ADMIN_TOKEN_PATH", tokFile)

	rt, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	token := rt.AdminToken()
	if token == "" {
		t.Fatal("expected generated local token")
	}
	b, err := os.ReadFile(tokFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != token {
		t.Fatalf("token file mismatch")
	}
	info, err := os.Stat(tokFile)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("token file perm %o want 0600", perm)
	}

	assets := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	ts := httptest.NewServer(Middleware(rt.Handler)(assets))
	defer ts.Close()

	res, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("healthz %d (must not require token)", res.StatusCode)
	}

	res, err = http.Get(ts.URL + "/api/agents")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api/agents %d %s", res.StatusCode, body)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/agents", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("authenticated /api/agents %d %s", res.StatusCode, body)
	}

	logged := fmt.Sprintf("%s %v %#v %+v", rt.Token, rt.Token, rt.Token, rt)
	if strings.Contains(logged, token) {
		t.Fatalf("token leaked into log-shaped output")
	}
}

func TestLocalTokenReusedOnSecondStart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOSO_DB_PATH", filepath.Join(dir, "goso.db"))
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	t.Setenv("GOSO_DEV_MODE", "")
	t.Setenv("GOSO_RATE_LIMIT", "0")
	t.Setenv("GOSO_ADMIN_TOKEN_PATH", filepath.Join(dir, "admin.token"))

	rt1, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	first := rt1.AdminToken()
	if first == "" {
		t.Fatal("empty token")
	}
	if err := rt1.Close(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOSO_ADMIN_TOKEN", "")
	rt2, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer rt2.Close()
	if rt2.AdminToken() != first {
		t.Fatal("second start minted a different token")
	}
}

func TestEnvTokenPreferredOverFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOSO_DB_PATH", filepath.Join(dir, "goso.db"))
	t.Setenv("GOSO_ADMIN_TOKEN", "from-env-not-file")
	t.Setenv("GOSO_DEV_MODE", "")
	t.Setenv("GOSO_ADMIN_TOKEN_PATH", filepath.Join(dir, "admin.token"))
	got, err := ResolveAdminToken()
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-env-not-file" {
		t.Fatalf("got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "admin.token")); !os.IsNotExist(err) {
		t.Fatal("env token should not write a file")
	}
}

func TestSecretRedacts(t *testing.T) {
	s := secret("super-secret-token-value")
	out := fmt.Sprintf("%s %v %#v", s, s, s)
	if strings.Contains(out, "super-secret") {
		t.Fatalf("token leaked: %q", out)
	}
}

func TestIsGatewayPath(t *testing.T) {
	if !IsGatewayPath("/healthz") || !IsGatewayPath("/metrics") || !IsGatewayPath("/api/agents") || !IsGatewayPath("/ws") {
		t.Fatal("expected gateway paths")
	}
	if IsGatewayPath("/") || IsGatewayPath("/assets/index.js") {
		t.Fatal("UI paths should not be gateway")
	}
}

func TestMiddlewareUsesGatewayHandler(t *testing.T) {
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	t.Setenv("GOSO_DEV_MODE", "1")
	h, closeFn, _, err := goso.OpenLocal(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	rr := httptest.NewRecorder()
	Middleware(h)(http.NotFoundHandler()).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
}
