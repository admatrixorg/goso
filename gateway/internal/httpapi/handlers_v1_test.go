// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestV1AliasesMatchAPI(t *testing.T) {
	st, h := newTestServer()
	a, err := st.CreateAgent(store.Agent{AgentKey: "v1a", DisplayName: "V1"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"/providers",
		"/agents",
		"/sessions",
		"/channels",
		"/skills",
		"/webhooks",
		"/teams",
		"/cron",
		"/memory?session_id=" + sess.ID,
	} {
		assertSameGET(t, h, "/api"+path, "/v1"+path)
	}

	wAPI := httptest.NewRecorder()
	h.ServeHTTP(wAPI, httptest.NewRequest(http.MethodGet, "/api/memory", nil))
	wV1 := httptest.NewRecorder()
	h.ServeHTTP(wV1, httptest.NewRequest(http.MethodGet, "/v1/memory", nil))
	if wAPI.Code != http.StatusOK || wV1.Code != wAPI.Code || wAPI.Body.String() != wV1.Body.String() {
		t.Fatalf("memory list-all api=%d %s v1=%d %s", wAPI.Code, wAPI.Body.String(), wV1.Code, wV1.Body.String())
	}
}

func TestV1DoesNotInventCRUD(t *testing.T) {
	st, h := newTestServer()
	a, err := st.CreateAgent(store.Agent{AgentKey: "v1b", DisplayName: "V1B"})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/agents/"+a.ID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/agents/{id} %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/agents/"+a.ID, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /v1/agents/{id} must not be invented, got %d %s", w.Code, w.Body.String())
	}
}

func TestV1ChatSameHandler(t *testing.T) {
	st, h := newTestServer()
	a, err := st.CreateAgent(store.Agent{AgentKey: "v1c", DisplayName: "V1C"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"hi v1"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /v1/chat %d %s", w.Code, w.Body.String())
	}
	var chat map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &chat); err != nil {
		t.Fatal(err)
	}
	if chat["reply"] != "echo: hi v1" {
		t.Fatalf("chat reply %v", chat)
	}
}

func assertSameGET(t *testing.T, h http.Handler, apiPath, v1Path string) {
	t.Helper()
	wAPI := httptest.NewRecorder()
	h.ServeHTTP(wAPI, httptest.NewRequest(http.MethodGet, apiPath, nil))
	wV1 := httptest.NewRecorder()
	h.ServeHTTP(wV1, httptest.NewRequest(http.MethodGet, v1Path, nil))
	if wAPI.Code != http.StatusOK {
		t.Fatalf("GET %s %d %s", apiPath, wAPI.Code, wAPI.Body.String())
	}
	if wV1.Code != wAPI.Code {
		t.Fatalf("GET %s %d vs %s %d body %s", apiPath, wAPI.Code, v1Path, wV1.Code, wV1.Body.String())
	}
	if wAPI.Body.String() != wV1.Body.String() {
		t.Fatalf("GET %s vs %s body mismatch\n%s\n%s", apiPath, v1Path, wAPI.Body.String(), wV1.Body.String())
	}
}
