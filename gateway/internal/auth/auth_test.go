// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

	v1get := httptest.NewRequest("GET", "/v1/agents", nil)
	v1get.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, v1get)
	if w.Code != 200 {
		t.Fatalf("view GET /v1/agents 200, got %d", w.Code)
	}

	v1sess := httptest.NewRequest("GET", "/v1/sessions", nil)
	v1sess.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, v1sess)
	if w.Code != 200 {
		t.Fatalf("view GET /v1/sessions 200, got %d", w.Code)
	}

	post := httptest.NewRequest("POST", "/api/chat", nil)
	post.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, post)
	if w.Code != 403 {
		t.Fatalf("view POST chat 403, got %d", w.Code)
	}

	v1post := httptest.NewRequest("POST", "/v1/chat", nil)
	v1post.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, v1post)
	if w.Code != 403 {
		t.Fatalf("view POST /v1/chat 403, got %d", w.Code)
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

func TestRequireTokens_ViewPOSTDenyMatrix(t *testing.T) {
	mw := RequireTokens("admin-077", "view-077", []string{"/healthz"})
	h := mw(okHandler())
	paths := []string{
		"/api/system/backup",
		"/api/kg/entities",
		"/api/kg/relations",
		"/api/skills",
		"/api/agents/abc/evolution/tick",
		"/v1/system/backup",
		"/v1/kg/entities",
		"/v1/skills",
		"/api/pairing",
	}
	for _, path := range paths {
		req := httptest.NewRequest("POST", path, nil)
		req.Header.Set("Authorization", "Bearer view-077")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatalf("view POST %s got %d want 403", path, w.Code)
		}
	}
	req := httptest.NewRequest("POST", "/api/system/backup", nil)
	req.Header.Set("Authorization", "Bearer admin-077")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("admin POST backup 200, got %d", w.Code)
	}
}

func TestRequire_PairingExchangeExactPath(t *testing.T) {
	h := Require(Config{Admin: "admin-077", Bypass: []string{"/healthz"}})(okHandler())

	ex := httptest.NewRequest("POST", "/api/pairing/exchange", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, ex)
	if w.Code != 200 {
		t.Fatalf("exact POST exchange 200, got %d", w.Code)
	}

	extra := httptest.NewRequest("POST", "/api/pairing/exchange/extra", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, extra)
	if w.Code != 401 {
		t.Fatalf("suffix exchange 401, got %d", w.Code)
	}

	get := httptest.NewRequest("GET", "/api/pairing/exchange", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 401 {
		t.Fatalf("GET exchange 401, got %d", w.Code)
	}

	create := httptest.NewRequest("POST", "/api/pairing", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, create)
	if w.Code != 401 {
		t.Fatalf("anon POST pairing 401, got %d", w.Code)
	}
}

func TestRequire_EnvViewAfterCodeExpiry(t *testing.T) {
	p := NewPairing()
	base := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return base }
	issued, err := p.Issue("view-077")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exchange(issued.Code); err != nil {
		t.Fatal(err)
	}
	p.now = func() time.Time { return base.Add(PairingTTL + time.Second) }
	h := Require(Config{Admin: "admin-077", View: "view-077", Pairing: p, Bypass: []string{"/healthz"}})(okHandler())
	req := httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer view-077")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("env view after code TTL 200, got %d", w.Code)
	}
}

func TestRequire_MintedGrantGETOnly(t *testing.T) {
	p := NewPairing()
	issued, err := p.Issue("")
	if err != nil {
		t.Fatal(err)
	}
	ex, err := p.Exchange(issued.Code)
	if err != nil {
		t.Fatal(err)
	}
	h := Require(Config{Admin: "admin-077", Pairing: p, Bypass: []string{"/healthz"}})(okHandler())

	get := httptest.NewRequest("GET", "/api/agents", nil)
	get.Header.Set("Authorization", "Bearer "+ex.Token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 200 {
		t.Fatalf("grant GET agents 200, got %d", w.Code)
	}

	post := httptest.NewRequest("POST", "/api/system/backup", nil)
	post.Header.Set("Authorization", "Bearer "+ex.Token)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, post)
	if w.Code != 403 {
		t.Fatalf("grant POST backup 403, got %d", w.Code)
	}
}

func TestRequireToken_ProductionIgnoresQuery(t *testing.T) {
	t.Setenv("GOSO_ENV", "production")
	mw := RequireToken("secret", []string{"/healthz"})
	h := mw(okHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/agents?token=secret", nil))
	if w.Code != 401 {
		t.Fatalf("production query token ignored, got %d", w.Code)
	}
	req := httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("production bearer 200, got %d", w.Code)
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
