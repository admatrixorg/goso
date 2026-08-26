// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
}

func TestRequireToken_PassAndFail(t *testing.T) {
	mw := RequireToken("secret", []string{"/healthz"})
	h := mw(okHandler())

	// no token -> 401
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents", nil))
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	// correct bearer
	req := httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// query token
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents?token=secret", nil))
	if w.Code != 200 {
		t.Fatalf("query token 200, got %d", w.Code)
	}
	// wrong token
	req = httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("wrong token 401, got %d", w.Code)
	}
	// bypass healthz
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Fatalf("healthz bypass 200, got %d", w.Code)
	}
}

func TestRequireToken_EmptyRefuses(t *testing.T) {
	mw := RequireToken("", []string{"/healthz"})
	h := mw(okHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents", nil))
	if w.Code != 401 {
		t.Fatalf("empty token 401, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Fatalf("healthz bypass 200, got %d", w.Code)
	}
}
