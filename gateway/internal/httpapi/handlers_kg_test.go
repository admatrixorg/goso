// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
)

func TestKGAPI_EntitiesSearchExpand(t *testing.T) {
	st, h := newTestServer()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/kg/entities", bytes.NewBufferString(`{"name":"Acme Billing","kind":"org","body":"invoices"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("POST entity %d %s", w.Code, w.Body.String())
	}
	var acme store.KGEntity
	if err := json.Unmarshal(w.Body.Bytes(), &acme); err != nil || acme.ID == "" || acme.TenantID != store.DefaultTenant {
		t.Fatalf("created %v %#v", err, acme)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/kg/entities", bytes.NewBufferString(`{"name":"Zeta Warehouse","kind":"place","body":"stock"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("POST other %d %s", w.Code, w.Body.String())
	}
	var zeta store.KGEntity
	_ = json.Unmarshal(w.Body.Bytes(), &zeta)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/kg/relations", bytes.NewBufferString(`{"from_id":"`+acme.ID+`","to_id":"`+zeta.ID+`","rel":"ships_to"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("POST rel %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/kg/search?q=Acme", nil))
	if w.Code != 200 {
		t.Fatalf("search %d %s", w.Code, w.Body.String())
	}
	var hits []store.KGSearchHit
	if err := json.Unmarshal(w.Body.Bytes(), &hits); err != nil || len(hits) == 0 {
		t.Fatalf("hits %v %s", err, w.Body.String())
	}
	if hits[0].ID != acme.ID || hits[0].Tier != store.TierL2 || hits[0].Snippet == "" {
		t.Fatalf("named hit %#v", hits[0])
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/kg/search?q=no-such-token-xyz", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("empty search %d %q", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/kg/search", nil))
	if w.Code != 400 {
		t.Fatalf("empty q %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/kg/entities/"+acme.ID, nil))
	if w.Code != 200 {
		t.Fatalf("expand %d %s", w.Code, w.Body.String())
	}
	var exp store.KGExpand
	if err := json.Unmarshal(w.Body.Bytes(), &exp); err != nil || exp.Entity == nil || len(exp.Relations) != 1 {
		t.Fatalf("expand body %v %s", err, w.Body.String())
	}
	if exp.Relations[0].ToName != "Zeta Warehouse" || exp.Relations[0].Rel != "ships_to" {
		t.Fatalf("linked %#v", exp.Relations[0])
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/kg/search?q=Acme", nil))
	if w.Code != 200 {
		t.Fatalf("v1 alias %d %s", w.Code, w.Body.String())
	}
	_ = st
}

func TestKGAPI_SQLiteFTS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kg-http.db")
	st, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h := Router(st, "0.1.0")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/kg/entities", bytes.NewBufferString(`{"name":"Pineapple Co","body":"fruit"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("POST %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/kg/entities", bytes.NewBufferString(`{"name":"Mango LLC","body":"unrelated"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("POST other %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/kg/search?q=Pineapple", nil))
	if w.Code != 200 {
		t.Fatalf("search %d %s", w.Code, w.Body.String())
	}
	var hits []store.KGSearchHit
	_ = json.Unmarshal(w.Body.Bytes(), &hits)
	if len(hits) != 1 || hits[0].Tier != store.TierL2 || hits[0].Name != "Pineapple Co" {
		t.Fatalf("hits %#v", hits)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/kg/search?q=zzzz-absent", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("empty %d %q", w.Code, w.Body.String())
	}
}

func TestKGAPI_BearerAuth(t *testing.T) {
	mux := Router(store.New(), "0.1.0")
	h := auth.RequireToken("secret", []string{"/healthz"})(mux)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/kg/search?q=hi", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth %d", w.Code)
	}
	req := httptest.NewRequest("GET", "/api/kg/search?q=hi", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("auth %d %s", w.Code, w.Body.String())
	}
}

