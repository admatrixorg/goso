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

func contactsServer(t *testing.T) (*store.Store, *channel.Contacts, *eventstore.Store, http.Handler) {
	t.Helper()
	t.Setenv("GOSO_LITE", "")
	st := store.New()
	dir := channel.NewContacts()
	ev := eventstore.New(64)
	h := NewRouter(Options{Store: st, Version: "t", Contacts: dir, Events: ev})
	return st, dir, ev, h
}

func TestContacts_ListEmptyAndV1(t *testing.T) {
	_, _, _, h := contactsServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/contacts", nil))
	if w.Code != 200 {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"contacts":[]`)) && !bytes.Contains(w.Body.Bytes(), []byte(`"contacts": []`)) {
		t.Fatalf("empty %s", w.Body.String())
	}
	assertSameGET(t, h, "/api/contacts", "/v1/contacts")
}

func TestContacts_ListOmitsSecretsAndNamesAgent(t *testing.T) {
	st, dir, _, h := contactsServer(t)
	a, err := st.CreateAgent(store.Agent{AgentKey: "tg", DisplayName: "Support"})
	if err != nil {
		t.Fatal(err)
	}
	dir.Observe(channel.Sighting{Channel: "telegram", Dest: "-1001", Kind: "group", AgentID: a.ID})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/contacts", nil))
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
		Contacts []map[string]any `json:"contacts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Contacts) != 1 {
		t.Fatalf("contacts %#v", parsed.Contacts)
	}
	row := parsed.Contacts[0]
	if row["channel"] != "telegram" || row["dest"] != "-1001" || row["kind"] != "group" {
		t.Fatalf("row %#v", row)
	}
	if row["agent"] != "Support" {
		t.Fatalf("agent %#v", row)
	}
	if row["permission"] != "group" {
		t.Fatalf("permission %#v", row)
	}
}

func TestContacts_MergeAndUndoConfirm(t *testing.T) {
	_, dir, ev, h := contactsServer(t)
	a := dir.Observe(channel.Sighting{Channel: "telegram", Dest: "111", Kind: "user"})
	b := dir.Observe(channel.Sighting{Channel: "discord", Dest: "222", Kind: "user"})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/contacts/"+a.ID+"/merge", bytes.NewBufferString(`{"source_id":"`+b.ID+`"}`)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirm is required") {
		t.Fatalf("empty confirm %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/contacts/"+a.ID+"/merge", bytes.NewBufferString(`{"source_id":"`+b.ID+`","confirm":"nope"}`)))
	if w.Code != 400 {
		t.Fatalf("mismatch %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/contacts/"+a.ID+"/merge", bytes.NewBufferString(`{"source_id":"`+b.ID+`","confirm":"`+b.ID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("merge %d %s", w.Code, w.Body.String())
	}
	var merged map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &merged)
	idents, _ := merged["identifiers"].([]any)
	if len(idents) != 2 {
		t.Fatalf("merged idents %#v", merged)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/contacts/"+b.ID, nil))
	if w.Code != 404 {
		t.Fatalf("merged source GET %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/contacts/"+a.ID+"/undo", bytes.NewBufferString(`{"confirm":"`+b.ID+`"}`)))
	if w.Code != 200 {
		t.Fatalf("undo %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/contacts", nil))
	var listed struct {
		Contacts []map[string]any `json:"contacts"`
		Total    int              `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listed)
	if listed.Total != 2 {
		t.Fatalf("after undo %s", w.Body.String())
	}
	events := ev.Filter("", "contacts", 10)
	if len(events) < 2 {
		t.Fatalf("audit %d", len(events))
	}
}

func TestContacts_LiteForbidden(t *testing.T) {
	t.Setenv("GOSO_LITE", "1")
	st := store.New()
	dir := channel.NewContacts()
	h := NewRouter(Options{Store: st, Version: "t", Contacts: dir})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/contacts", nil))
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "lite: channels off") {
		t.Fatalf("lite GET %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/contacts/x/merge", bytes.NewBufferString(`{"source_id":"y","confirm":"x"}`)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("lite merge %d %s", w.Code, w.Body.String())
	}
}

func TestContacts_MissingAndFilter(t *testing.T) {
	_, dir, _, h := contactsServer(t)
	dir.Observe(channel.Sighting{Channel: "slack", Dest: "d1", Kind: "user"})
	dir.Observe(channel.Sighting{Channel: "telegram", Dest: "-2", Kind: "group"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/contacts/nope", nil))
	if w.Code != 404 {
		t.Fatalf("missing %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/contacts?channel=slack", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"dest":"d1"`) || strings.Contains(w.Body.String(), `"-2"`) {
		t.Fatalf("filter %s", w.Body.String())
	}
}
