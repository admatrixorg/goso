// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestRejectInvalidTenant(t *testing.T) {
	t.Setenv("GOSO_MULTI_TENANT", "1")
	st := store.New()
	h := RejectInvalidTenant(NewRouter(Options{Store: st, Version: "t"}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Goso-Tenant", "bad tenant!")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("invalid tenant id must be 400, got %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Goso-Tenant", "acme")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("valid tenant id must pass, got %d", w.Code)
	}
	// Multi-tenant off → header ignored, never 400.
	t.Setenv("GOSO_MULTI_TENANT", "")
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Goso-Tenant", "bad tenant!")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("single-tenant mode must ignore header, got %d", w.Code)
	}
}

func TestKGAPI_SupersedeRoutes(t *testing.T) {
	_, h := newTestServer()
	post := func(path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, req)
		return w
	}
	w := post("/api/kg/entities", `{"name":"Alpha","kind":"person"}`)
	var an store.KGEntity
	_ = json.Unmarshal(w.Body.Bytes(), &an)
	w = post("/api/kg/entities", `{"name":"Acme","kind":"org"}`)
	var acme store.KGEntity
	_ = json.Unmarshal(w.Body.Bytes(), &acme)
	w = post("/api/kg/entities", `{"name":"Zeta","kind":"org"}`)
	var zeta store.KGEntity
	_ = json.Unmarshal(w.Body.Bytes(), &zeta)

	w = post("/api/kg/relations", `{"from_id":"`+an.ID+`","to_id":"`+acme.ID+`","rel":"works_at"}`)
	if w.Code != 201 {
		t.Fatalf("rel1 %d %s", w.Code, w.Body.String())
	}
	var r1 store.KGRelation
	_ = json.Unmarshal(w.Body.Bytes(), &r1)
	w = post("/api/kg/relations", `{"from_id":"`+an.ID+`","to_id":"`+zeta.ID+`","rel":"works_at","exclusive":true}`)
	if w.Code != 201 {
		t.Fatalf("rel2 %d %s", w.Code, w.Body.String())
	}
	var r2 store.KGRelation
	_ = json.Unmarshal(w.Body.Bytes(), &r2)

	w = post("/api/kg/relations/"+r1.ID+"/supersede", ``)
	if w.Code != 200 {
		t.Fatalf("supersede already-closed relation should still be 200, got %d %s", w.Code, w.Body.String())
	}
	w = post("/api/kg/relations/nope/supersede", `{}`)
	if w.Code != 404 {
		t.Fatalf("unknown relation %d", w.Code)
	}
	w = post("/api/kg/entities/"+acme.ID+"/supersede", `{"valid_until":"not-a-date"}`)
	if w.Code != 400 {
		t.Fatalf("bad valid_until %d", w.Code)
	}
	w = post("/api/kg/entities/"+acme.ID+"/supersede", `{"valid_until":"2020-01-01T00:00:00Z"}`)
	if w.Code != 200 {
		t.Fatalf("supersede entity %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/kg/entities/"+acme.ID, nil)
	h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"valid_until":"2020-01-01T00:00:00Z"`) {
		t.Fatalf("superseded entity must expose its closed window: %d %s", w.Code, w.Body.String())
	}
}