func TestKGAPI_TenantIsolation(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-072")
	st := store.New()
	h := auth.RequireToken("admin-072", []string{"/healthz"})(NewRouter(Options{Store: st, Version: "t"}))

	w := tenantDo(h, "POST", "/api/kg/entities", `{"name":"Acme Only"}`, "admin-072", "acme")
	if w.Code != 201 {
		t.Fatalf("create acme %d %s", w.Code, w.Body.String())
	}
	var acme store.KGEntity
	_ = json.Unmarshal(w.Body.Bytes(), &acme)

	w = tenantDo(h, "GET", "/api/kg/search?q=Acme", "", "admin-072", "other")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("other tenant search %d %s", w.Code, w.Body.String())
	}
	w = tenantDo(h, "GET", "/api/kg/entities/"+acme.ID, "", "admin-072", "other")
	if w.Code != 404 {
		t.Fatalf("other expand %d %s", w.Code, w.Body.String())
	}
	w = tenantDo(h, "GET", "/api/kg/search?q=Acme", "", "admin-072", "acme")
	if w.Code != 200 || !strings.Contains(w.Body.String(), acme.ID) {
		t.Fatalf("acme search %d %s", w.Code, w.Body.String())
	}
}

func TestChat_MemorySearchExpandTools(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "memtool"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	e, err := st.PutKGEntity(store.KGEntity{Name: "Acme Billing", Kind: "org", Body: "invoices"})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := st.PutKGEntity(store.KGEntity{Name: "Zeta", Kind: "org"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = st.PutKGRelation(store.KGRelation{FromID: e.ID, ToID: peer.ID, Rel: "knows"})

	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "s1", Name: "memory_search", Arguments: map[string]any{"query": "Acme"}}}},
		{ToolCalls: []llm.ToolCall{{ID: "e1", Name: "memory_expand", Arguments: map[string]any{"id": e.ID}}}},
		{Text: "found billing"},
	}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(32), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"find acme"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	foundSearch, foundExpand := false, false
	for _, tools := range scripted.RecordedTools {
		for _, ts := range tools {
			if ts.Name == "memory_search" {
				foundSearch = true
			}
			if ts.Name == "memory_expand" {
				foundExpand = true
			}
		}
	}
	if !foundSearch || !foundExpand {
		t.Fatalf("tools missing %#v", scripted.RecordedTools)
	}
	if !strings.Contains(w.Body.String(), "found billing") {
		t.Fatalf("reply %s", w.Body.String())
	}
	msgs, _ := st.ListMessages(sess.ID)
	sawHits, sawExpand := false, false
	for _, m := range msgs {
		if m == nil || m.Role != "tool" {
			continue
		}
		if strings.Contains(m.Content, e.ID) && strings.Contains(m.Content, "l2") {
			sawHits = true
		}
		if strings.Contains(m.Content, "knows") {
			sawExpand = true
		}
	}
	if !sawHits || !sawExpand {
		t.Fatalf("tool results hits=%v expand=%v msgs=%#v", sawHits, sawExpand, msgs)
	}
}

