// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestTelegramSelfAuth(t *testing.T) {
	for _, tc := range []struct {
		method, path, secret string
		want                 bool
	}{
		{"POST", "/api/channels/telegram/webhook", "s", true},
		{"POST", "/api/channels/telegram/webhook/tenant-a/shop", "s", true},
		{"POST", "/api/channels/telegram/webhook", "", false},
		{"GET", "/api/channels/telegram/webhook", "s", false},
		{"POST", "/api/channels/telegram/webhookx", "s", false},
		{"POST", "/api/agents", "s", false},
		{"PUT", "/api/channels/telegram/secrets", "s", false},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.secret != "" {
			r.Header.Set(TelegramSecretHeader, tc.secret)
		}
		if got := TelegramSelfAuth(r); got != tc.want {
			t.Errorf("%s %s secret=%q: %v want %v", tc.method, tc.path, tc.secret, got, tc.want)
		}
	}
}

func webhookCall(t *testing.T, tg *Telegram, secret string) int {
	t.Helper()
	req := httptest.NewRequest("POST", tg.webhookPath(), bytes.NewBufferString(`{"update_id":1,"message":{"message_id":1,"chat":{"id":5},"from":{"id":5},"text":"hi"}}`))
	if secret != "" {
		req.Header.Set(TelegramSecretHeader, secret)
	}
	w := httptest.NewRecorder()
	tg.HandleUpdate(w, req)
	return w.Code
}

// A call with Telegram's secret header skipped the admin token, so it must
// carry this bot's own secret.
func TestTelegramWebhookSecretIsPerBot(t *testing.T) {
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	t.Setenv("GOSO_TELEGRAM_WEBHOOK_SECRET", "legacy-only")
	st := store.New()
	agentA, _ := st.CreateAgent(store.Agent{AgentKey: "a", DisplayName: "A", TenantID: "tenant-a"})
	agentB, _ := st.CreateAgent(store.Agent{AgentKey: "b", DisplayName: "B", TenantID: "tenant-b"})
	botA, botB, bare := "tenant-a/telegram/shop", "tenant-b/telegram/shop", "tenant-a/telegram/no-token"
	openBot(t, st, botA, agentA.ID)
	openBot(t, st, botB, agentB.ID)
	openBot(t, st, bare, agentA.ID)
	for _, n := range []string{botA, botB} {
		if err := secrets.Put(st, SecretName(n, KindBot), []byte("tok-"+n)); err != nil {
			t.Fatal(err)
		}
	}
	var outA sent
	tgA := &Telegram{Instance: botA, Store: st, LLM: llm.Echo{}, Sender: outA.fn}
	tgB := &Telegram{Instance: botB, Store: st, LLM: llm.Echo{}}
	tgBare := &Telegram{Instance: bare, Store: st, LLM: llm.Echo{}}

	secA := tgA.webhookSecret(tgA.resolveToken())
	secB := tgB.webhookSecret(tgB.resolveToken())
	if len(secA) != 64 || secA == secB {
		t.Fatalf("secrets A=%q B=%q", secA, secB)
	}
	if got := (&Telegram{Instance: botA}).webhookSecret("other-token"); got == secA {
		t.Fatal("secret does not follow the token")
	}
	if code := webhookCall(t, tgA, secA); code != http.StatusOK || outA.count() != 1 {
		t.Fatalf("own secret %d replies %d", code, outA.count())
	}
	for _, tc := range []struct {
		name   string
		tg     *Telegram
		secret string
	}{
		{"wrong secret", tgA, "nope"},
		{"other bot's secret", tgA, secB},
		{"bot without token", tgBare, tgBare.webhookSecret("")},
		{"bot without token, guessed", tgBare, secA},
	} {
		if code := webhookCall(t, tc.tg, tc.secret); code != http.StatusUnauthorized {
			t.Errorf("%s: %d", tc.name, code)
		}
	}
	if outA.count() != 1 {
		t.Fatal("a refused call reached the agent")
	}
}

func TestWebhookModeGivesTelegramTheSecret(t *testing.T) {
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	t.Setenv("GOSO_LITE", "")
	t.Setenv("GOSO_TELEGRAM_MODE", "webhook")
	t.Setenv("GOSO_PUBLIC_URL", "https://gw.example/")
	st := store.New()
	bot := "tenant-a/telegram/shop"
	if err := secrets.Put(st, SecretName(bot, KindBot), []byte("tok-a")); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var hook map[string]string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bottok-a/setWebhook" {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			_ = json.Unmarshal(b, &hook)
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer api.Close()
	m := NewManager()
	tg := &Telegram{Instance: bot, Store: st, APIBase: api.URL}
	tg.startLive(context.Background(), m)
	mu.Lock()
	defer mu.Unlock()
	if hook["url"] != "https://gw.example/api/channels/telegram/webhook/tenant-a/shop" {
		t.Fatalf("webhook url %q", hook["url"])
	}
	if hook["secret_token"] == "" || hook["secret_token"] != tg.webhookSecret("tok-a") {
		t.Fatal("setWebhook did not send the bot's secret_token")
	}
	if !m.Running(bot) || m.Transport(bot) != "webhook" {
		t.Fatalf("running=%v transport=%q", m.Running(bot), m.Transport(bot))
	}
}
