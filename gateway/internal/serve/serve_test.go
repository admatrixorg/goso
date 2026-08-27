// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestNewHealthzAndAgent(t *testing.T) {
	t.Setenv("GOSO_DEV_MODE", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	t.Setenv("GOSO_E2E_SCRIPTED", "")
	st := store.New()
	h, status := New(st, "test")
	if !status.DevMode {
		t.Fatal("expected explicit GOSO_DEV_MODE passthrough")
	}
	if status.Provider == "" {
		t.Fatal("expected provider name")
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz status %d", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != true {
		t.Fatalf("healthz body %+v", body)
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents", strings.NewReader(`{"agent_key":"desk","display_name":"Desktop"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create agent status %d body %s", rr.Code, rr.Body.String())
	}
	list := st.ListAgents()
	if len(list) != 1 || list[0].AgentKey != "desk" {
		t.Fatalf("store not reused: %+v", list)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/stats", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("stats status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestNewRequiresToken(t *testing.T) {
	t.Setenv("GOSO_DEV_MODE", "")
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	st := store.New()
	h, status := New(st, "test")
	if status.Auth || status.DevMode {
		t.Fatalf("status %+v", status)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("empty token /api/agents %d body %s", rr.Code, rr.Body.String())
	}

	t.Setenv("GOSO_ADMIN_TOKEN", "secret-016")
	h, status = New(st, "test")
	if !status.Auth {
		t.Fatal("expected auth on")
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer %d", rr.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer secret-016")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bearer /api/agents %d %s", rr.Code, rr.Body.String())
	}
}