func TestChat_MemorySearchEmptyQueryFailClosed(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "emptyq"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	scripted := &llm.Scripted{Replies: []llm.Reply{
		{ToolCalls: []llm.ToolCall{{ID: "s1", Name: "memory_search", Arguments: map[string]any{"query": "  "}}}},
		{Text: "no search"},
	}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(8), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"search"}`))
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
		if strings.Contains(m.Content, "q is required") {
			sawErr = true
		}
		if strings.Contains(m.Content, `"tier"`) && strings.Contains(m.Content, `"l1"`) {
			t.Fatalf("empty query must not search %#v", m)
		}
	}
	if !sawErr {
		t.Fatalf("expected fail-closed error, msgs=%#v", msgs)
	}
}

func TestChat_KGExtractDefaultOff(t *testing.T) {
	t.Setenv("GOSO_KG_EXTRACT", "")
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "extractoff"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "Name: ShouldNotInsert"}}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(8), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	hits, err := st.SearchProgressive("ShouldNotInsert", store.DefaultTenant)
	if err != nil {
		t.Fatal(err)
	}
	if hasTier(hits, store.TierL2) {
		t.Fatalf("extract default off inserted L2 %#v", hits)
	}
}

func TestChat_KGExtractOn(t *testing.T) {
	t.Setenv("GOSO_KG_EXTRACT", "1")
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "extracton"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "Entity: Extracted Co\nok"}}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(8), scripted)
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted, Runtime: rt})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	hits, err := st.SearchProgressive("Extracted", store.DefaultTenant)
	if err != nil || !hasTier(hits, store.TierL2) {
		t.Fatalf("extract on %v %#v", err, hits)
	}
}

func TestKGAPI_GraphRequiresAgentAndNeverLeaksSecrets(t *testing.T) {
	st, h := newTestServer()
	a, err := st.CreateAgent(store.Agent{AgentKey: "kg-ui"})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/kg/graph", nil))
	if w.Code != 400 {
		t.Fatalf("missing agent %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/kg/graph?agent_id=missing", nil))
	if w.Code != 404 {
		t.Fatalf("unknown agent %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/kg/entities", bytes.NewBufferString(`{"name":"Acme","kind":"org","body":"sk-live-abcdefghijk","agent_id":"`+a.ID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("POST entity %d %s", w.Code, w.Body.String())
	}
	var acme store.KGEntity
	if err := json.Unmarshal(w.Body.Bytes(), &acme); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/kg/entities", bytes.NewBufferString(`{"name":"Zeta","kind":"place","agent_id":"`+a.ID+`","source":"extracted"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("POST extracted %d %s", w.Code, w.Body.String())
	}
	var zeta store.KGEntity
	_ = json.Unmarshal(w.Body.Bytes(), &zeta)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/kg/relations", bytes.NewBufferString(`{"from_id":"`+acme.ID+`","to_id":"`+zeta.ID+`","rel":"ships_to","source":"extracted"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("POST rel %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/kg/graph?agent_id="+a.ID+"&limit=40", nil))
	if w.Code != 200 {
		t.Fatalf("graph %d %s", w.Code, w.Body.String())
	}
	raw := w.Body.String()
	for _, leak := range []string{`"token"`, `"secret"`, `"api_key"`, `"bot_token"`, `"private_key"`, "sk-live-"} {
		if strings.Contains(raw, leak) {
			t.Fatalf("GET leaked %s: %s", leak, raw)
		}
	}
	var g struct {
		Nodes               []store.KGGraphNode `json:"nodes"`
		Edges               []store.KGGraphEdge `json:"edges"`
		Truncated           bool                `json:"truncated"`
		NodeCap             int                 `json:"node_cap"`
		InferredAreNotFacts bool                `json:"inferred_are_not_facts"`
		Embedding           string              `json:"embedding"`
		EmbeddingConfigured bool                `json:"embedding_configured"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	if !g.InferredAreNotFacts || g.Embedding != "not_configured" || g.EmbeddingConfigured {
		t.Fatalf("health %#v", g)
	}
	if len(g.Nodes) != 2 || len(g.Edges) != 1 || g.NodeCap != 40 {
		t.Fatalf("graph size %#v", g)
	}
	if !g.Edges[0].Inferred {
		t.Fatalf("edge should be inferred %#v", g.Edges[0])
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/kg/graph?agent_id="+a.ID+"&limit=1", nil))
	if w.Code != 200 {
		t.Fatalf("cap %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &g); err != nil || !g.Truncated || len(g.Nodes) != 1 {
		t.Fatalf("cap body %v %#v", err, g)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/kg/graph?agent_id="+a.ID, nil))
	if w.Code != 200 {
		t.Fatalf("v1 alias %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/kg/index", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"embedding":"not_configured"`) {
		t.Fatalf("index %d %s", w.Code, w.Body.String())
	}
}

func hasTier(hits []store.KGSearchHit, tier string) bool {
	for _, h := range hits {
		if h.Tier == tier {
			return true
		}
	}
	return false
}
