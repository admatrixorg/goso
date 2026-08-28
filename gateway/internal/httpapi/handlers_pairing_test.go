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

func pairingServer(t *testing.T, view string) (http.Handler, *auth.Pairing) {
	t.Helper()
	t.Setenv("GOSO_VIEW_TOKEN", view)
	p := auth.NewPairing()
	inner := NewRouter(Options{Store: store.New(), Version: "t", Pairing: p})
	h := auth.Require(auth.Config{
		Admin:   "admin-077",
		View:    view,
		Pairing: p,
		Bypass:  []string{"/healthz"},
	})(inner)
	return h, p
}

func TestPairing_HTTPExchangeOnce(t *testing.T) {
	h, _ := pairingServer(t, "view-077")

	req := httptest.NewRequest("POST", "/api/pairing", nil)
	req.Header.Set("Authorization", "Bearer admin-077")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created struct {
		Code       string `json:"code"`
		TTLSeconds int    `json:"ttl_seconds"`
		Role       string `json:"role"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Code == "" || created.TTLSeconds != 600 || created.Role != "view" {
		t.Fatalf("created %+v", created)
	}

	exReq := httptest.NewRequest("POST", "/api/pairing/exchange", bytes.NewBufferString(`{"code":"`+created.Code+`"}`))
	exReq.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, exReq)
	if w.Code != 200 {
		t.Fatalf("exchange %d %s", w.Code, w.Body.String())
	}
	var ex struct {
		Token     string  `json:"token"`
		Role      string  `json:"role"`
		Minted    bool    `json:"minted"`
		ExpiresAt *string `json:"expires_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ex); err != nil {
		t.Fatal(err)
	}
	if ex.Token != "view-077" || ex.Role != "view" || ex.Minted || ex.ExpiresAt != nil {
		t.Fatalf("grant %+v", ex)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/pairing/exchange", bytes.NewBufferString(`{"code":"`+created.Code+`"}`)))
	if w.Code != 401 {
		t.Fatalf("second exchange %d %s", w.Code, w.Body.String())
	}

	get := httptest.NewRequest("GET", "/api/agents", nil)
	get.Header.Set("Authorization", "Bearer "+ex.Token)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 200 {
		t.Fatalf("exchanged view GET agents %d %s", w.Code, w.Body.String())
	}

	post := httptest.NewRequest("POST", "/api/system/backup", strings.NewReader("{}"))
	post.Header.Set("Authorization", "Bearer "+ex.Token)
	post.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, post)
	if w.Code != 403 {
		t.Fatalf("exchanged view POST backup %d %s", w.Code, w.Body.String())
	}
}

func TestPairing_HTTPViewCannotCreate(t *testing.T) {
	h, _ := pairingServer(t, "view-077")
	req := httptest.NewRequest("POST", "/api/pairing", nil)
	req.Header.Set("Authorization", "Bearer view-077")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("view create %d", w.Code)
	}

	req = httptest.NewRequest("POST", "/api/pairing", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("anon create %d", w.Code)
	}
}

func TestPairing_HTTPMintedGrant(t *testing.T) {
	h, _ := pairingServer(t, "")
	req := httptest.NewRequest("POST", "/api/pairing", nil)
	req.Header.Set("Authorization", "Bearer admin-077")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	exReq := httptest.NewRequest("POST", "/v1/pairing/exchange", bytes.NewBufferString(`{"code":"`+created.Code+`"}`))
	exReq.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, exReq)
	if w.Code != 200 {
		t.Fatalf("v1 exchange %d %s", w.Code, w.Body.String())
	}
	var ex struct {
		Token     string  `json:"token"`
		Minted    bool    `json:"minted"`
		ExpiresAt *string `json:"expires_at"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &ex)
	if !strings.HasPrefix(ex.Token, "gv_") || !ex.Minted || ex.ExpiresAt == nil {
		t.Fatalf("expected minted grant, got %+v", ex)
	}

	get := httptest.NewRequest("GET", "/v1/sessions", nil)
	get.Header.Set("Authorization", "Bearer "+ex.Token)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 200 {
		t.Fatalf("minted GET sessions %d", w.Code)
	}

	post := httptest.NewRequest("POST", "/api/kg/entities", bytes.NewBufferString(`{"name":"x"}`))
	post.Header.Set("Authorization", "Bearer "+ex.Token)
	post.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, post)
	if w.Code != 403 {
		t.Fatalf("minted POST kg %d %s", w.Code, w.Body.String())
	}

	post = httptest.NewRequest("POST", "/api/skills", bytes.NewBufferString(`{"name":"x","body":"y"}`))
	post.Header.Set("Authorization", "Bearer "+ex.Token)
	post.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, post)
	if w.Code != 403 {
		t.Fatalf("minted POST skills %d", w.Code)
	}

	post = httptest.NewRequest("POST", "/api/agents/abc/evolution/tick", strings.NewReader("{}"))
	post.Header.Set("Authorization", "Bearer "+ex.Token)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, post)
	if w.Code != 403 {
		t.Fatalf("minted POST evolution tick %d", w.Code)
	}
}

func TestPairing_HTTPExchangeBadInput(t *testing.T) {
	h, _ := pairingServer(t, "view-077")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/pairing/exchange", bytes.NewBufferString(`{`)))
	if w.Code != 400 {
		t.Fatalf("bad json %d", w.Code)
	}
	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/pairing/exchange", bytes.NewBufferString(`{"code":""}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("empty code %d", w.Code)
	}
}
