// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestTelegram_StartGetMeAndWebhookURL(t *testing.T) {
	var gotMe int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "getMe") {
			gotMe++
			_, _ = io.WriteString(w, `{"ok":true}`)
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GOSO_LITE", "")
	t.Setenv("GOSO_TELEGRAM_MODE", "poll")
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "tok")
	t.Setenv("GOSO_PUBLIC_URL", "")
	tg := &Telegram{BotToken: "tok", APIBase: srv.URL, HTTPClient: srv.Client(), probeEvery: time.Hour}
	mgr := NewManager()
	tg.Start(context.Background(), mgr)
	t.Cleanup(tg.Stop)
	if gotMe < 1 {
		t.Fatal("getMe")
	}
	if !mgr.Running("telegram") {
		t.Fatalf("running err=%s", mgr.LastError("telegram"))
	}

	t.Setenv("GOSO_TELEGRAM_MODE", "webhook")
	mgr2 := NewManager()
	tg2 := &Telegram{BotToken: "tok", APIBase: srv.URL, HTTPClient: srv.Client()}
	tg2.Start(context.Background(), mgr2)
	if !mgr2.Failed("telegram") || !strings.Contains(mgr2.LastError("telegram"), "public url required") {
		t.Fatalf("want failed public url got running=%v err=%s", mgr2.Running("telegram"), mgr2.LastError("telegram"))
	}
}

func TestTelegram_WebhookSecret(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_TELEGRAM_WEBHOOK_SECRET", "s3cret")
	tg := &Telegram{Store: store.New(), Sender: func(context.Context, int64, string) error { return nil }}
	req := httptest.NewRequest("POST", "/api/channels/telegram/webhook", strings.NewReader(`{"message":{"chat":{"id":1},"text":"hi"}}`))
	w := httptest.NewRecorder()
	tg.HandleUpdate(w, req)
	if w.Code != 401 {
		t.Fatalf("missing secret %d", w.Code)
	}
	req2 := httptest.NewRequest("POST", "/api/channels/telegram/webhook", strings.NewReader(`{"message":{"chat":{"id":1},"text":"hi"}}`))
	req2.Header.Set("X-Goso-Telegram-Secret", "s3cret")
	w2 := httptest.NewRecorder()
	tg.HandleUpdate(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("ok secret %d %s", w2.Code, w2.Body.String())
	}
}
