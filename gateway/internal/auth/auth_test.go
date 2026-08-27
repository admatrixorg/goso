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

func TestRequireToken_ConstantTimeBearer(t *testing.T) {
	mw := RequireToken("secret-041", []string{"/healthz"})
	h := mw(okHandler())
	req := httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer secret-041")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("match 200, got %d", w.Code)
	}
	req = httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer secret-040")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("mismatch 401, got %d", w.Code)
	}
}

func TestRequireTokens_ViewGETOnly(t *testing.T) {
	mw := RequireTokens("admin-041", "view-041", []string{"/healthz"})
	h := mw(okHandler())

	get := httptest.NewRequest("GET", "/api/agents", nil)
	get.Header.Set("Authorization", "Bearer view-041")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 200 {
		t.Fatalf("view GET agents 200, got %d", w.Code)
	}

	sess := httptest.NewRequest("GET", "/api/sessions", nil)
	sess.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, sess)
	if w.Code != 200 {
		t.Fatalf("view GET sessions 200, got %d", w.Code)
	}

	post := httptest.NewRequest("POST", "/api/chat", nil)
	post.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, post)
	if w.Code != 403 {
		t.Fatalf("view POST chat 403, got %d", w.Code)
	}

	one := httptest.NewRequest("GET", "/api/agents/abc", nil)
	one.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, one)
	if w.Code != 200 {
		t.Fatalf("view GET agent id 200, got %d", w.Code)
	}

	msgs := httptest.NewRequest("GET", "/api/sessions/abc/messages", nil)
	msgs.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, msgs)
	if w.Code != 403 {
		t.Fatalf("view GET messages 403, got %d", w.Code)
	}

	other := httptest.NewRequest("GET", "/api/vault/docs", nil)
	other.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, other)
	if w.Code != 403 {
		t.Fatalf("view GET vault 403, got %d", w.Code)
	}

	admin := httptest.NewRequest("POST", "/api/chat", nil)
	admin.Header.Set("Authorization", "Bearer admin-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, admin)
	if w.Code != 200 {
		t.Fatalf("admin POST 200, got %d", w.Code)
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
