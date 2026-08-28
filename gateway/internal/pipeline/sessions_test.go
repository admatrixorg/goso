// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestDispatchSessionTool_ListJailsTenantAndCap(t *testing.T) {
	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "a", TenantID: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateAgent(store.Agent{AgentKey: "b", TenantID: "other"})
	if err != nil {
		t.Fatal(err)
	}
	keep, err := st.CreateSession(store.Session{AgentID: a.ID, TenantID: "acme", Label: "keep-me"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSession(store.Session{AgentID: b.ID, TenantID: "other", Label: "secret"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < SessionToolCap; i++ {
		if _, err := st.CreateSession(store.Session{AgentID: a.ID, TenantID: "acme"}); err != nil {
			t.Fatal(err)
		}
	}
	body, err := DispatchSessionTool(st, "acme", llm.ToolCall{Name: ToolSessionsList})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "secret") {
		t.Fatalf("leaked other tenant %s", body)
	}
	var out struct {
		Sessions []sessionListItem `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Sessions) != SessionToolCap {
		t.Fatalf("cap %d", len(out.Sessions))
	}
	for _, s := range out.Sessions {
		if s.ID == keep.ID && s.Label != "keep-me" {
			t.Fatalf("label %q", s.Label)
		}
		if strings.Contains(s.Label, "user:") || strings.Contains(s.Label, "{") {
			t.Fatalf("unexpected dump %#v", s)
		}
	}
	if strings.Contains(body, `"content"`) {
		t.Fatalf("sessions_list must not dump message bodies %s", body)
	}
}

func TestDispatchSessionTool_HistoryFailClosedAndCap(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "ha", TenantID: "acme"})
	b, _ := st.CreateAgent(store.Agent{AgentKey: "hb", TenantID: "other"})
	mine, _ := st.CreateSession(store.Session{AgentID: a.ID, TenantID: "acme"})
	theirs, _ := st.CreateSession(store.Session{AgentID: b.ID, TenantID: "other"})
	for i := 0; i < SessionToolCap+1; i++ {
		if _, err := st.AddMessage(store.Message{SessionID: mine.ID, Role: "user", Content: "m"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.AddMessage(store.Message{SessionID: theirs.ID, Role: "user", Content: "secret-body"}); err != nil {
		t.Fatal(err)
	}

	_, err := DispatchSessionTool(st, "acme", llm.ToolCall{Name: ToolSessionsHistory, Arguments: map[string]any{"session_id": "  "}})
	if err == nil {
		t.Fatal("empty session_id must fail")
	}
	_, err = DispatchSessionTool(st, "acme", llm.ToolCall{Name: ToolSessionsHistory, Arguments: map[string]any{"session_id": "missing"}})
	if err == nil {
		t.Fatal("missing must fail")
	}
	body, err := DispatchSessionTool(st, "acme", llm.ToolCall{Name: ToolSessionsHistory, Arguments: map[string]any{"session_id": theirs.ID}})
	if err == nil || strings.Contains(body, "secret-body") {
		t.Fatalf("other tenant must fail-closed %v %s", err, body)
	}

	got, err := DispatchSessionTool(st, "acme", llm.ToolCall{Name: ToolSessionsHistory, Arguments: map[string]any{"session_id": mine.ID}})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		SessionID string               `json:"session_id"`
		Messages  []sessionHistoryItem `json:"messages"`
	}
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatal(err)
	}
	if out.SessionID != mine.ID || len(out.Messages) != SessionToolCap {
		t.Fatalf("history %+v", out)
	}
}
