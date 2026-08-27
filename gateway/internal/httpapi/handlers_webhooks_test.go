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

func TestWebhookAPI_ListPublicOnly(t *testing.T) {
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "test", Provider: llm.Echo{}})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/webhooks", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("empty list %d %s", w.Code, w.Body.String())
	}
	var empty struct {
		Webhooks []webhook.Public `json:"webhooks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Webhooks == nil {
		t.Fatalf("want empty array not null %s", w.Body.String())
	}
	if len(empty.Webhooks) != 0 {
		t.Fatalf("want 0 got %d %s", len(empty.Webhooks), w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/webhooks", nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created webhook.Created
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.HMACKey == "" || created.TokenPrefix == "" {
		t.Fatalf("created %+v", created)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/webhooks", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	raw := w.Body.String()
	if strings.Contains(raw, created.Token) {
		t.Fatalf("full token in list %s", raw)
	}
	if strings.Contains(raw, created.HMACKey) {
		t.Fatalf("hmac in list %s", raw)
	}
	if strings.Contains(raw, `"token":`) {
		t.Fatalf("token field in list %s", raw)
	}
	if strings.Contains(raw, "hmac") {
		t.Fatalf("hmac field in list %s", raw)
	}

	var listed struct {
		Webhooks []map[string]any `json:"webhooks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Webhooks) != 1 {
		t.Fatalf("want 1 got %d %s", len(listed.Webhooks), raw)
	}
	row := listed.Webhooks[0]
	if row["id"] != created.ID {
		t.Fatalf("id %v", row)
	}
	if row["token_prefix"] != created.TokenPrefix {
		t.Fatalf("prefix %v want %s", row, created.TokenPrefix)
	}
	if !strings.HasPrefix(created.Token, created.TokenPrefix) {
		t.Fatalf("prefix %q not prefix of token", created.TokenPrefix)
	}
	if _, ok := row["token"]; ok {
		t.Fatal("token field present")
	}
	if _, ok := row["hmac_key"]; ok {
		t.Fatal("hmac_key field present")
	}
	if len(row) != 2 {
		t.Fatalf("unexpected fields %v", row)
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

func TestChannelsAPI_LiteFlag(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	_, h := newTestServer()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/channels", nil))
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	var off struct {
		Lite     bool `json:"lite"`
		Channels []struct {
			Name       string `json:"name"`
			Configured bool   `json:"configured"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &off); err != nil {
		t.Fatal(err)
	}
	if off.Lite {
		t.Fatalf("lite unset should be false: %s", w.Body.String())
	}
	if len(off.Channels) != 7 {
		t.Fatalf("want 7 channels, got %d", len(off.Channels))
	}

	t.Setenv("GOSO_LITE", "1")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/channels", nil))
	if w.Code != 200 {
		t.Fatalf("lite status %d %s", w.Code, w.Body.String())
	}
	var on struct {
		Lite     bool `json:"lite"`
		Channels []struct {
			Name string `json:"name"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &on); err != nil {
		t.Fatal(err)
	}
	if !on.Lite {
		t.Fatalf("GOSO_LITE=1 want lite true: %s", w.Body.String())
	}
	if len(on.Channels) != 7 {
		t.Fatalf("lite still lists adapters, got %d", len(on.Channels))
	}
}
