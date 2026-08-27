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

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/webhook"
)

func TestWebhookAPI_BearerAndHMAC(t *testing.T) {
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "test", Provider: llm.Echo{}})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/webhooks", nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created webhook.Created
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Token, "wh_") || created.HMACKey == "" {
		t.Fatalf("created %+v", created)
	}

	body := []byte(`{"input":"hello hook","mode":"sync"}`)

	w = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("bearer llm %d %s", w.Code, w.Body.String())
	}
	var reply map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	if reply["reply"] != "echo: hello hook" {
		t.Fatalf("reply %v", reply)
	}

	sig := webhook.Sign(created.HMACKey, time.Now(), body)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("X-Goso-Signature", sig)
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("hmac llm %d %s", w.Code, w.Body.String())
	}

	bad := webhook.Sign("not-the-key", time.Now(), body)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/webhooks/llm", bytes.NewReader(body))
	req.Header.Set("X-Goso-Signature", bad)
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("bad hmac %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/webhooks/llm", bytes.NewReader([]byte(`{"input":"x","mode":"async"}`)))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	h.ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatalf("async %d %s", w.Code, w.Body.String())
	}
	var accepted map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &accepted)
	if accepted["id"] == nil || accepted["id"] == "" {
		t.Fatalf("async id %v", accepted)
	}
}

func TestChannelsAPI_ListsSeven(t *testing.T) {
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("GOSO_ZALO_PERSONAL_TOKEN", "")
	t.Setenv("GOSO_ZALO_OA_ACCESS_TOKEN", "")
	t.Setenv("GOSO_DISCORD_BOT_TOKEN", "")
	t.Setenv("GOSO_SLACK_BOT_TOKEN", "")
	t.Setenv("GOSO_FEISHU_APP_SECRET", "")
	t.Setenv("GOSO_WHATSAPP_ACCESS_TOKEN", "")
	_, h := newTestServer()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/channels", nil))
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Channels []struct {
			Name       string `json:"name"`
			Configured bool   `json:"configured"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Channels) != 7 {
		t.Fatalf("want 7 got %d %s", len(body.Channels), w.Body.String())
	}
	names := map[string]bool{}
	for _, c := range body.Channels {
		names[c.Name] = true
		if c.Configured {
			t.Fatalf("%s configured", c.Name)
		}
	}
	for _, n := range []string{"telegram", "zalo-personal", "zalo-oa", "discord", "slack", "feishu", "whatsapp"} {
		if !names[n] {
			t.Fatalf("missing %s", n)
		}
	}
}
