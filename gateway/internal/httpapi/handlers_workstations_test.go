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
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/workstation"
)

func workstationsServer(t *testing.T) (*workstation.Workstations, *eventstore.Store, http.Handler) {
	t.Helper()
	st := store.New()
	reg := workstation.New()
	ev := eventstore.New(64)
	h := NewRouter(Options{Store: st, Version: "t", Workstations: reg, Events: ev})
	return reg, ev, h
}

func TestWorkstations_ListEmptyAndV1(t *testing.T) {
	_, _, h := workstationsServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/workstations", nil))
	if w.Code != 200 {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"workstations":[]`) && !strings.Contains(body, `"workstations": []`) {
		t.Fatalf("empty %s", body)
	}
	assertSameGET(t, h, "/api/workstations", "/v1/workstations")
}

func TestWorkstations_CreateTestDeleteNoSecrets(t *testing.T) {
	_, ev, h := workstationsServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/workstations", bytes.NewBufferString(`{}`)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "display is required") {
		t.Fatalf("empty %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/workstations", bytes.NewBufferString(`{"display":"lab","backend":"ssh","host":"10.0.0.8","user":"ops","private_key":"AAAA"}`)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "path/ref") {
		t.Fatalf("private_key %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/workstations", bytes.NewBufferString(`{"display":"lab","backend":"ssh","host":"10.0.0.8","user":"ops","identity_ref":"-----BEGIN OPENSSH PRIVATE KEY-----"}`)))
	if w.Code != 400 {
		t.Fatalf("pem identity %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/workstations", bytes.NewBufferString(`{"display":"lab","backend":"ssh","host":"10.0.0.8","user":"ops","identity_ref":"~/.ssh/id_ed25519","agent_id":"ag_1"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	assertNoWorkstationSecrets(t, w.Body.String())
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	if id == "" || created["backend"] != "ssh" || created["identity_set"] != true {
		t.Fatalf("created %#v", created)
	}
	if created["identity_ref"] != "~/.ssh/id_ed25519" {
		t.Fatalf("path %#v", created["identity_ref"])
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/workstations", nil))
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	assertNoWorkstationSecrets(t, w.Body.String())

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/workstations/"+id, nil))
	if w.Code != 200 {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}
	assertNoWorkstationSecrets(t, w.Body.String())

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/workstations/"+id+"/test", nil))
	if w.Code != 200 {
		t.Fatalf("test %d %s", w.Code, w.Body.String())
	}
	assertNoWorkstationSecrets(t, w.Body.String())
	if strings.Contains(w.Body.String(), `"identity_ref"`) || strings.Contains(w.Body.String(), "~/.ssh") {
		t.Fatalf("test output leaked path %s", w.Body.String())
	}
	var tr map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &tr); err != nil {
		t.Fatal(err)
	}
	if tr["ok"] != true || tr["health"] != "ok" || tr["identity_set"] != true {
		t.Fatalf("test %#v", tr)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/workstations/"+id+"/disconnect", bytes.NewBufferString(`{}`)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirm is required") {
		t.Fatalf("empty confirm %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/workstations/"+id+"/disconnect", bytes.NewBufferString(`{"confirm":"nope"}`)))
	if w.Code != 400 {
		t.Fatalf("mismatch %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/workstations/"+id+"/disconnect", bytes.NewBufferString(`{"confirm":"lab"}`)))
	if w.Code != 200 {
		t.Fatalf("disconnect %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/workstations/"+id+"/delete", bytes.NewBufferString(`{"confirm":"`+id+`"}`)))
	if w.Code != 200 {
		t.Fatalf("delete %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/workstations/"+id, nil))
	if w.Code != 404 {
		t.Fatalf("gone %d %s", w.Code, w.Body.String())
	}

	events := ev.Filter("", "workstations", 10)
	if len(events) < 4 {
		t.Fatalf("audit %d", len(events))
	}
}

func TestWorkstations_PatchMissingAndV1(t *testing.T) {
	_, _, h := workstationsServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/workstations", bytes.NewBufferString(`{"display":"dock","backend":"docker","host":"127.0.0.1"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("v1 create %d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	id, _ := created["id"].(string)
	if created["port"] != float64(2375) {
		t.Fatalf("docker port %#v", created["port"])
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/api/workstations/"+id, bytes.NewBufferString(`{"display":"dock-2","agent_id":"ag_9"}`)))
	if w.Code != 200 {
		t.Fatalf("patch %d %s", w.Code, w.Body.String())
	}
	var patched map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if patched["display"] != "dock-2" || patched["agent_id"] != "ag_9" {
		t.Fatalf("patched %#v", patched)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/workstations/nope/delete", bytes.NewBufferString(`{"confirm":"nope"}`)))
	if w.Code != 404 {
		t.Fatalf("missing %d %s", w.Code, w.Body.String())
	}
}

func assertNoWorkstationSecrets(t *testing.T, body string) {
	t.Helper()
	for _, leak := range []string{`"private_key"`, `"password"`, `"secret"`, `"token"`, `"ssh_key"`, `"pem"`, `"api_key"`} {
		if strings.Contains(body, leak) {
			t.Fatalf("leaked %s in %s", leak, body)
		}
	}
}
