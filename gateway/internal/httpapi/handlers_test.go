// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func newTestServer() (*store.Store, http.Handler) {
	st := store.New()
	mux := Router(st, "0.1.0").(*http.ServeMux)
	RegisterWS(mux, st, llm.Echo{})
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
	t.Setenv("GOSO_QUOTA_DAY", "")
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
	t.Setenv("GOSO_QUOTA_DAY", "")
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

func TestPatchAgentOrchestrationMode(t *testing.T) {
	_, h := newTestServer()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents", bytes.NewBufferString(`{"agent_key":"orch","display_name":"Orch","orchestration_mode":"auto","instructions":"keep-me"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create agent %d %s", w.Code, w.Body.String())
	}
	var created store.Agent
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.OrchestrationMode != "auto" {
		t.Fatalf("create orchestration_mode %q", created.OrchestrationMode)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/agents/"+created.ID, bytes.NewBufferString(`{"orchestration_mode":"manual"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("patch agent %d %s", w.Code, w.Body.String())
	}
	var patched store.Agent
	if err := json.Unmarshal(w.Body.Bytes(), &patched); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if patched.OrchestrationMode != "manual" {
		t.Fatalf("patch orchestration_mode %q", patched.OrchestrationMode)
	}
	if patched.Instructions != "keep-me" {
		t.Fatalf("patch wiped instructions %q", patched.Instructions)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents/"+created.ID, nil))
	if w.Code != 200 {
		t.Fatalf("get agent %d %s", w.Code, w.Body.String())
	}
	var got store.Agent
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.OrchestrationMode != "manual" {
		t.Fatalf("get orchestration_mode %q", got.OrchestrationMode)
	}
	if got.Instructions != "keep-me" {
		t.Fatalf("get wiped instructions %q", got.Instructions)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/agents/"+created.ID, bytes.NewBufferString(`{"orchestration_mode":"banana"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("bad mode expected 400, got %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/agents/"+created.ID, bytes.NewBufferString(`{"orchestration_mode":""}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("empty mode expected 400, got %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/agents/missing", bytes.NewBufferString(`{"orchestration_mode":"auto"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("missing agent expected 404, got %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/agents/"+created.ID, bytes.NewBufferString(`{"model":"echo","instructions":"prefix"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("patch fields %d %s", w.Code, w.Body.String())
	}
	var fields store.Agent
	if err := json.Unmarshal(w.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode fields: %v", err)
	}
	if fields.Model != "echo" || fields.Instructions != "prefix" || fields.OrchestrationMode != "manual" {
		t.Fatalf("optional fields %+v", fields)
	}
}

func setupChat(t *testing.T, h http.Handler) (agentID, sessID string) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents", bytes.NewBufferString(`{"agent_key":"q1","display_name":"Q1"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create agent %d %s", w.Code, w.Body.String())
	}
	var a map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	agentID = a["id"].(string)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(`{"agent_id":"`+agentID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create session %d %s", w.Code, w.Body.String())
	}
	var sess map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	return agentID, sess["id"].(string)
}

func postChat(h http.Handler, sessID, msg string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sessID+`","message":"`+msg+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	return w
}

func TestQuotaAPI_Disabled(t *testing.T) {
	// AC-02: quota disabled (0) never 429 even after many chats.
	t.Setenv("GOSO_QUOTA_DAY", "0")
	st := store.New()
	meter := billing.New()
	h := RouterWithBilling(st, "0.1.0", llm.Echo{}, nil, nil, nil, meter)
	_, sessID := setupChat(t, h)

	for i := 0; i < 5; i++ {
		w := postChat(h, sessID, "hello quota")
		if w.Code != 200 {
			t.Fatalf("chat %d status %d %s", i, w.Code, w.Body.String())
		}
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/quota", nil))
	if w.Code != 200 {
		t.Fatalf("quota %d %s", w.Code, w.Body.String())
	}
	var q map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &q)
	if q["enabled"] != false {
		t.Fatalf("enabled %+v", q)
	}
	if q["requestsToday"].(float64) != 5 {
		t.Fatalf("requestsToday %v body %s", q["requestsToday"], w.Body.String())
	}
	day, _ := q["day"].(map[string]any)
	if day["limit"].(float64) != 0 {
		t.Fatalf("limit %v", day)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Fatalf("healthz %d", w.Code)
	}
}

func TestQuotaAPI_SecondChat429(t *testing.T) {
	// AC-03: GOSO_QUOTA_DAY=1 → first chat 200, second 429.
	t.Setenv("GOSO_QUOTA_DAY", "1")
	st := store.New()
	meter := billing.New()
	h := RouterWithBilling(st, "0.1.0", llm.Echo{}, nil, nil, nil, meter)
	agentID, sessID := setupChat(t, h)

	w := postChat(h, sessID, "abcd")
	if w.Code != 200 {
		t.Fatalf("first chat %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/quota", nil))
	if w.Code != 200 {
		t.Fatalf("quota %d %s", w.Code, w.Body.String())
	}
	var q map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &q)
	if q["enabled"] != true {
		t.Fatalf("enabled %+v", q)
	}
	if q["requestsToday"].(float64) != 1 {
		t.Fatalf("requestsToday %v", q["requestsToday"])
	}
	if q["inputTokensToday"].(float64) < 1 {
		t.Fatalf("inputTokensToday %v", q["inputTokensToday"])
	}
	day, _ := q["day"].(map[string]any)
	if day["limit"].(float64) != 1 || day["used"].(float64) < 1 {
		t.Fatalf("day %v", day)
	}

	w = postChat(h, sessID, "abcd")
	if w.Code != 429 {
		t.Fatalf("second chat want 429, got %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After")
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "quota_exceeded" {
		t.Fatalf("body %s", w.Body.String())
	}

	// AC-01: GET /api/usage still 200; 429 must not record a second call.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/usage?agent_id="+agentID, nil))
	if w.Code != 200 {
		t.Fatalf("usage %d %s", w.Code, w.Body.String())
	}
	var usage map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &usage)
	if usage["calls"].(float64) != 1 {
		t.Fatalf("calls after 429 %v", usage)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Fatalf("healthz must never 429, got %d", w.Code)
	}
}

