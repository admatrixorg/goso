// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestChatTurn_AgentModelMentionAndSwitch(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	t.Setenv("GOSO_SSRF", "0")
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if m, ok := req["model"].(string); ok {
			gotModel = m
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"xin chao"}}]}`))
	}))
	defer upstream.Close()

	st := store.New()
	p := &llm.OpenAI{APIKey: "test-key", BaseURL: upstream.URL, Client: upstream.Client(), Model: "fallback-model"}
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: p})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents", strings.NewReader(`{"agent_key":"dau-tu","display_name":"Đầu tư","model":"grok-4.5","llm_provider":""}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("agent A %d %s", w.Code, w.Body.String())
	}
	var a map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	agentA := a["id"].(string)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/agents", strings.NewReader(`{"agent_key":"phap-che","display_name":"Pháp chế","model":"gemini-2.5-flash","llm_provider":""}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("agent B %d %s", w.Code, w.Body.String())
	}
	var b map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	agentB := b["id"].(string)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/sessions", strings.NewReader(`{"agent_id":"`+agentA+`","label":"quy"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("session %d %s", w.Code, w.Body.String())
	}
	var sess map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &sess)
	sessID := sess["id"].(string)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"session_id":"`+sessID+`","message":"tran dinh gia?"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat theo agent %d %s", w.Code, w.Body.String())
	}
	if gotModel != "grok-4.5" {
		t.Fatalf("theo agent model %q", gotModel)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"session_id":"`+sessID+`","message":"@Pháp chế kiểm tra điều khoản","agent_id":"`+agentB+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("mention %d %s", w.Code, w.Body.String())
	}
	if gotModel != "gemini-2.5-flash" {
		t.Fatalf("mention model %q", gotModel)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions/"+sessID, nil))
	// session GET may be list-only; confirm via list
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions", nil))
	if !strings.Contains(w.Body.String(), `"agent_id":"`+agentA+`"`) {
		t.Fatalf("mention changed session: %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/sessions/"+sessID, strings.NewReader(`{"model_override":"gpt-4o","provider_override":""}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"model_override":"gpt-4o"`) {
		t.Fatalf("patch model %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"session_id":"`+sessID+`","message":"dung model da chon"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("override chat %d %s", w.Code, w.Body.String())
	}
	if gotModel != "gpt-4o" {
		t.Fatalf("override model %q", gotModel)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/sessions/"+sessID, strings.NewReader(`{"prompt_mode":"minimal"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"model_override":"gpt-4o"`) {
		t.Fatalf("prompt patch cleared model %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/sessions/"+sessID, strings.NewReader(`{"agent_id":"`+agentB+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("switch agent %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"session_id":"`+sessID+`","message":"nope","agent_id":"missing-agent"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("missing mention %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions/"+sessID+"/messages", nil))
	var payload struct {
		Messages []store.Message `json:"messages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var sawEvent, sawMention, sawOverride bool
	for _, m := range payload.Messages {
		if m.Role == "event" && m.AgentID == agentB {
			sawEvent = true
		}
		if m.Role == "user" && m.MentionAgentID == agentB && m.AgentID == agentA {
			sawMention = true
		}
		if m.Role == "assistant" && m.Model == "gpt-4o" && m.AgentID == agentA {
			sawOverride = true
		}
	}
	if !sawEvent || !sawMention || !sawOverride {
		t.Fatalf("messages event=%v mention=%v override=%v %#v", sawEvent, sawMention, sawOverride, payload.Messages)
	}
}
