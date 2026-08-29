// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestMemoryAPI_CRUDAndSearch(t *testing.T) {
	st, h := newTestServer()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "m1", DisplayName: "M"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/memory", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","body":"episodic banana note"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("POST memory %d %s", w.Code, w.Body.String())
	}
	var created store.Memory
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID == "" || created.Kind != store.KindEpisodic {
		t.Fatalf("created %v %s", err, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/memory?session_id="+sess.ID, nil))
	if w.Code != 200 {
		t.Fatalf("GET memory %d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Memories []*store.Memory `json:"memories"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Memories) != 1 {
		t.Fatalf("list %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/memory/search?q=banana", nil))
	if w.Code != 200 {
		t.Fatalf("search %d %s", w.Code, w.Body.String())
	}
	var hits []store.SearchHit
	if err := json.Unmarshal(w.Body.Bytes(), &hits); err != nil || len(hits) == 0 {
		t.Fatalf("hits %v %s", err, w.Body.String())
	}
	if hits[0].Snippet == "" || hits[0].SessionID != sess.ID {
		t.Fatalf("hit %#v", hits[0])
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/memory/search?q=no-such-token-xyz", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("empty search %d %q", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/memory/search", nil))
	if w.Code != 400 {
		t.Fatalf("empty q %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/memory", nil))
	if w.Code != 200 {
		t.Fatalf("list all %d %s", w.Code, w.Body.String())
	}
	var all struct {
		Memories []struct {
			Body    string `json:"body"`
			Snippet string `json:"snippet"`
		} `json:"memories"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &all); err != nil || len(all.Memories) == 0 {
		t.Fatalf("list all json %v %s", err, w.Body.String())
	}
	if all.Memories[0].Body != "" || all.Memories[0].Snippet == "" {
		t.Fatalf("list must snippet without body %#v", all.Memories[0])
	}
}

func TestMemoryAPI_OperatorFiltersCRUDIndex(t *testing.T) {
	st, h := newTestServer()
	a1, _ := st.CreateAgent(store.Agent{AgentKey: "op-a", DisplayName: "Alpha"})
	a2, _ := st.CreateAgent(store.Agent{AgentKey: "op-b", DisplayName: "Beta"})
	s1, _ := st.CreateSession(store.Session{AgentID: a1.ID, Label: "s1"})
	s2, _ := st.CreateSession(store.Session{AgentID: a2.ID, Label: "s2"})

	post := func(sid, body, kind string) store.Memory {
		t.Helper()
		payload := `{"session_id":"` + sid + `","body":"` + body + `"`
		if kind != "" {
			payload += `,"kind":"` + kind + `"`
		}
		payload += `}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/memory", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, req)
		if w.Code != 201 {
			t.Fatalf("POST %d %s", w.Code, w.Body.String())
		}
		var m store.Memory
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil || m.ID == "" {
			t.Fatalf("created %v %s", err, w.Body.String())
		}
		return m
	}

	epi := post(s1.ID, "session note banana", "")
	dur := post(s1.ID, "durable playbook", "document")
	other := post(s2.ID, "other agent note", "durable")
	if dur.Kind != store.KindDurable {
		t.Fatalf("document alias %q", dur.Kind)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/memory", bytes.NewBufferString(`{"session_id":"`+s1.ID+`","body":"x","kind":"message"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("reserved kind %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/memory?agent_id="+a1.ID, nil))
	if w.Code != 200 {
		t.Fatalf("agent filter %d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Memories []struct {
			ID      string `json:"id"`
			Kind    string `json:"kind"`
			Snippet string `json:"snippet"`
			Body    string `json:"body"`
			AgentID string `json:"agent_id"`
		} `json:"memories"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("list json %v %s", err, w.Body.String())
	}
	if len(listed.Memories) != 2 {
		t.Fatalf("agent list %s", w.Body.String())
	}
	for _, row := range listed.Memories {
		if row.Body != "" {
			t.Fatalf("list body leaked %#v", row)
		}
		if row.Snippet == "" || row.AgentID != a1.ID {
			t.Fatalf("list row %#v", row)
		}
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/memory?kind=durable&session_id="+s1.ID, nil))
	_ = json.Unmarshal(w.Body.Bytes(), &listed)
	if w.Code != 200 || len(listed.Memories) != 1 || listed.Memories[0].ID != dur.ID {
		t.Fatalf("kind filter %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/memory/"+dur.ID, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "durable playbook") {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/api/memory/"+dur.ID, bytes.NewBufferString(`{"body":"durable playbook v2"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "v2") {
		t.Fatalf("patch %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/memory/"+other.ID, nil))
	if w.Code != 200 {
		t.Fatalf("delete %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/memory/"+other.ID, nil))
	if w.Code != 404 {
		t.Fatalf("deleted get %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/memory/index", nil))
	if w.Code != 200 {
		t.Fatalf("index %d %s", w.Code, w.Body.String())
	}
	var idx struct {
		Search              string `json:"search"`
		FTS                 bool   `json:"fts"`
		Embedding           string `json:"embedding"`
		EmbeddingConfigured bool   `json:"embedding_configured"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &idx); err != nil {
		t.Fatalf("index json %v %s", err, w.Body.String())
	}
	if idx.Embedding != "not_configured" || idx.EmbeddingConfigured {
		t.Fatalf("must not invent embedder %#v", idx)
	}
	if idx.Search != "substring" && idx.Search != "fts5" {
		t.Fatalf("search mode %q", idx.Search)
	}

	_ = epi
}

func TestMemoryAPI_IndexV1Alias(t *testing.T) {
	_, h := newTestServer()
	wAPI := httptest.NewRecorder()
	h.ServeHTTP(wAPI, httptest.NewRequest("GET", "/api/memory/index", nil))
	wV1 := httptest.NewRecorder()
	h.ServeHTTP(wV1, httptest.NewRequest("GET", "/v1/memory/index", nil))
	if wAPI.Code != 200 || wV1.Code != 200 || wAPI.Body.String() != wV1.Body.String() {
		t.Fatalf("index alias api=%d %s v1=%d %s", wAPI.Code, wAPI.Body.String(), wV1.Code, wV1.Body.String())
	}
}

func TestMemoryAPI_SummarizeFlag(t *testing.T) {
	st, h := newTestServer()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "m2", DisplayName: "M"})
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat?summarize=1", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"flag-hello"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat %d %s", w.Code, w.Body.String())
	}
	sum, err := st.LatestSummary(sess.ID)
	if err != nil || sum == nil || !strings.Contains(sum.Body, "flag-hello") {
		t.Fatalf("summary %v %#v", err, sum)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"again","summarize":1}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat json flag %d %s", w.Code, w.Body.String())
	}
}

func TestMemoryAPI_BearerAuth(t *testing.T) {
	st := store.New()
	mux := Router(st, "0.1.0")
	h := auth.RequireToken("secret", []string{"/healthz"})(mux)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/memory?session_id=x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/memory/search?q=hi", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("search unauth %d", w.Code)
	}

	req := httptest.NewRequest("GET", "/api/memory/search?q=hi", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("auth search %d %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("auth empty %q", w.Body.String())
	}
}
