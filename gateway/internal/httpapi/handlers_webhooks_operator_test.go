// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/webhook"
)

func TestWebhookAPI_ListStatusEndpointLastDelivery(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_SSRF", "")
	var sawSecret bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "hmac_key") || strings.Contains(string(b), `"token"`) || strings.Contains(string(b), "wh_") {
			sawSecret = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ev := eventstore.New(64)
	h := NewRouter(Options{Store: store.New(), Version: "test", Provider: llm.Echo{}, Events: ev})

	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"name": "ops", "endpoint": srv.URL})
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/webhooks", bytes.NewReader(body)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created webhook.Created
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.HMACKey == "" {
		t.Fatalf("secret once %+v", created)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/webhooks", nil))
	raw := w.Body.String()
	if strings.Contains(raw, created.Token) || strings.Contains(raw, created.HMACKey) || strings.Contains(raw, `"hmac_key"`) || strings.Contains(raw, `"token":`) {
		t.Fatalf("GET list leaked secret %s", raw)
	}
	var listed struct {
		Webhooks []webhook.Public `json:"webhooks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Webhooks) != 1 {
		t.Fatalf("list %s", raw)
	}
	row := listed.Webhooks[0]
	if row.Status != "active" || row.Endpoint != srv.URL || row.TokenPrefix == "" {
		t.Fatalf("row %+v", row)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/webhooks/"+created.ID+"/test", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("test %d %s", w.Code, w.Body.String())
	}
	if sawSecret {
		t.Fatal("test payload contained secrets")
	}
	if strings.Contains(w.Body.String(), created.Token) || strings.Contains(w.Body.String(), created.HMACKey) {
		t.Fatalf("test response leaked %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/webhooks", nil))
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Webhooks[0].LastDelivery == nil || listed.Webhooks[0].LastDelivery.Status == "" {
		t.Fatalf("last delivery %+v", listed.Webhooks[0])
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/webhooks/"+created.ID+"/replay", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("replay %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/webhooks/"+created.ID+"/rotate", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("rotate %d %s", w.Code, w.Body.String())
	}
	var rotated webhook.Created
	_ = json.Unmarshal(w.Body.Bytes(), &rotated)
	if rotated.Token == "" || rotated.Token == created.Token {
		t.Fatalf("rotate secret %+v", rotated)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/webhooks/"+created.ID, nil))
	getRaw := w.Body.String()
	if strings.Contains(getRaw, created.Token) || strings.Contains(getRaw, created.HMACKey) || strings.Contains(getRaw, rotated.Token) || strings.Contains(getRaw, rotated.HMACKey) {
		t.Fatalf("GET leaked %s", getRaw)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/webhooks/"+created.ID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("revoke %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/webhooks", nil))
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Webhooks) != 1 || listed.Webhooks[0].Status != "revoked" {
		t.Fatalf("revoked list %+v", listed.Webhooks)
	}

	events := ev.Filter("", "webhooks", 20)
	tools := map[string]bool{}
	for _, e := range events {
		if strings.Contains(e.Summary, created.Token) || strings.Contains(e.Summary, created.HMACKey) {
			t.Fatalf("audit leaked secret %#v", e)
		}
		tools[e.Tool] = true
	}
	for _, want := range []string{"test", "replay", "rotate", "revoke"} {
		if !tools[want] {
			t.Fatalf("missing audit %s %#v", want, events)
		}
	}
}

func TestWebhookAPI_TestWithoutEndpoint400(t *testing.T) {
	h := webhookRouter(t)
	created := createWebhook(t, h)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/webhooks/"+created.ID+"/test", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("test %d %s", w.Code, w.Body.String())
	}
}
