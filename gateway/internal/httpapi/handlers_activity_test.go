// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func activityServer(t *testing.T) (*store.Store, *auditlog.Store, http.Handler) {
	t.Helper()
	st := store.New()
	al := auditlog.New(64)
	h := NewRouter(Options{Store: st, Version: "t", Events: eventstore.New(32), Audit: al})
	return st, al, h
}

func TestActivity_ListRedactsSecretsAndV1(t *testing.T) {
	_, al, h := activityServer(t)
	al.Append(auditlog.Record{
		Action:   "update",
		Actor:    "operator",
		Entity:   "provider",
		EntityID: "openai",
		IP:       "203.0.113.9",
		After: map[string]any{
			"enabled": true,
			"api_key": "sk-live-abcdefghijk",
			"token":   "super-secret",
			"body":    "secret-chat-body",
		},
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/activity", nil))
	if w.Code != 200 {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "super-secret") || strings.Contains(body, "sk-live-") || strings.Contains(body, "secret-chat-body") {
		t.Fatalf("leaked %s", body)
	}
	if strings.Contains(strings.ToLower(body), `"api_key"`) || strings.Contains(strings.ToLower(body), `"token"`) {
		t.Fatalf("secret keys %s", body)
	}
	if !strings.Contains(body, "203.0.113.9") || !strings.Contains(body, `"enabled":true`) && !strings.Contains(body, `"enabled": true`) {
		t.Fatalf("kept metadata %s", body)
	}
	assertSameGET(t, h, "/api/activity", "/v1/activity")
}

func TestActivity_FiltersPaginationAndSeparateFromEvents(t *testing.T) {
	_, al, h := activityServer(t)
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		al.Append(auditlog.Record{
			Action: "update", Actor: "operator", Entity: "agent", EntityID: "ag1",
			IP: "10.0.0.1", TS: base.Add(time.Duration(i) * time.Minute),
		})
	}
	al.Append(auditlog.Record{
		Action: "approve", Actor: "alice", Entity: "node", EntityID: "nd1",
		IP: "10.0.0.9", TS: base.Add(10 * time.Minute),
	})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/activity?action=approve", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"entity":"node"`) {
		t.Fatalf("action %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/activity?actor=alice", nil))
	if !strings.Contains(w.Body.String(), "nd1") {
		t.Fatalf("actor %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/activity?entity=agent", nil))
	var page auditlog.Page
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 5 {
		t.Fatalf("entity total %d", page.Total)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/activity?ip=10.0.0.9", nil))
	if !strings.Contains(w.Body.String(), "alice") {
		t.Fatalf("ip %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/activity?entity=agent&limit=2", nil))
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 2 || page.NextBefore == 0 {
		t.Fatalf("page1 %+v", page)
	}
	cursor := page.NextBefore
	al.Append(auditlog.Record{Action: "update", Actor: "operator", Entity: "agent", EntityID: "ag-new", TS: base.Add(30 * time.Minute)})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/activity?entity=agent&limit=2&before="+strconv.FormatInt(cursor, 10), nil))
	var page2 auditlog.Page
	if err := json.Unmarshal(w.Body.Bytes(), &page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Records) != 2 {
		t.Fatalf("page2 %+v", page2)
	}
	for _, r := range page2.Records {
		if r.Seq >= cursor {
			t.Fatalf("unstable %#v", r)
		}
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/events", nil))
	if strings.Contains(w.Body.String(), `"entity":"node"`) && strings.Contains(w.Body.String(), "alice") {
		t.Fatalf("events mixed audit %s", w.Body.String())
	}
}

func TestActivity_AgentCreateRecordsAndViewGET(t *testing.T) {
	_, al, h := activityServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewBufferString(`{"agent_key":"a1","display_name":"A1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.7:4444"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	page := al.Query(auditlog.Query{Entity: "agent", Action: "create"})
	if page.Total != 1 {
		t.Fatalf("audit %d %#v", page.Total, page.Records)
	}
	if page.Records[0].IP != "198.51.100.7" {
		t.Fatalf("ip %q", page.Records[0].IP)
	}
	if page.Records[0].After["api_key"] != nil {
		t.Fatalf("secret %#v", page.Records[0].After)
	}

	mw := auth.RequireTokens("admin-110", "view-110", []string{"/healthz"})
	guarded := mw(h)
	get := httptest.NewRequest(http.MethodGet, "/api/activity", nil)
	get.Header.Set("Authorization", "Bearer view-110")
	w = httptest.NewRecorder()
	guarded.ServeHTTP(w, get)
	if w.Code != 200 {
		t.Fatalf("view GET %d %s", w.Code, w.Body.String())
	}
	post := httptest.NewRequest(http.MethodPost, "/api/activity", strings.NewReader(`{}`))
	post.Header.Set("Authorization", "Bearer view-110")
	w = httptest.NewRecorder()
	guarded.ServeHTTP(w, post)
	if w.Code != 403 {
		t.Fatalf("view POST %d %s", w.Code, w.Body.String())
	}
}
