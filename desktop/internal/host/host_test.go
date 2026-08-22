// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package host

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestStartSQLiteAndGatewayReuse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goso.db")
	t.Setenv("GOSO_DB_PATH", path)
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	t.Setenv("GOSO_RATE_LIMIT", "0")

	rt, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	if rt.DBPath != path {
		t.Fatalf("DBPath=%q want %q", rt.DBPath, path)
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

func TestIsGatewayPath(t *testing.T) {
	if !IsGatewayPath("/healthz") || !IsGatewayPath("/metrics") || !IsGatewayPath("/api/agents") || !IsGatewayPath("/ws") {
		t.Fatal("expected gateway paths")
	}
	if IsGatewayPath("/") || IsGatewayPath("/assets/index.js") {
		t.Fatal("UI paths should not be gateway")
	}
}

func TestMiddlewareUsesGatewayHandler(t *testing.T) {
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
