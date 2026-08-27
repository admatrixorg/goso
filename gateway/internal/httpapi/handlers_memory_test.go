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
	if w.Code != 400 {
		t.Fatalf("missing session %d", w.Code)
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
