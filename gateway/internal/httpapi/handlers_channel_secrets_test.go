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

	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestPutChannelSecrets_WriteOnly(t *testing.T) {
	const leak = "088-bot-token-must-not-leak"
	st := store.New()
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("GOSO_LITE", "")
	h := NewRouter(Options{Store: st, Version: "t"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/channels/telegram/secrets", bytes.NewBufferString(`{"bot_token":"`+leak+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("put %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), leak) {
		t.Fatalf("PUT echoed token: %s", w.Body.String())
	}
	var put struct {
		OK        bool `json:"ok"`
		SecretSet bool `json:"secret_set"`
		FromEnv   bool `json:"from_env"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &put); err != nil {
		t.Fatal(err)
	}
	if !put.OK || !put.SecretSet || put.FromEnv {
		t.Fatalf("put body %+v %s", put, w.Body.String())
	}

	gw := httptest.NewRecorder()
	h.ServeHTTP(gw, httptest.NewRequest("GET", "/api/channels", nil))
	if strings.Contains(gw.Body.String(), leak) {
		t.Fatalf("GET leaked: %s", gw.Body.String())
	}
	assertNoTokenLikeValues(t, gw.Body.String())
	var body struct {
		Channels []struct {
			Name      string   `json:"name"`
			SecretSet bool     `json:"secret_set"`
			FromEnv   bool     `json:"from_env"`
			Writable  []string `json:"writable"`
			Missing   bool     `json:"missing"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(gw.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range body.Channels {
		if c.Name != "telegram" {
			continue
		}
		found = true
		if !c.SecretSet || c.FromEnv || c.Missing {
			t.Fatalf("telegram %+v", c)
		}
		if len(c.Writable) != 1 || c.Writable[0] != "bot_token" {
			t.Fatalf("writable %v", c.Writable)
		}
	}
	if !found {
		t.Fatal("telegram row")
	}

	wKeep := httptest.NewRecorder()
	reqKeep := httptest.NewRequest("PUT", "/api/channels/telegram/secrets", bytes.NewBufferString(`{"bot_token":""}`))
	reqKeep.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(wKeep, reqKeep)
	if wKeep.Code != 400 {
		t.Fatalf("blank put %d %s", wKeep.Code, wKeep.Body.String())
	}

	wPatch := httptest.NewRecorder()
	reqPatch := httptest.NewRequest("PATCH", "/api/channels/telegram", bytes.NewBufferString(`{"bot_token":"`+leak+`"}`))
	reqPatch.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(wPatch, reqPatch)
	if wPatch.Code != 400 {
		t.Fatalf("PATCH still writes? %d %s", wPatch.Code, wPatch.Body.String())
	}

	wPark := httptest.NewRecorder()
	reqPark := httptest.NewRequest("PUT", "/api/channels/discord/secrets", bytes.NewBufferString(`{"bot_token":"x"}`))
	reqPark.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(wPark, reqPark)
	if wPark.Code != 409 {
		t.Fatalf("discord put %d %s", wPark.Code, wPark.Body.String())
	}

	wQR := httptest.NewRecorder()
	reqQR := httptest.NewRequest("PUT", "/api/channels/zalo-personal/secrets", bytes.NewBufferString(`{"bot_token":"x"}`))
	reqQR.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(wQR, reqQR)
	if wQR.Code != 400 {
		t.Fatalf("personal put %d %s", wQR.Code, wQR.Body.String())
	}
}

func TestPutChannelSecrets_NeedsMasterKey(t *testing.T) {
	st := store.New()
	t.Setenv("GOSO_MASTER_KEY", "")
	t.Setenv("GOSO_LITE", "")
	h := NewRouter(Options{Store: st, Version: "t"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/channels/telegram/secrets", bytes.NewBufferString(`{"bot_token":"abc"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("no master key %d %s", w.Code, w.Body.String())
	}
}

func TestPutChannelSecrets_EnvWinsFlag(t *testing.T) {
	st := store.New()
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "from-env-token")
	t.Setenv("GOSO_LITE", "")
	h := NewRouter(Options{Store: st, Version: "t"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/channels/telegram/secrets", bytes.NewBufferString(`{"bot_token":"from-box-token"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("put %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "from-env-token") || strings.Contains(w.Body.String(), "from-box-token") {
		t.Fatalf("leaked %s", w.Body.String())
	}
	v, fromEnv, set := channel.Credential(st, "telegram", channel.KindBot, []string{"GOSO_TELEGRAM_BOT_TOKEN"})
	if !set || !fromEnv || v != "from-env-token" {
		t.Fatalf("env wins %q %v %v", v, fromEnv, set)
	}
}

func TestPutChannelSecrets_OA(t *testing.T) {
	st := store.New()
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	t.Setenv("GOSO_ZALO_OA_ACCESS_TOKEN", "")
	t.Setenv("GOSO_ZALO_OA_SECRET", "")
	t.Setenv("GOSO_LITE", "")
	h := NewRouter(Options{Store: st, Version: "t"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/channels/zalo-oa/secrets", bytes.NewBufferString(`{"access_token":"oa-access","app_secret":"oa-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("oa put %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "oa-access") || strings.Contains(w.Body.String(), "oa-secret") {
		t.Fatalf("leaked %s", w.Body.String())
	}
	gw := httptest.NewRecorder()
	h.ServeHTTP(gw, httptest.NewRequest("GET", "/api/channels", nil))
	if strings.Contains(gw.Body.String(), "oa-access") || strings.Contains(gw.Body.String(), "oa-secret") {
		t.Fatalf("GET leaked %s", gw.Body.String())
	}
	var body struct {
		Channels []struct {
			Name      string `json:"name"`
			SecretSet bool   `json:"secret_set"`
			Missing   bool   `json:"missing"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(gw.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, c := range body.Channels {
		if c.Name == "zalo-oa" && (!c.SecretSet || c.Missing) {
			t.Fatalf("oa %+v", c)
		}
	}
}

func TestPostChannelTest_TelegramGetMe(t *testing.T) {
	st := store.New()
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("GOSO_LITE", "")
	if err := secrets.Put(st, channel.SecretName("telegram", channel.KindBot), []byte("boxed")); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/botboxed/getMe") {
			w.WriteHeader(404)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)
	mgr := channel.NewManager()
	mgr.Telegram = &channel.Telegram{Store: st, APIBase: srv.URL, HTTPClient: srv.Client()}
	h := NewRouter(Options{Store: st, Version: "t", Channels: mgr})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/channels/telegram/test", nil))
	if w.Code != 200 {
		t.Fatalf("test %d %s", w.Code, w.Body.String())
	}
	if !mgr.Running("telegram") {
		t.Fatalf("health %s", mgr.LastError("telegram"))
	}
	if strings.Contains(w.Body.String(), "boxed") {
		t.Fatalf("leaked %s", w.Body.String())
	}

	wPark := httptest.NewRecorder()
	h.ServeHTTP(wPark, httptest.NewRequest("POST", "/api/channels/discord/test", nil))
	if wPark.Code != 409 {
		t.Fatalf("discord test %d", wPark.Code)
	}
}
