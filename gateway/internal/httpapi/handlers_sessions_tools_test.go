// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestChat_SessionsListHistoryTools(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "sess-tools"})
	other, _ := st.CreateAgent(store.Agent{AgentKey: "other-t", TenantID: "other"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID, Label: "mine"})
	foreign, _ := st.CreateSession(store.Session{AgentID: other.ID, TenantID: "other", Label: "nope"})
	_, _ = st.AddMessage(store.Message{SessionID: sess.ID, Role: "user", Content: "hello-history"})
	_, _ = st.AddMessage(store.Message{SessionID: foreign.ID, Role: "user", Content: "secret-foreign"})

	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "l1", Name: "sessions_list", Arguments: map[string]any{}}}},
		{ToolCalls: []llm.ToolCall{{ID: "h1", Name: "sessions_history", Arguments: map[string]any{"session_id": sess.ID}}}},
		{Text: "listed sessions"},
	}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(32), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"show sessions"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	foundList, foundHist := false, false
	for _, tools := range scripted.RecordedTools {
		for _, ts := range tools {
			if ts.Name == pipeline.ToolSessionsList {
				foundList = true
			}
			if ts.Name == pipeline.ToolSessionsHistory {
				foundHist = true
			}
		}
	}
	if !foundList || !foundHist {
		t.Fatalf("tools missing %#v", scripted.RecordedTools)
	}
	if !strings.Contains(w.Body.String(), "listed sessions") {
		t.Fatalf("reply %s", w.Body.String())
	}
	msgs, _ := st.ListMessages(sess.ID)
	sawList, sawHist, leaked := false, false, false
	for _, m := range msgs {
		if m == nil || m.Role != "tool" {
			continue
		}
		if strings.Contains(m.Content, sess.ID) && strings.Contains(m.Content, "mine") && !strings.Contains(m.Content, "hello-history") {
			sawList = true
		}
		if strings.Contains(m.Content, "hello-history") {
			sawHist = true
		}
		if strings.Contains(m.Content, "secret-foreign") || strings.Contains(m.Content, "nope") {
			leaked = true
		}
	}
	if !sawList || !sawHist || leaked {
		t.Fatalf("tool results list=%v hist=%v leak=%v msgs=%#v", sawList, sawHist, leaked, msgs)
	}
}

func TestChat_SessionsHistoryOtherTenantFailClosed(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "sess-jail"})
	other, _ := st.CreateAgent(store.Agent{AgentKey: "sess-other", TenantID: "other"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	foreign, _ := st.CreateSession(store.Session{AgentID: other.ID, TenantID: "other"})
	_, _ = st.AddMessage(store.Message{SessionID: foreign.ID, Role: "user", Content: "secret-foreign"})

	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "h1", Name: "sessions_history", Arguments: map[string]any{"session_id": foreign.ID}}}},
		{Text: "denied"},
	}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(8), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"history"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	msgs, _ := st.ListMessages(sess.ID)
	sawErr := false
	for _, m := range msgs {
		if m == nil || m.Role != "tool" {
			continue
		}
		if strings.Contains(m.Content, "not found") {
			sawErr = true
		}
		if strings.Contains(m.Content, "secret-foreign") {
			t.Fatalf("leaked %#v", m)
		}
	}
	if !sawErr {
		t.Fatalf("expected fail-closed, msgs=%#v", msgs)
	}
}

func TestChat_SessionsToolsNotKeywordMatched(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "kw"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "sessions_list sessions_history"}}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(8), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"sessions_list please"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	msgs, _ := st.ListMessages(sess.ID)
	for _, m := range msgs {
		if m != nil && m.Role == "tool" {
			t.Fatalf("keyword must not dispatch %#v", msgs)
		}
	}
}

func TestChat_SessionsHistoryMissingFailClosed(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "miss"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "h1", Name: "sessions_history", Arguments: map[string]any{"session_id": "no-such"}}}},
		{Text: "missing"},
	}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(8), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"hist"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	var parsed map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	msgs, _ := st.ListMessages(sess.ID)
	sawErr := false
	for _, m := range msgs {
		if m != nil && m.Role == "tool" && strings.Contains(m.Content, "not found") {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatalf("expected not found, msgs=%#v", msgs)
	}
}
