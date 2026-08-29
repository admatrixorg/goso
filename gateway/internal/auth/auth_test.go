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

	pending := httptest.NewRequest("GET", "/api/pending-messages", nil)
	pending.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, pending)
	if w.Code != 200 {
		t.Fatalf("view GET pending 200, got %d", w.Code)
	}

	compact := httptest.NewRequest("POST", "/api/pending-messages/pg_1/compact", nil)
	compact.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, compact)
	if w.Code != 403 {
		t.Fatalf("view POST compact 403, got %d", w.Code)
	}

	clear := httptest.NewRequest("POST", "/api/pending-messages/pg_1/clear", nil)
	clear.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, clear)
	if w.Code != 403 {
		t.Fatalf("view POST clear 403, got %d", w.Code)
	}

	contacts := httptest.NewRequest("GET", "/api/contacts", nil)
	contacts.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, contacts)
	if w.Code != 200 {
		t.Fatalf("view GET contacts 200, got %d", w.Code)
	}

	merge := httptest.NewRequest("POST", "/api/contacts/ct_1/merge", nil)
	merge.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, merge)
	if w.Code != 403 {
		t.Fatalf("view POST merge 403, got %d", w.Code)
	}

	undo := httptest.NewRequest("POST", "/api/contacts/ct_1/undo", nil)
	undo.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, undo)
	if w.Code != 403 {
		t.Fatalf("view POST undo 403, got %d", w.Code)
	}

	nodes := httptest.NewRequest("GET", "/api/nodes", nil)
	nodes.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, nodes)
	if w.Code != 200 {
		t.Fatalf("view GET nodes 200, got %d", w.Code)
	}

	approve := httptest.NewRequest("POST", "/api/nodes/nd_1/approve", nil)
	approve.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, approve)
	if w.Code != 403 {
		t.Fatalf("view POST approve 403, got %d", w.Code)
	}

	deny := httptest.NewRequest("POST", "/api/nodes/nd_1/deny", nil)
	deny.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, deny)
	if w.Code != 403 {
		t.Fatalf("view POST deny 403, got %d", w.Code)
	}

	revoke := httptest.NewRequest("POST", "/api/nodes/nd_1/revoke", nil)
	revoke.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, revoke)
	if w.Code != 403 {
		t.Fatalf("view POST revoke 403, got %d", w.Code)
	}

	ws := httptest.NewRequest("GET", "/api/workstations", nil)
	ws.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, ws)
	if w.Code != 200 {
		t.Fatalf("view GET workstations 200, got %d", w.Code)
	}

	wsOne := httptest.NewRequest("GET", "/api/workstations/ws_1", nil)
	wsOne.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, wsOne)
	if w.Code != 200 {
		t.Fatalf("view GET workstation id 200, got %d", w.Code)
	}

	wsCreate := httptest.NewRequest("POST", "/api/workstations", nil)
	wsCreate.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, wsCreate)
	if w.Code != 403 {
		t.Fatalf("view POST workstations 403, got %d", w.Code)
	}

	wsTest := httptest.NewRequest("POST", "/api/workstations/ws_1/test", nil)
	wsTest.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, wsTest)
	if w.Code != 403 {
		t.Fatalf("view POST test 403, got %d", w.Code)
	}

	wsDel := httptest.NewRequest("POST", "/api/workstations/ws_1/delete", nil)
	wsDel.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, wsDel)
	if w.Code != 403 {
		t.Fatalf("view POST delete 403, got %d", w.Code)
	}

	stList := httptest.NewRequest("GET", "/api/storage", nil)
	stList.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, stList)
	if w.Code != 200 {
		t.Fatalf("view GET storage 200, got %d", w.Code)
	}

	stPrev := httptest.NewRequest("GET", "/api/storage/preview", nil)
	stPrev.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, stPrev)
	if w.Code != 200 {
		t.Fatalf("view GET storage preview 200, got %d", w.Code)
	}

	evList := httptest.NewRequest("GET", "/api/events", nil)
	evList.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, evList)
	if w.Code != 200 {
		t.Fatalf("view GET events 200, got %d", w.Code)
	}

	evStream := httptest.NewRequest("GET", "/api/events/stream", nil)
	evStream.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, evStream)
	if w.Code != 200 {
		t.Fatalf("view GET events stream 200, got %d", w.Code)
	}

	v1ev := httptest.NewRequest("GET", "/v1/events", nil)
	v1ev.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, v1ev)
	if w.Code != 200 {
		t.Fatalf("view GET /v1/events 200, got %d", w.Code)
	}

	act := httptest.NewRequest("GET", "/api/activity", nil)
	act.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, act)
	if w.Code != 200 {
		t.Fatalf("view GET activity 200, got %d", w.Code)
	}

	v1act := httptest.NewRequest("GET", "/v1/activity", nil)
	v1act.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, v1act)
	if w.Code != 200 {
		t.Fatalf("view GET /v1/activity 200, got %d", w.Code)
	}

	actPost := httptest.NewRequest("POST", "/api/activity", nil)
	actPost.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, actPost)
	if w.Code != 403 {
		t.Fatalf("view POST activity 403, got %d", w.Code)
	}

	lgList := httptest.NewRequest("GET", "/api/logs", nil)
	lgList.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, lgList)
	if w.Code != 200 {
		t.Fatalf("view GET logs 200, got %d", w.Code)
	}

	lgStream := httptest.NewRequest("GET", "/api/logs/stream", nil)
	lgStream.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, lgStream)
	if w.Code != 200 {
		t.Fatalf("view GET logs stream 200, got %d", w.Code)
	}

	v1lg := httptest.NewRequest("GET", "/v1/logs", nil)
	v1lg.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, v1lg)
	if w.Code != 200 {
		t.Fatalf("view GET /v1/logs 200, got %d", w.Code)
	}

	lgPost := httptest.NewRequest("POST", "/api/logs", nil)
	lgPost.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, lgPost)
	if w.Code != 403 {
		t.Fatalf("view POST logs 403, got %d", w.Code)
	}

	tnList := httptest.NewRequest("GET", "/api/tenants", nil)
	tnList.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, tnList)
	if w.Code != 200 {
		t.Fatalf("view GET tenants 200, got %d", w.Code)
	}

	tnCtx := httptest.NewRequest("GET", "/api/tenant", nil)
	tnCtx.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, tnCtx)
	if w.Code != 200 {
		t.Fatalf("view GET tenant 200, got %d", w.Code)
	}

	v1tn := httptest.NewRequest("GET", "/v1/tenants", nil)
	v1tn.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, v1tn)
	if w.Code != 200 {
		t.Fatalf("view GET /v1/tenants 200, got %d", w.Code)
	}

	tnPost := httptest.NewRequest("POST", "/api/tenants", nil)
	tnPost.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, tnPost)
	if w.Code != 403 {
		t.Fatalf("view POST tenants 403, got %d", w.Code)
	}

	akList := httptest.NewRequest("GET", "/api/api-keys", nil)
	akList.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, akList)
	if w.Code != 200 {
		t.Fatalf("view GET api-keys 200, got %d", w.Code)
	}

	v1ak := httptest.NewRequest("GET", "/v1/api-keys", nil)
	v1ak.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, v1ak)
	if w.Code != 200 {
		t.Fatalf("view GET /v1/api-keys 200, got %d", w.Code)
	}

	akPost := httptest.NewRequest("POST", "/api/api-keys", nil)
	akPost.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, akPost)
	if w.Code != 403 {
		t.Fatalf("view POST api-keys 403, got %d", w.Code)
	}

	pkgList := httptest.NewRequest("GET", "/api/packages", nil)
	pkgList.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, pkgList)
	if w.Code != 200 {
		t.Fatalf("view GET packages 200, got %d", w.Code)
	}

	v1pkg := httptest.NewRequest("GET", "/v1/packages", nil)
	v1pkg.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, v1pkg)
	if w.Code != 200 {
		t.Fatalf("view GET /v1/packages 200, got %d", w.Code)
	}

	pkgOne := httptest.NewRequest("GET", "/api/packages/pk_1", nil)
	pkgOne.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, pkgOne)
	if w.Code != 200 {
		t.Fatalf("view GET package id 200, got %d", w.Code)
	}

	pkgInstall := httptest.NewRequest("POST", "/api/packages/install", nil)
	pkgInstall.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, pkgInstall)
	if w.Code != 403 {
		t.Fatalf("view POST packages install 403, got %d", w.Code)
	}

	apprList := httptest.NewRequest("GET", "/api/approvals", nil)
	apprList.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, apprList)
	if w.Code != 200 {
		t.Fatalf("view GET approvals 200, got %d", w.Code)
	}

	v1appr := httptest.NewRequest("GET", "/v1/approvals", nil)
	v1appr.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, v1appr)
	if w.Code != 200 {
		t.Fatalf("view GET /v1/approvals 200, got %d", w.Code)
	}

	apprOne := httptest.NewRequest("GET", "/api/approvals/appr-1", nil)
	apprOne.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, apprOne)
	if w.Code != 200 {
		t.Fatalf("view GET approval id 200, got %d", w.Code)
	}

	apprDec := httptest.NewRequest("POST", "/api/approvals/appr-1/decision", nil)
	apprDec.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, apprDec)
	if w.Code != 403 {
		t.Fatalf("view POST approvals 403, got %d", w.Code)
	}

	stDel := httptest.NewRequest("POST", "/api/storage/delete", nil)
	stDel.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, stDel)
	if w.Code != 403 {
		t.Fatalf("view POST storage delete 403, got %d", w.Code)
	}

	stUp := httptest.NewRequest("POST", "/api/storage/upload", nil)
	stUp.Header.Set("Authorization", "Bearer view-041")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, stUp)
	if w.Code != 403 {
		t.Fatalf("view POST storage upload 403, got %d", w.Code)
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
		"/api/system/heartbeat",
		"/api/kg/entities",
		"/api/kg/relations",
		"/api/skills",
		"/api/agents/abc/evolution/tick",
		"/v1/system/backup",
		"/v1/system/heartbeat",
		"/v1/kg/entities",
		"/v1/skills",
		"/api/pairing",
		"/api/nodes/nd_1/approve",
		"/api/nodes/nd_1/deny",
		"/api/nodes/nd_1/revoke",
		"/v1/nodes/nd_1/approve",
		"/api/workstations",
		"/api/workstations/ws_1/test",
		"/api/workstations/ws_1/disconnect",
		"/api/workstations/ws_1/delete",
		"/v1/workstations/ws_1/delete",
		"/api/storage/upload",
		"/api/storage/delete",
		"/v1/storage/delete",
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

func TestRequire_NodeRequestExactPath(t *testing.T) {
	h := Require(Config{Admin: "admin-077", Bypass: []string{"/healthz"}})(okHandler())

	req := httptest.NewRequest("POST", "/api/nodes/request", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("exact POST nodes request 200, got %d", w.Code)
	}

	v1 := httptest.NewRequest("POST", "/v1/nodes/request", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, v1)
	if w.Code != 200 {
		t.Fatalf("v1 POST nodes request 200, got %d", w.Code)
	}

	extra := httptest.NewRequest("POST", "/api/nodes/request/extra", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, extra)
	if w.Code != 401 {
		t.Fatalf("suffix request 401, got %d", w.Code)
	}

	get := httptest.NewRequest("GET", "/api/nodes/request", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 401 {
		t.Fatalf("GET nodes request 401, got %d", w.Code)
	}

	list := httptest.NewRequest("GET", "/api/nodes", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, list)
	if w.Code != 401 {
		t.Fatalf("anon GET nodes 401, got %d", w.Code)
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

type fakeKeys struct {
	token string
	grant Grant
}

func (f fakeKeys) Accept(token string) (Grant, bool) {
	if token == f.token {
		return f.grant, true
	}
	return Grant{}, false
}

func TestRequire_IssuedAPIKeyScopes(t *testing.T) {
	keys := fakeKeys{token: "gk_issued", grant: Grant{ID: "ak_1", Prefix: "gk_issued", Scopes: []string{"read"}}}
	h := Require(Config{Admin: "admin-113", Keys: keys, Bypass: []string{"/healthz"}})(okHandler())

	get := httptest.NewRequest("GET", "/api/api-keys", nil)
	get.Header.Set("Authorization", "Bearer gk_issued")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 200 {
		t.Fatalf("read GET keys %d", w.Code)
	}

	post := httptest.NewRequest("POST", "/api/api-keys", nil)
	post.Header.Set("Authorization", "Bearer gk_issued")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, post)
	if w.Code != 403 {
		t.Fatalf("read POST keys %d", w.Code)
	}

	writeKeys := fakeKeys{token: "gk_write", grant: Grant{ID: "ak_2", Prefix: "gk_write", Scopes: []string{"write"}}}
	h = Require(Config{Admin: "admin-113", Keys: writeKeys})(okHandler())
	agents := httptest.NewRequest("POST", "/api/agents", nil)
	agents.Header.Set("Authorization", "Bearer gk_write")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, agents)
	if w.Code != 200 {
		t.Fatalf("write POST agents %d", w.Code)
	}
	keysPost := httptest.NewRequest("POST", "/api/api-keys", nil)
	keysPost.Header.Set("Authorization", "Bearer gk_write")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, keysPost)
	if w.Code != 403 {
		t.Fatalf("write POST keys %d", w.Code)
	}
	pkgPost := httptest.NewRequest("POST", "/api/packages/install", nil)
	pkgPost.Header.Set("Authorization", "Bearer gk_write")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, pkgPost)
	if w.Code != 403 {
		t.Fatalf("write POST packages %d", w.Code)
	}

	adminKeys := fakeKeys{token: "gk_admin", grant: Grant{ID: "ak_3", Prefix: "gk_admin", Scopes: []string{"admin"}}}
	h = Require(Config{Admin: "admin-113", Keys: adminKeys})(okHandler())
	adminPost := httptest.NewRequest("POST", "/api/api-keys", nil)
	adminPost.Header.Set("Authorization", "Bearer gk_admin")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, adminPost)
	if w.Code != 200 {
		t.Fatalf("admin POST keys %d", w.Code)
	}

	revoked := httptest.NewRequest("GET", "/api/agents", nil)
	revoked.Header.Set("Authorization", "Bearer missing")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, revoked)
	if w.Code != 401 {
		t.Fatalf("unknown issued %d", w.Code)
	}
}
