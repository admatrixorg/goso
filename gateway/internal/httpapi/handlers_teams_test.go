// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/team"
)

func postJSON(h http.Handler, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	return w
}

func patchJSON(h http.Handler, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	return w
}

func TestTeamsAPI_CRUDKanbanMailbox(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	_, h := newTestServer()
	w := postJSON(h, "/api/agents", `{"agent_key":"lead","display_name":"Lead"}`)
	if w.Code != 201 {
		t.Fatalf("lead %d %s", w.Code, w.Body.String())
	}
	var lead store.Agent
	_ = json.Unmarshal(w.Body.Bytes(), &lead)
	w = postJSON(h, "/api/agents", `{"agent_key":"mem","display_name":"Mem"}`)
	var mem store.Agent
	_ = json.Unmarshal(w.Body.Bytes(), &mem)

	w = postJSON(h, "/api/teams", `{"name":"Ops","lead_agent_id":"`+lead.ID+`"}`)
	if w.Code != 201 {
		t.Fatalf("team %d %s", w.Code, w.Body.String())
	}
	var tm store.Team
	_ = json.Unmarshal(w.Body.Bytes(), &tm)

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/teams", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"Ops"`) {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}

	w = postJSON(h, "/api/teams/"+tm.ID+"/members", `{"agent_id":"`+mem.ID+`","role":"member"}`)
	if w.Code != 201 {
		t.Fatalf("member %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/teams/"+tm.ID+"/members", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), mem.ID) {
		t.Fatalf("members %s", w.Body.String())
	}

	w = postJSON(h, "/api/teams/"+tm.ID+"/tasks", `{"title":"Ship","status":"todo"}`)
	if w.Code != 201 {
		t.Fatalf("task %d %s", w.Code, w.Body.String())
	}
	var task store.TeamTask
	_ = json.Unmarshal(w.Body.Bytes(), &task)

	w = httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/teams/"+tm.ID+"/tasks/"+task.ID, bytes.NewBufferString(`{"status":"doing"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"doing"`) {
		t.Fatalf("patch %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/teams/"+tm.ID+"/tasks?status=doing", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Ship") {
		t.Fatalf("kanban %s", w.Body.String())
	}

	w = postJSON(h, "/api/teams/"+tm.ID+"/messages", `{"from_agent_id":"`+lead.ID+`","body":"hi board"}`)
	if w.Code != 201 {
		t.Fatalf("mailbox %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/teams/"+tm.ID+"/messages", nil))
	if !strings.Contains(w.Body.String(), "hi board") {
		t.Fatalf("messages %s", w.Body.String())
	}

	w = postJSON(h, "/api/agents/"+lead.ID+"/links", `{"to_agent_id":"`+mem.ID+`","bidirectional":true}`)
	if w.Code != 201 {
		t.Fatalf("links %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents/"+lead.ID+"/links", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"bidirectional":true`) {
		t.Fatalf("list links %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "instructions") {
		t.Fatalf("links leaked prompt %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	put := httptest.NewRequest("PUT", "/api/teams/"+tm.ID, bytes.NewBufferString(`{"name":"Ops desk","lead_agent_id":"`+mem.ID+`"}`))
	put.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, put)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Ops desk") {
		t.Fatalf("update team %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/teams/"+tm.ID+"/members/"+mem.ID, nil))
	if w.Code != 200 {
		t.Fatalf("remove member %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/agents/"+lead.ID+"/links/"+mem.ID+"?pair=1", nil))
	if w.Code != 200 {
		t.Fatalf("unlink %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents/"+lead.ID+"/links", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), mem.ID) {
		t.Fatalf("unlinked leftover %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/teams/"+tm.ID, nil))
	if w.Code != 200 {
		t.Fatalf("delete team %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/teams/"+tm.ID, nil))
	if w.Code != 404 {
		t.Fatalf("deleted team %d %s", w.Code, w.Body.String())
	}
}

func TestTeamsAPI_LiteCapsAndBearer(t *testing.T) {
	t.Setenv("GOSO_LITE", "1")
	st := store.New()
	mux := Router(st, "0.1.0")
	h := auth.RequireToken("secret", []string{"/healthz"})(mux)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/teams", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth %d", w.Code)
	}

	create := func(key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/agents", bytes.NewBufferString(`{"agent_key":"`+key+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	var firstID string
	for i := 0; i < 5; i++ {
		w = create("lite" + strings.Repeat("x", i+1))
		if w.Code != 201 {
			t.Fatalf("agent %d %d %s", i, w.Code, w.Body.String())
		}
		if firstID == "" {
			var a store.Agent
			_ = json.Unmarshal(w.Body.Bytes(), &a)
			firstID = a.ID
		}
	}
	w = create("overflow")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "lite cap") {
		t.Fatalf("6th agent %d %s", w.Code, w.Body.String())
	}

	req := httptest.NewRequest("POST", "/api/teams", bytes.NewBufferString(`{"name":"one","lead_agent_id":"`+firstID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("team1 %d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest("POST", "/api/teams", bytes.NewBufferString(`{"name":"two","lead_agent_id":"`+firstID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "lite cap") {
		t.Fatalf("2nd team %d %s", w.Code, w.Body.String())
	}
}

func TestEvolutionAPI_Guardrail(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	st, h := newTestServer()
	w := postJSON(h, "/api/agents", `{"agent_key":"evo","display_name":"Evo"}`)
	var a store.Agent
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	st.RecordChatRun(a.ID)
	st.RecordChatRun(a.ID)
	st.RecordChatRun(a.ID)
	st.RecordToolUse(a.ID, "x", true)
	st.RecordToolUse(a.ID, "x", true)
	st.RecordAdvertisedTools(a.ID, []string{"idle_tool"})

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents/"+a.ID+"/evolution", nil))
	if w.Code != 200 {
		t.Fatalf("get evo %d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Suggestions []team.Suggestion         `json:"suggestions"`
		Guardrails  store.EvolutionGuardrails `json:"guardrails"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Guardrails.AutoAdapt {
		t.Fatal("default auto_adapt must be false")
	}
	if listed.Guardrails.MinRuns != store.DefaultMinRuns {
		t.Fatalf("min_runs %d", listed.Guardrails.MinRuns)
	}
	for _, tok := range []string{"display_name", "agent_key"} {
		found := false
		for _, k := range listed.Guardrails.Locked {
			if k == tok {
				found = true
			}
		}
		if !found {
			t.Fatalf("locked missing %s %#v", tok, listed.Guardrails.Locked)
		}
	}
	for _, s := range listed.Suggestions {
		if team.ForbiddenWrite(s.Text) || team.ForbiddenWrite(s.ID) || team.ForbiddenWrite(s.Rule) {
			t.Fatalf("suggestion names protected field: %#v", s)
		}
	}

	w = postJSON(h, "/api/agents/"+a.ID+"/evolution/display_name/apply", `{"display_name":"Nope"}`)
	if w.Code != 400 {
		t.Fatalf("rename apply %d %s", w.Code, w.Body.String())
	}
	w = postJSON(h, "/api/agents/"+a.ID+"/evolution/"+team.RuleHighToolError+"/apply", `{}`)
	if w.Code != 200 {
		t.Fatalf("apply %d %s", w.Code, w.Body.String())
	}
	var got store.Agent
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.DisplayName != "Evo" || got.AgentKey != "evo" {
		t.Fatalf("identity %#v", got)
	}
	if !strings.Contains(got.Instructions, team.PrefixHighError) {
		t.Fatalf("prefix %q", got.Instructions)
	}
}

func TestEvolutionAPI_TickAndRollback(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	t.Setenv("GOSO_EVOLUTION_AUTO", "")
	st, h := newTestServer()
	w := postJSON(h, "/api/agents", `{"agent_key":"tick","display_name":"Tick","instructions":"base"}`)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var a store.Agent
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	for i := 0; i < 5; i++ {
		st.RecordChatRun(a.ID)
		st.RecordToolUse(a.ID, "x", true)
	}
	st.RecordAdvertisedTools(a.ID, []string{"idle_tool"})

	w = postJSON(h, "/api/agents/"+a.ID+"/evolution/tick", `{}`)
	if w.Code != 200 {
		t.Fatalf("tick off %d %s", w.Code, w.Body.String())
	}
	var tick team.TickResult
	_ = json.Unmarshal(w.Body.Bytes(), &tick)
	if tick.Action != "noop" || tick.Reason != "auto_adapt_off" {
		t.Fatalf("off %#v", tick)
	}
	if tick.Agent != nil && strings.Contains(tick.Agent.Instructions, team.PrefixHighError) {
		t.Fatalf("demo/off changed instructions %#v", tick.Agent)
	}

	w = patchJSON(h, "/api/agents/"+a.ID+"/evolution", `{"auto_adapt":true,"min_runs":5,"locked":[]}`)
	if w.Code != 200 {
		t.Fatalf("patch %d %s", w.Code, w.Body.String())
	}
	var patched struct {
		Guardrails store.EvolutionGuardrails `json:"guardrails"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if !patched.Guardrails.AutoAdapt || patched.Guardrails.MinRuns != 5 {
		t.Fatalf("patched %#v", patched.Guardrails)
	}
	for _, tok := range []string{"display_name", "agent_key"} {
		found := false
		for _, k := range patched.Guardrails.Locked {
			if k == tok {
				found = true
			}
		}
		if !found {
			t.Fatalf("cannot unlock %s %#v", tok, patched.Guardrails.Locked)
		}
	}

	w = postJSON(h, "/api/agents/"+a.ID+"/evolution/tick", `{}`)
	if w.Code != 200 {
		t.Fatalf("tick on %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tick)
	if tick.Action != "applied" {
		t.Fatalf("apply %#v", tick)
	}
	if tick.Agent == nil || !strings.Contains(tick.Agent.Instructions, team.PrefixHighError) {
		t.Fatalf("applied instructions %#v", tick.Agent)
	}
	if tick.Agent.DisplayName != "Tick" || tick.Agent.AgentKey != "tick" {
		t.Fatalf("identity %#v", tick.Agent)
	}

	st.RecordToolUse(a.ID, "x", true)
	st.RecordToolUse(a.ID, "x", true)
	w = postJSON(h, "/api/agents/"+a.ID+"/evolution/tick", `{}`)
	if w.Code != 200 {
		t.Fatalf("tick rb %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tick)
	if tick.Action != "rolled_back" {
		t.Fatalf("rollback %#v", tick)
	}
	if tick.Agent == nil || tick.Agent.Instructions != "base" {
		t.Fatalf("restored %#v", tick.Agent)
	}

	w = postJSON(h, "/api/agents/"+a.ID+"/evolution/display_name/apply", `{}`)
	if w.Code != 400 {
		t.Fatalf("name apply %d %s", w.Code, w.Body.String())
	}
	w = postJSON(h, "/api/agents/"+a.ID+"/evolution/agent_key/apply", `{}`)
	if w.Code != 400 {
		t.Fatalf("key apply %d %s", w.Code, w.Body.String())
	}
}

func TestChat_TeamToolsAndTEAMNote(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_DIR", root)
	if err := os.WriteFile(filepath.Join(root, "TEAM.md"), []byte("daily standup"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := store.New()
	lead, _ := st.CreateAgent(store.Agent{AgentKey: "lead", DisplayName: "Lead"})
	peer, _ := st.CreateAgent(store.Agent{AgentKey: "peer", DisplayName: "Peer"})
	_, _ = st.CreateTeam(store.Team{Name: "Ops", LeadAgentID: lead.ID})
	_, _ = st.AddTeamMember(store.TeamMember{TeamID: st.ListTeams()[0].ID, AgentID: peer.ID, Role: "member"})
	sess, _ := st.CreateSession(store.Session{AgentID: lead.ID})

	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "d1", Name: "delegate", Arguments: map[string]any{
			"to_agent_id": peer.ID, "message": "hi peer", "mode": "sync",
		}}}},
		{Text: "done"},
	}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(32), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"delegate please"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	if len(scripted.Recorded) == 0 {
		t.Fatal("scripted saw no turns")
	}
	sys := ""
	for _, m := range scripted.Recorded[0] {
		if m.Role == "system" {
			sys = m.Content
			break
		}
	}
	if !strings.Contains(sys, "Team: Ops") || !strings.Contains(sys, "daily standup") {
		t.Fatalf("system note %q", sys)
	}
	found := false
	for _, tools := range scripted.RecordedTools {
		for _, ts := range tools {
			if ts.Name == "delegate" || ts.Name == "spawn" || ts.Name == "team_tasks" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("auto mode tools missing %#v", scripted.RecordedTools)
	}
	if !strings.Contains(w.Body.String(), "done") {
		t.Fatalf("reply %s", w.Body.String())
	}
}

func TestChat_ManualModeHidesTeamTools(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "solo"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "echo: hi"}}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(8), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	for _, tools := range scripted.RecordedTools {
		for _, ts := range tools {
			if ts.Name == "delegate" || ts.Name == "spawn" || ts.Name == "team_tasks" {
				t.Fatalf("manual advertised %s", ts.Name)
			}
		}
	}
}
