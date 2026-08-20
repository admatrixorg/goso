// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
}

func TestLimiter_AllowAndBlock(t *testing.T) {
	lim := New(2)
	h := lim.Middleware(okHandler())

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/agents", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		h.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("request %d expected 200, got %d", i, w.Code)
		}
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/agents", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	h.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After")
	}
	// different IP not blocked
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/agents", nil)
	req.RemoteAddr = "5.6.7.8:1234"
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("different IP 200, got %d", w.Code)
	}
}

func TestLimiter_Disabled(t *testing.T) {
	lim := New(0)
	h := lim.Middleware(okHandler())
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/agents", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		h.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("disabled expected 200, got %d", w.Code)
		}
	}
}
