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
		if strings.Contains(r.URL.Path, "setWebhook") || strings.Contains(r.URL.Path, "getUpdates") {
			_, _ = io.WriteString(w, `{"ok":true,"result":[]}`)
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

	t.Setenv("GOSO_PUBLIC_URL", "https://example.test")
	mgr3 := NewManager()
	tg3 := &Telegram{BotToken: "tok", APIBase: srv.URL, HTTPClient: srv.Client(), probeEvery: time.Hour}
	tg3.Start(context.Background(), mgr3)
	t.Cleanup(tg3.Stop)
	if !mgr3.Running("telegram") {
		t.Fatalf("webhook with public url running=%v err=%s", mgr3.Running("telegram"), mgr3.LastError("telegram"))
	}
}

func TestTelegram_StartZeroProbeDoesNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"result":[]}`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GOSO_LITE", "")
	t.Setenv("GOSO_TELEGRAM_MODE", "poll")
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "tok")
	tg := &Telegram{BotToken: "tok", APIBase: srv.URL, HTTPClient: srv.Client()}
	mgr := NewManager()
	tg.Start(context.Background(), mgr)
	t.Cleanup(tg.Stop)
	if !mgr.Running("telegram") {
		t.Fatalf("running err=%s", mgr.LastError("telegram"))
	}
}

func TestTelegram_PairingSendsCode(t *testing.T) {
	t.Setenv("GOSO_ENV", "production")
	t.Setenv("GOSO_TELEGRAM_WEBHOOK_SECRET", "")
	var sent []string
	st := store.New()
	tg := &Telegram{Store: st, Sender: func(_ context.Context, _ int64, text string) error {
		sent = append(sent, text)
		return nil
	}}
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"message":{"chat":{"id":9},"from":{"id":991},"text":"hello"}}`))
	w := httptest.NewRecorder()
	tg.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body.String())
	}
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "Pairing code: ") || len(sent[0]) < 20 {
		t.Fatalf("sent %v", sent)
	}
	rows := st.ListChannelPairings()
	if len(rows) != 1 || rows[0].Status != "pending" || rows[0].SenderID != "991" || rows[0].Channel != "telegram" {
		t.Fatalf("pending row %+v", rows)
	}
	if rows[0].CodeHash == "" || strings.Contains(sent[0], rows[0].CodeHash) {
		t.Fatalf("hash %+v sent %q", rows[0], sent[0])
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
