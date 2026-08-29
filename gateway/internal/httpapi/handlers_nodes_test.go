// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/node"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func nodesServer(t *testing.T) (*node.Nodes, *eventstore.Store, http.Handler) {
	t.Helper()
	st := store.New()
	reg := node.New()
	ev := eventstore.New(64)
	h := NewRouter(Options{Store: st, Version: "t", Nodes: reg, Events: ev})
	return reg, ev, h
}

func TestNodes_ListEmptyAndV1(t *testing.T) {
	_, _, h := nodesServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/nodes", nil))
	if w.Code != 200 {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"pending":[]`) && !strings.Contains(body, `"pending": []`) {
		t.Fatalf("empty pending %s", body)
	}
	if !strings.Contains(body, `"paired":[]`) && !strings.Contains(body, `"paired": []`) {
		t.Fatalf("empty paired %s", body)
	}
	assertSameGET(t, h, "/api/nodes", "/v1/nodes")
}

func TestNodes_RequestApproveRevokeNoSecrets(t *testing.T) {
	_, ev, h := nodesServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/nodes/request", bytes.NewBufferString(`{}`)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "display is required") {
		t.Fatalf("empty display %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/request", bytes.NewBufferString(`{"display":"ops-laptop"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("request %d %s", w.Code, w.Body.String())
	}
	assertNoNodeSecrets(t, w.Body.String())
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	if id == "" || created["status"] != "pending" {
		t.Fatalf("created %#v", created)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/nodes", nil))
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	assertNoNodeSecrets(t, w.Body.String())
	var listed struct {
		Pending []map[string]any `json:"pending"`
		Paired  []map[string]any `json:"paired"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Pending) != 1 || len(listed.Paired) != 0 {
		t.Fatalf("list %#v", listed)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/nodes/"+id+"/approve", bytes.NewBufferString(`{}`)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirm is required") {
		t.Fatalf("empty confirm %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/nodes/"+id+"/approve", bytes.NewBufferString(`{"confirm":"nope"}`)))
	if w.Code != 400 {
		t.Fatalf("mismatch %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/nodes/"+id+"/approve", bytes.NewBufferString(`{"confirm":"`+id+`"}`)))
	if w.Code != 200 {
		t.Fatalf("approve %d %s", w.Code, w.Body.String())
	}
	assertNoNodeSecrets(t, w.Body.String())

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/nodes", nil))
	_ = json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Pending) != 0 || len(listed.Paired) != 1 {
		t.Fatalf("after approve %#v", listed)
	}
	if listed.Paired[0]["health"] != "ok" {
		t.Fatalf("health %#v", listed.Paired[0])
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/nodes/"+id+"/revoke", bytes.NewBufferString(`{"confirm":"ops-laptop"}`)))
	if w.Code != 200 {
		t.Fatalf("revoke %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/nodes", nil))
	_ = json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Pending) != 0 || len(listed.Paired) != 0 {
		t.Fatalf("after revoke %#v", listed)
	}

	events := ev.Filter("", "nodes", 10)
	if len(events) < 3 {
		t.Fatalf("audit %d", len(events))
	}
}

func TestNodes_DenyMissingAndV1Request(t *testing.T) {
	_, _, h := nodesServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/request", bytes.NewBufferString(`{"display":"phone"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("v1 request %d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	id, _ := created["id"].(string)

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/nodes/"+id+"/deny", bytes.NewBufferString(`{"confirm":"phone"}`)))
	if w.Code != 200 {
		t.Fatalf("deny %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/nodes/nope/approve", bytes.NewBufferString(`{"confirm":"nope"}`)))
	if w.Code != 404 {
		t.Fatalf("missing %d %s", w.Code, w.Body.String())
	}
}

func assertNoNodeSecrets(t *testing.T, body string) {
	t.Helper()
	for _, leak := range []string{`"token"`, `"code"`, `"secret"`, `"password"`, `"hmac"`, `"bot_token"`, `"content"`, `"text"`} {
		if strings.Contains(body, leak) {
			t.Fatalf("leaked %s in %s", leak, body)
		}
	}
}
