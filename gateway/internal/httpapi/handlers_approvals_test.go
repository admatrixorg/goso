// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/apikey"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func approvalsServer(t *testing.T) (*approval.Gate, *auditlog.Store, http.Handler) {
	t.Helper()
	st := store.New()
	gate := approval.New(0)
	al := auditlog.New(64)
	ev := eventstore.New(32)
	reg := connector.NewRegistry()
	fake := connector.NewFake("zalocrm", []connector.Tool{
		{Name: "contact_search", Description: "s", RequiresApproval: false, InputSchema: json.RawMessage(`{}`)},
		{Name: "message_send", Description: "m", RequiresApproval: true, InputSchema: json.RawMessage(`{}`)},
	})
	_ = reg.Register(fake)
	rt := agent.New(st, reg, gate, ev, nil)
	h := NewRouter(Options{Store: st, Version: "t", Registry: reg, Gate: gate, Events: ev, Runtime: rt, Audit: al})
	return gate, al, h
}

func apprJSON(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func assertNoApprovalSecrets(t *testing.T, body, secret string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, n := range []string{`"args"`, `"arguments"`, `"token"`, `"secret"`, `"api_key"`, `"password"`, `"authorization"`} {
		if strings.Contains(lower, n) {
			t.Fatalf("secret field in body: %s", body)
		}
	}
	if secret != "" && strings.Contains(body, secret) {
		t.Fatalf("plaintext in body: %s", body)
	}
	if strings.Contains(body, "sk-") || strings.Contains(body, "gsk_") {
		t.Fatalf("token shape in body: %s", body)
	}
}

func seedPending(t *testing.T, h http.Handler, secret string) string {
	t.Helper()
	w := apprJSON(t, h, "POST", "/api/tools/invoke", `{"connector":"zalocrm","tool":"message_send","arguments":{"contact_id":"1","text":"hi","token":"`+secret+`"}}`, "")
	if w.Code != 200 {
		t.Fatalf("invoke %d %s", w.Code, w.Body.String())
	}
	var inv struct {
		Result struct {
			ApprovalID string `json:"approval_id"`
			Status     string `json:"status"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &inv); err != nil || inv.Result.ApprovalID == "" {
		t.Fatalf("id %v %s", err, w.Body.String())
	}
	if inv.Result.Status != "pending_approval" {
		t.Fatalf("status %s", w.Body.String())
	}
	return inv.Result.ApprovalID
}

func TestApprovals_ListGETOmitsSecretsAuditSingle(t *testing.T) {
	_, al, h := approvalsServer(t)
	secret := "sk-live-fixture-not-vendor-zzzz"
	id := seedPending(t, h, secret)

	w := apprJSON(t, h, "GET", "/api/approvals", "", "")
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	assertNoApprovalSecrets(t, w.Body.String(), secret)
	if !strings.Contains(w.Body.String(), `"kind":"execution"`) || !strings.Contains(w.Body.String(), id) {
		t.Fatalf("inbox %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"pending":1`) && !strings.Contains(w.Body.String(), `"pending": 1`) {
		t.Fatalf("pending count %s", w.Body.String())
	}

	w = apprJSON(t, h, "GET", "/api/approvals/"+id, "", "")
	if w.Code != 200 {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}
	assertNoApprovalSecrets(t, w.Body.String(), secret)

	w = apprJSON(t, h, "POST", "/api/approvals/"+id+"/decision", `{"decision":"deny"}`, "")
	if w.Code != 400 || !strings.Contains(w.Body.String(), "denial reason is required") {
		t.Fatalf("deny reason %d %s", w.Code, w.Body.String())
	}

	w = apprJSON(t, h, "POST", "/api/approvals/"+id+"/decision", `{"decision":"deny","reason":"too risky"}`, "")
	if w.Code != 200 {
		t.Fatalf("deny %d %s", w.Code, w.Body.String())
	}
	assertNoApprovalSecrets(t, w.Body.String(), secret)
	if !strings.Contains(w.Body.String(), `"status":"rejected"`) {
		t.Fatalf("rejected %s", w.Body.String())
	}

	w = apprJSON(t, h, "POST", "/api/approvals/"+id+"/decision", `{"decision":"approve"}`, "")
	if w.Code != http.StatusConflict {
		t.Fatalf("second %d %s", w.Code, w.Body.String())
	}

	w = apprJSON(t, h, "GET", "/v1/approvals", "", "")
	if w.Code != 200 {
		t.Fatalf("v1 list %d %s", w.Code, w.Body.String())
	}
	assertNoApprovalSecrets(t, w.Body.String(), secret)
	if !strings.Contains(w.Body.String(), `"pending":0`) && !strings.Contains(w.Body.String(), `"pending": 0`) {
		t.Fatalf("v1 empty inbox %s", w.Body.String())
	}

	page := al.Query(auditlog.Query{Entity: "approval"})
	if page.Total < 1 {
		t.Fatalf("audit %d", page.Total)
	}
	for _, rec := range page.Records {
		raw, _ := json.Marshal(rec)
		if strings.Contains(string(raw), secret) || rec.After["args"] != nil || rec.After["token"] != nil {
			t.Fatalf("audit secret %#v", rec)
		}
		if rec.Action != "deny" {
			t.Fatalf("action %s", rec.Action)
		}
	}

	w = apprJSON(t, h, "GET", "/api/approvals", "", "")
	if strings.Contains(w.Body.String(), id) {
		t.Fatalf("resolved still in inbox %s", w.Body.String())
	}
}

func TestApprovals_ApproveAndExpired(t *testing.T) {
	_, al, h := approvalsServer(t)
	id := seedPending(t, h, "sk-aaaaaaaaaaaaaaaa")
	w := apprJSON(t, h, "POST", "/api/approvals/"+id+"/decision", `{"decision":"approve"}`, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"approved"`) {
		t.Fatalf("approve %d %s", w.Code, w.Body.String())
	}
	page := al.Query(auditlog.Query{Action: "approve", Entity: "approval"})
	if page.Total < 1 {
		t.Fatalf("approve audit %d", page.Total)
	}

	short := approval.New(10 * time.Millisecond)
	h2 := NewRouter(Options{Store: store.New(), Version: "t", Gate: short, Events: eventstore.New(8), Audit: auditlog.New(64)})
	req := short.Submit("c", "t", nil, nil)
	time.Sleep(20 * time.Millisecond)
	w = apprJSON(t, h2, "GET", "/api/approvals/"+req.ID, "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"expired"`) {
		t.Fatalf("expired get %d %s", w.Code, w.Body.String())
	}
	assertNoApprovalSecrets(t, w.Body.String(), "")
	w = apprJSON(t, h2, "POST", "/api/approvals/"+req.ID+"/decision", `{"decision":"approve"}`, "")
	if w.Code != http.StatusGone {
		t.Fatalf("expired decide %d %s", w.Code, w.Body.String())
	}
}

func TestApprovals_ViewTokenGETOnly(t *testing.T) {
	_, _, inner := approvalsServer(t)
	h := auth.RequireTokens("admin-115", "view-115", []string{"/healthz"})(inner)
	w := apprJSON(t, h, "GET", "/api/approvals", "", "view-115")
	if w.Code != 200 {
		t.Fatalf("view GET %d %s", w.Code, w.Body.String())
	}
	w = apprJSON(t, h, "GET", "/v1/approvals", "", "view-115")
	if w.Code != 200 {
		t.Fatalf("view v1 %d %s", w.Code, w.Body.String())
	}
	w = apprJSON(t, h, "POST", "/api/approvals/x/decision", `{"decision":"approve"}`, "view-115")
	if w.Code != http.StatusForbidden {
		t.Fatalf("view POST %d %s", w.Code, w.Body.String())
	}
}

func TestApprovals_IssuedScopes(t *testing.T) {
	keys := apikey.New()
	st := store.New()
	gate := approval.New(0)
	inner := NewRouter(Options{Store: st, Version: "t", Gate: gate, APIKeys: keys, Events: eventstore.New(8)})
	h := auth.Require(auth.Config{Admin: "admin-115", Keys: keys})(inner)

	w := apprJSON(t, h, "POST", "/api/api-keys", `{"name":"reader","scopes":["read"]}`, "admin-115")
	if w.Code != 201 {
		t.Fatalf("create read %d %s", w.Code, w.Body.String())
	}
	var reader struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &reader)
	w = apprJSON(t, h, "GET", "/api/approvals", "", reader.Secret)
	if w.Code != 200 {
		t.Fatalf("read GET %d %s", w.Code, w.Body.String())
	}
	req := gate.Submit("zalocrm", "message_send", map[string]any{"contact_id": "1"}, nil)
	w = apprJSON(t, h, "POST", "/api/approvals/"+req.ID+"/decision", `{"decision":"approve"}`, reader.Secret)
	if w.Code != http.StatusForbidden {
		t.Fatalf("read POST %d %s", w.Code, w.Body.String())
	}

	w = apprJSON(t, h, "POST", "/api/api-keys", `{"name":"writer","scopes":["write"]}`, "admin-115")
	var writer struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &writer)
	w = apprJSON(t, h, "POST", "/api/approvals/"+req.ID+"/decision", `{"decision":"approve"}`, writer.Secret)
	if w.Code != http.StatusForbidden {
		t.Fatalf("write decide %d %s", w.Code, w.Body.String())
	}

	w = apprJSON(t, h, "POST", "/api/api-keys", `{"name":"approver","scopes":["approvals"]}`, "admin-115")
	var approver struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &approver)
	w = apprJSON(t, h, "GET", "/api/approvals", "", approver.Secret)
	if w.Code != 200 {
		t.Fatalf("approvals GET %d %s", w.Code, w.Body.String())
	}
	w = apprJSON(t, h, "POST", "/api/approvals/"+req.ID+"/decision", `{"decision":"approve"}`, approver.Secret)
	if w.Code != 200 {
		t.Fatalf("approvals decide %d %s", w.Code, w.Body.String())
	}
}
