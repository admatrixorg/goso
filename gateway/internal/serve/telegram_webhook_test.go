// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package serve

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Telegram cannot send the admin token. Its webhook call gets past the
// gateway's token check only with the bot's own secret; nothing else does.
func TestTelegramWebhookPassesAuthOnlyWithBotSecret(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_DEV_MODE", "")
	t.Setenv("GOSO_ADMIN_TOKEN", "adm")
	t.Setenv("GOSO_LITE", "1")
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "tok-legacy")
	t.Setenv("GOSO_TELEGRAM_WEBHOOK_SECRET", "")
	h, _ := New(store.New(), "test")

	mac := hmac.New(sha256.New, []byte("tok-legacy"))
	mac.Write([]byte("goso telegram webhook\x00telegram"))
	secret := hex.EncodeToString(mac.Sum(nil))

	call := func(path, tgSecret, bearer string) int {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		if tgSecret != "" {
			req.Header.Set(channel.TelegramSecretHeader, tgSecret)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}
	const hook = "/api/channels/telegram/webhook"
	for _, tc := range []struct {
		name, path, secret, bearer string
		want                       int
	}{
		{"telegram with the bot secret", hook, secret, "", http.StatusOK},
		{"no secret, no token", hook, "", "", http.StatusUnauthorized},
		{"wrong secret", hook, "guess", "", http.StatusUnauthorized},
		{"admin token still works", hook, "", "adm", http.StatusOK},
		{"secret does not open other routes", "/api/agents", secret, "", http.StatusUnauthorized},
		{"tenant bot not running", hook + "/tenant-a/shop", secret, "", http.StatusNotFound},
	} {
		if got := call(tc.path, tc.secret, tc.bearer); got != tc.want {
			t.Errorf("%s: %d want %d", tc.name, got, tc.want)
		}
	}
}
