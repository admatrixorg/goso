// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestConnector_CRUDAndApproval(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	manifest := `{
		"schema_version":"1.0",
		"tools":[
			{"name":"contact_search","description":"search","requires_approval":false,
			 "input_schema":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}},
			{"name":"message_send","description":"send","requires_approval":true,
			 "input_schema":{"type":"object","properties":{"contact_id":{"type":"string"},"text":{"type":"string"}},"required":["contact_id","text"]}}
		]
	}`
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("GET /manifest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, manifest)
	})
	mux.HandleFunc("POST /tools/contact_search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"contacts": []string{"A"}})
	})
	mux.HandleFunc("POST /tools/message_send", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("message_send must not be invoked")
	})
	fake := httptest.NewServer(mux)
	defer fake.Close()

	st := store.New()
	h := NewRouter(Options{Store: st, Version: "0.1.0"})

	// existing routes still work
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Fatalf("healthz %d", w.Code)
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents", bytes.NewBufferString(`{"agent_key":"a1","display_name":"A1"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("agent %d %s", w.Code, w.Body.String())
	}
	var agent map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &agent)
	agentID := agent["id"].(string)

	body := `{"name":"zalocrm","transport":"http","endpoint":"` + fake.URL + `","enabled":true}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/connectors", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create connector %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/connectors", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "zalocrm") {
		t.Fatalf("list connectors %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/agents/"+agentID+"/connectors", bytes.NewBufferString(`{"connector":"zalocrm"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("link %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/tools/invoke", bytes.NewBufferString(`{"connector":"zalocrm","tool":"contact_search","arguments":{"query":"A"}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("invoke %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/tools/invoke", bytes.NewBufferString(`{"connector":"zalocrm","tool":"message_send","arguments":{"contact_id":"1","text":"hi"}}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("pending invoke %d %s", w.Code, w.Body.String())
	}
	var inv map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &inv)
	res, _ := inv["result"].(map[string]any)
	if res["status"] != "pending_approval" {
		t.Fatalf("expected pending_approval, got %s", w.Body.String())
	}
	apprID, _ := res["approval_id"].(string)
	if apprID == "" {
		t.Fatalf("no approval_id %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/approvals/"+apprID+"/decision", bytes.NewBufferString(`{"decision":"reject","reason":"not now"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("decision %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/events?kind=human_feedback", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "human_feedback") {
		t.Fatalf("events %d %s", w.Code, w.Body.String())
	}

	// sessions + chat still work
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(`{"agent_id":"`+agentID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("session %d %s", w.Code, w.Body.String())
	}
	var sess map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	sid := sess["id"].(string)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sid+`","message":"hi there"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "echo: hi there") {
		t.Fatalf("chat body %s", w.Body.String())
	}
}
