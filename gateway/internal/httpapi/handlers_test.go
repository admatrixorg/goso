// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func newTestServer() (*store.Store, http.Handler) {
	st := store.New()
	mux := Router(st, "0.1.0").(*http.ServeMux)
	RegisterWS(mux)
	return st, mux
}

func TestHealthz(t *testing.T) {
	_, h := newTestServer()
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("healthz status %d", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["ok"] != true {
		t.Fatalf("healthz body %s", w.Body.String())
	}
}

func TestAgentsAndSessions(t *testing.T) {
	_, h := newTestServer()
	// create agent
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents", bytes.NewBufferString(`{"agent_key":"a1","display_name":"A1"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create agent %d %s", w.Code, w.Body.String())
	}
	var a map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	agentID := a["id"].(string)

	// list agents
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents", nil))
	if w.Code != 200 {
		t.Fatalf("list agents %d", w.Code)
	}

	// get agent
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents/"+agentID, nil))
	if w.Code != 200 {
		t.Fatalf("get agent %d %s", w.Code, w.Body.String())
	}

	// create session
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(`{"agent_id":"`+agentID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create session %d %s", w.Code, w.Body.String())
	}
	var sess map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	sessID := sess["id"].(string)

	// add message
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/sessions/"+sessID+"/messages", bytes.NewBufferString(`{"content":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("add message %d %s", w.Code, w.Body.String())
	}

	// list messages
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions/"+sessID+"/messages", nil))
	if w.Code != 200 {
		t.Fatalf("list messages %d %s", w.Code, w.Body.String())
	}
	var lm map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &lm)
	if len(lm["messages"].([]any)) != 1 {
		t.Fatalf("messages len %v", lm)
	}

	// chat echo
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sessID+`","message":"hi there"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	var chat map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &chat)
	if chat["reply"] != "echo: hi there" {
		t.Fatalf("chat reply %v", chat)
	}
}

func TestUsageAPI(t *testing.T) {
	st := store.New()
	meter := billing.New()
	h := RouterWithBilling(st, "0.1.0", llm.Echo{}, nil, nil, nil, meter)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents", bytes.NewBufferString(`{"agent_key":"u1","display_name":"U1"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create agent %d %s", w.Code, w.Body.String())
	}
	var a map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	agentID := a["id"].(string)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(`{"agent_id":"`+agentID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	var sess map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	sessID := sess["id"].(string)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sessID+`","message":"abcd"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}

	from := "2000-01-01"

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/usage?agent_id="+agentID+"&from="+from, nil))
	if w.Code != 200 {
		t.Fatalf("usage %d %s", w.Code, w.Body.String())
	}
	var usage map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &usage)
	if usage["calls"].(float64) != 1 {
		t.Fatalf("calls %v body %s", usage["calls"], w.Body.String())
	}
	if usage["prompt_tokens"].(float64) < 1 || usage["total_tokens"].(float64) < 1 {
		t.Fatalf("tokens %v", usage)
	}
	if usage["agent_id"] != agentID {
		t.Fatalf("agent_id %v", usage["agent_id"])
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/usage?agent_id="+agentID+"&provider=echo", nil))
	_ = json.Unmarshal(w.Body.Bytes(), &usage)
	if usage["calls"].(float64) != 1 || usage["provider"] != "echo" {
		t.Fatalf("provider echo %v", usage)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/usage?agent_id="+agentID+"&provider=openai", nil))
	_ = json.Unmarshal(w.Body.Bytes(), &usage)
	if usage["calls"].(float64) != 0 {
		t.Fatalf("provider openai should be empty %v", usage)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/usage?from=not-a-date", nil))
	if w.Code != 400 {
		t.Fatalf("invalid from %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/usage?to=2026-13-40", nil))
	if w.Code != 400 {
		t.Fatalf("invalid to %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/usage?from=2099-01-01&to=2099-01-02", nil))
	_ = json.Unmarshal(w.Body.Bytes(), &usage)
	if usage["calls"].(float64) != 0 {
		t.Fatalf("future range should be empty %v", usage)
	}
}

func TestValidation(t *testing.T) {
	_, h := newTestServer()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
