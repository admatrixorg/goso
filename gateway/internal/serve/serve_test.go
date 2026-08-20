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
	st := store.New()
	h, status := New(st, "test")
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
}
