// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func pendingServer(t *testing.T) (*store.Store, *channel.Pending, *eventstore.Store, http.Handler) {
	t.Helper()
	t.Setenv("GOSO_LITE", "")
	st := store.New()
	buf := channel.NewPending()
	ev := eventstore.New(64)
	h := NewRouter(Options{Store: st, Version: "t", Pending: buf, Events: ev})
	return st, buf, ev, h
}

func TestPendingMessages_ListEmptyAndV1(t *testing.T) {
	_, _, _, h := pendingServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/pending-messages", nil))
	if w.Code != 200 {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"groups":[]`)) && !bytes.Contains(w.Body.Bytes(), []byte(`"groups": []`)) {
		t.Fatalf("empty %s", w.Body.String())
	}
	assertSameGET(t, h, "/api/pending-messages", "/v1/pending-messages")
}

func TestPendingMessages_ListOmitsSecretsAndNamesAgent(t *testing.T) {
	st, buf, _, h := pendingServer(t)
	a, err := st.CreateAgent(store.Agent{AgentKey: "tg", DisplayName: "Support"})
	if err != nil {
		t.Fatal(err)
	}
	buf.Enqueue(channel.Enqueue{Channel: "telegram", Dest: "-1001", AgentID: a.ID})
	buf.Enqueue(channel.Enqueue{Channel: "telegram", Dest: "-1001", AgentID: a.ID})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/pending-messages", nil))
	if w.Code != 200 {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, leak := range []string{`"token"`, `"code"`, `"secret"`, `"content"`, `"text"`, `"bot_token"`, `"hmac"`} {
		if strings.Contains(body, leak) {
			t.Fatalf("GET leaked %s in %s", leak, body)
		}
	}
	var parsed struct {
		Groups []map[string]any `json:"groups"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Groups) != 1 {
		t.Fatalf("groups %#v", parsed.Groups)
	}
	g := parsed.Groups[0]
	if g["channel"] != "telegram" || g["dest"] != "-1001" || g["count"] != float64(2) {
		t.Fatalf("row %#v", g)
	}
	if g["agent"] != "Support" {
		t.Fatalf("agent %#v", g)
	}
}

func TestPendingMessages_CompactAndClearConfirm(t *testing.T) {
	st, buf, ev, h := pendingServer(t)
	a, err := st.CreateAgent(store.Agent{AgentKey: "dc", DisplayName: "Desk"})
	if err != nil {
		t.Fatal(err)
	}
	g := buf.Enqueue(channel.Enqueue{Channel: "discord", Dest: "room-1", AgentID: a.ID})
	buf.Enqueue(channel.Enqueue{Channel: "discord", Dest: "room-1", AgentID: a.ID})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/pending-messages/"+g.ID+"/compact", bytes.NewBufferString(`{}`)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirm is required") {
		t.Fatalf("empty confirm %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/pending-messages/"+g.ID+"/compact", bytes.NewBufferString(`{"confirm":"nope"}`)))
	if w.Code != 400 {
		t.Fatalf("mismatch %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/pending-messages/"+g.ID+"/compact", bytes.NewBufferString(`{"confirm":"discord:room-1"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("compact %d %s", w.Code, w.Body.String())
	}
	var compacted map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &compacted)
	if compacted["compacted"] != true || compacted["count"] != float64(1) {
		t.Fatalf("compacted %#v", compacted)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/pending-messages/"+g.ID+"/clear", bytes.NewBufferString(`{"confirm":"room-1"}`)))
	if w.Code != 200 {
		t.Fatalf("clear %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/pending-messages", nil))
	if !bytes.Contains(w.Body.Bytes(), []byte(`"groups":[]`)) && !bytes.Contains(w.Body.Bytes(), []byte(`"groups": []`)) {
		t.Fatalf("after clear %s", w.Body.String())
	}
	events := ev.Filter("", "pending-messages", 10)
	if len(events) < 2 {
		t.Fatalf("audit %d", len(events))
	}
}

func TestPendingMessages_LiteForbidden(t *testing.T) {
	t.Setenv("GOSO_LITE", "1")
	st := store.New()
	buf := channel.NewPending()
	h := NewRouter(Options{Store: st, Version: "t", Pending: buf})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/pending-messages", nil))
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "lite: channels off") {
		t.Fatalf("lite GET %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/pending-messages/x/compact", bytes.NewBufferString(`{"confirm":"x"}`)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("lite compact %d %s", w.Code, w.Body.String())
	}
}

func TestPendingMessages_MissingAndBusy(t *testing.T) {
	_, buf, _, h := pendingServer(t)
	g := buf.Enqueue(channel.Enqueue{Channel: "slack", Dest: "d1"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/pending-messages/nope/clear", bytes.NewBufferString(`{"confirm":"d1"}`)))
	if w.Code != 404 {
		t.Fatalf("missing %d %s", w.Code, w.Body.String())
	}
	buf.HookBusy()
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/pending-messages/"+g.ID+"/compact", bytes.NewBufferString(`{"confirm":"d1"}`)))
	if w.Code != http.StatusConflict {
		t.Fatalf("busy %d %s", w.Code, w.Body.String())
	}
}