func TestHandleChat_QuotaWrappers(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "1")
	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "wrap", DisplayName: "W"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"session_id":"` + sess.ID + `","message":"abcd"}`

	meter := billing.New()
	echo := handleChat(st, meter)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(body))
	echo.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("handleChat first %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(body))
	echo.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("handleChat second %d %s", w.Code, w.Body.String())
	}

	meter2 := billing.New()
	llmH := handleChatWithLLM(st, llm.Echo{}, meter2)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(body))
	llmH.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("handleChatWithLLM first %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(body))
	llmH.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("handleChatWithLLM second %d %s", w.Code, w.Body.String())
	}
}

func TestChat_PromptModeUnknown400(t *testing.T) {
	h := Router(store.New(), "0.1.0")
	_, sessID := setupChat(t, h)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sessID+`","message":"hi","prompt_mode":"weird"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("want 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestChat_PromptModeTaskOK(t *testing.T) {
	h := Router(store.New(), "0.1.0")
	_, sessID := setupChat(t, h)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sessID+`","message":"hi","prompt_mode":"task"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200, got %d %s", w.Code, w.Body.String())
	}
}

func TestPatchSession_PromptMode(t *testing.T) {
	_, h := newTestServer()
	_, sessID := setupChat(t, h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/sessions/"+sessID, bytes.NewBufferString(`{"prompt_mode":"minimal"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("patch %d %s", w.Code, w.Body.String())
	}
	var sess map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	if sess["prompt_mode"] != "minimal" {
		t.Fatalf("prompt_mode %v", sess["prompt_mode"])
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions", nil))
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"prompt_mode":"minimal"`) {
		t.Fatalf("list missing stored mode: %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/sessions/"+sessID, bytes.NewBufferString(`{"prompt_mode":"weird"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("unknown want 400, got %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/sessions/missing", bytes.NewBufferString(`{"prompt_mode":"full"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("missing want 404, got %d %s", w.Code, w.Body.String())
	}
}

func TestDeleteSession(t *testing.T) {
	_, h := newTestServer()
	_, sessID := setupChat(t, h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/sessions/"+sessID+"/messages", bytes.NewBufferString(`{"content":"keep?"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("add message %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/sessions/"+sessID, nil))
	if w.Code != 200 {
		t.Fatalf("delete %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("delete body %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions/"+sessID+"/messages", nil))
	if w.Code != 404 {
		t.Fatalf("messages after delete want 404, got %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions", nil))
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), sessID) {
		t.Fatalf("list still has deleted session: %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/sessions/"+sessID, nil))
	if w.Code != 404 {
		t.Fatalf("second delete want 404, got %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/sessions/missing", nil))
	if w.Code != 404 {
		t.Fatalf("missing want 404, got %d %s", w.Code, w.Body.String())
	}
}

func TestChat_UsesStoredPromptMode(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "pm1", DisplayName: "Agent One"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "ok"}}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(32), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/sessions/"+sess.ID, bytes.NewBufferString(`{"prompt_mode":"none"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("patch %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	if len(scripted.Recorded) == 0 {
		t.Fatal("empty recorded")
	}
	for _, m := range scripted.Recorded[0] {
		if m.Role == "system" && m.Content != "" {
			t.Fatalf("stored none sent system: %#v", scripted.Recorded[0])
		}
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"hi","prompt_mode":"full"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("override %d %s", w.Code, w.Body.String())
	}
	last := scripted.Recorded[len(scripted.Recorded)-1]
	sys := ""
	for _, m := range last {
		if m.Role == "system" {
			sys += m.Content
		}
	}
	if !strings.Contains(sys, "You are Agent One") {
		t.Fatalf("request full should override stored none: %q", sys)
	}
}

func TestProvidersAPI_ConfiguredNamesOnly(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv("GOSO_GROQ_API_KEY", "k-groq")
	st := store.New()
	h := NewRouter(Options{
		Store: st, Version: "0.1.0", Provider: llm.Echo{},
		LLM: llm.NewRegistry(),
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/providers", nil))
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Providers []llm.ProviderInfo `json:"providers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := map[string]llm.ProviderInfo{}
	for _, p := range body.Providers {
		got[p.Name] = p
	}
	if _, ok := got["echo"]; !ok {
		t.Fatalf("missing echo: %s", w.Body.String())
	}
	if g, ok := got["groq"]; !ok || g.Type != "openai-compat" || !g.KeySet || g.Source != "env" {
		t.Fatalf("groq %+v", g)
	}
	if _, ok := got["openai"]; ok {
		t.Fatal("openai must be absent")
	}
	if strings.Contains(w.Body.String(), `"api_key"`) || strings.Contains(w.Body.String(), "sk-") || strings.Contains(w.Body.String(), "gsk_") {
		t.Fatal("response leaked a key")
	}
}
