// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// fakeZaloBotAPI is bot-api.zaloplatforms.com for one bot token. getUpdates
// hands out the queued events one at a time, then answers 408 as the platform
// does when nothing arrived.
type fakeZaloBotAPI struct {
	srv   *httptest.Server
	token string

	mu      sync.Mutex
	calls   []string
	queue   []string
	sent    []map[string]any
	webhook map[string]any
}

func newFakeZaloBotAPI(t *testing.T, token string) *fakeZaloBotAPI {
	t.Helper()
	f := &fakeZaloBotAPI{token: token}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		prefix := "/bot" + f.token + "/"
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, prefix) {
			_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
			return
		}
		method := strings.TrimPrefix(r.URL.Path, prefix)
		f.mu.Lock()
		f.calls = append(f.calls, method)
		var next string
		switch method {
		case "getUpdates":
			if len(f.queue) > 0 {
				next, f.queue = f.queue[0], f.queue[1:]
			}
		case "sendMessage":
			f.sent = append(f.sent, in)
		case "setWebhook":
			f.webhook = in
		}
		f.mu.Unlock()
		switch method {
		case "getMe":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":"b1","account_name":"bot.minhan","account_type":"BASIC","can_join_groups":false}}`))
		case "getUpdates":
			if next == "" {
				_, _ = w.Write([]byte(`{"ok":false,"error_code":408,"description":"Request timeout"}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":` + next + `}`))
		case "sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":"m-out","date":1750316131602}}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":{"url":""}}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeZaloBotAPI) snapshot() (calls []string, sent []map[string]any, webhook map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...), append([]map[string]any(nil), f.sent...), f.webhook
}

func zaloEvent(msgID, chatID, chatType, text string) string {
	b, _ := json.Marshal(map[string]any{
		"event_name": "message.text.received",
		"message": map[string]any{
			"from":       map[string]any{"id": "u-" + chatID, "display_name": "Khách", "is_bot": false},
			"chat":       map[string]any{"id": chatID, "chat_type": chatType},
			"text":       text,
			"message_id": msgID,
			"date":       1750316131602,
		},
	})
	return string(b)
}

const zaloBotA = "tenant-a/zalo-oa/shop"

// zaloBotStore has tenant-a's Zalo Bot, enabled, open to direct messages and
// bound to tenant-a's agent.
func zaloBotStore(t *testing.T, token string) (store.StoreIface, *store.Agent) {
	t.Helper()
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	st := store.New()
	agent, _ := st.CreateAgent(store.Agent{AgentKey: "minh-an", DisplayName: "Minh An", TenantID: "tenant-a"})
	if err := st.PutChannelConfig(store.ChannelConfig{Name: zaloBotA, Enabled: true, AgentID: agent.ID, DMPolicy: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Put(st, SecretName(zaloBotA, KindBot), []byte(token)); err != nil {
		t.Fatal(err)
	}
	return st, agent
}

func zaloWebhookCall(z *ZaloBot, secret, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, z.webhookPath(), strings.NewReader(body))
	if secret != "" {
		req.Header.Set(ZaloBotSecretHeader, secret)
	}
	w := httptest.NewRecorder()
	z.HandleUpdate(w, req)
	return w
}

func TestZaloBotWebhookAnswersOnce(t *testing.T) {
	st, agent := zaloBotStore(t, "tok-a")
	var sent []string
	z := &ZaloBot{Instance: zaloBotA, Store: st, LLM: llm.Echo{}, run: func(f func()) { f() }}
	z.Sender = func(_ context.Context, chatID, text string) error {
		sent = append(sent, chatID+"|"+text)
		return nil
	}
	secret := z.webhookSecret("tok-a")
	ev := `{"ok":true,"result":` + zaloEvent("m1", "c1", "PRIVATE", "xin chào") + `}`

	if w := zaloWebhookCall(z, "", ev); w.Code != http.StatusUnauthorized {
		t.Fatalf("no secret %d", w.Code)
	}
	if w := zaloWebhookCall(z, "guess", ev); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong secret %d", w.Code)
	}
	if w := zaloWebhookCall(z, secret, ev); w.Code != http.StatusOK {
		t.Fatalf("webhook %d", w.Code)
	}
	// The platform sending the same message again gets no second answer.
	_ = zaloWebhookCall(z, secret, ev)
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "c1|") || !strings.Contains(sent[0], "xin chào") {
		t.Fatalf("sent %v", sent)
	}
	var found bool
	for _, s := range st.ListSessions() {
		if s.AgentID == agent.ID && s.Label == zaloBotA+":c1" {
			found = true
		}
	}
	if !found {
		t.Fatal("no session for the chat")
	}
}

func TestZaloBotStaysQuietWhenItShould(t *testing.T) {
	st, _ := zaloBotStore(t, "tok-a")
	var sent []string
	z := &ZaloBot{Instance: zaloBotA, Store: st, LLM: llm.Echo{}, run: func(f func()) { f() }}
	z.Sender = func(_ context.Context, _, text string) error {
		sent = append(sent, text)
		return nil
	}
	secret := z.webhookSecret("tok-a")
	// Groups are off by default.
	_ = zaloWebhookCall(z, secret, zaloEvent("g1", "grp", "GROUP", "@bot hỏi"))
	// Other events (stickers, images) are not answered.
	_ = zaloWebhookCall(z, secret, strings.Replace(zaloEvent("s1", "c1", "PRIVATE", "x"), "message.text.received", "message.sticker.received", 1))
	// A model error never reaches the customer.
	z.LLM = failingLLM{}
	_ = zaloWebhookCall(z, secret, zaloEvent("e1", "c1", "PRIVATE", "lỗi"))
	z.LLM = llm.Echo{}
	// A disabled channel does not answer.
	cfg, _ := st.GetChannelConfig(zaloBotA)
	cfg.Enabled = false
	_ = st.PutChannelConfig(*cfg)
	_ = zaloWebhookCall(z, secret, zaloEvent("d1", "c1", "PRIVATE", "tắt"))
	if len(sent) != 0 {
		t.Fatalf("sent %v, want none", sent)
	}
}

// The manager runs an enabled tenant Zalo Bot by polling, as GoClaw did, and
// stops it when the channel is turned off.
func TestZaloBotPollsThroughManager(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	t.Setenv("GOSO_ZALO_BOT_MODE", "")
	st, _ := zaloBotStore(t, "tok-a")
	api := newFakeZaloBotAPI(t, "tok-a")
	api.queue = []string{zaloEvent("p1", "c9", "PRIVATE", "đơn hàng")}
	m := NewManager()
	m.Store = st
	m.newZaloBot = func(name string) *ZaloBot {
		return &ZaloBot{Instance: name, Store: st, LLM: llm.Echo{}, APIBase: api.srv.URL}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.StartAll(ctx)
	defer m.StopAll()

	waitFor(t, "reply sent", func() bool { _, sent, _ := api.snapshot(); return len(sent) == 1 })
	calls, sent, _ := api.snapshot()
	if calls[0] != "getMe" || calls[1] != "deleteWebhook" {
		t.Fatalf("calls %v", calls)
	}
	if sent[0]["chat_id"] != "c9" || !strings.Contains(sent[0]["text"].(string), "đơn hàng") {
		t.Fatalf("sent %v", sent[0])
	}
	if !m.Running(zaloBotA) || m.Transport(zaloBotA) != "poll" || m.ZaloBot(zaloBotA) == nil {
		t.Fatal("bot not shown running")
	}
	cfg, _ := st.GetChannelConfig(zaloBotA)
	cfg.Enabled = false
	_ = st.PutChannelConfig(*cfg)
	m.Reconcile()
	if m.ZaloBot(zaloBotA) != nil || m.Running(zaloBotA) {
		t.Fatal("disabled bot still running")
	}
}

func TestZaloBotWebhookModeRegistersSecret(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	t.Setenv("GOSO_ZALO_BOT_MODE", "webhook")
	t.Setenv("GOSO_PUBLIC_URL", "https://goso.example.test/")
	st, _ := zaloBotStore(t, "tok-a")
	api := newFakeZaloBotAPI(t, "tok-a")
	m := NewManager()
	z := &ZaloBot{Instance: zaloBotA, Store: st, LLM: llm.Echo{}, APIBase: api.srv.URL}
	z.Start(context.Background(), m)
	_, _, hook := api.snapshot()
	if hook["url"] != "https://goso.example.test/api/channels/zalo-oa/bot/tenant-a/shop" || hook["secret_token"] != z.webhookSecret("tok-a") {
		t.Fatalf("setWebhook %v", hook)
	}
	if !m.Running(zaloBotA) || m.Transport(zaloBotA) != "webhook" {
		t.Fatal("not running as webhook")
	}
}

func TestZaloBotBadTokenFailsWithoutLeakingIt(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	t.Setenv("GOSO_ZALO_BOT_MODE", "")
	st, _ := zaloBotStore(t, "tok-wrong")
	api := newFakeZaloBotAPI(t, "tok-a")
	m := NewManager()
	z := &ZaloBot{Instance: zaloBotA, Store: st, APIBase: api.srv.URL}
	z.Start(context.Background(), m)
	if !m.Failed(zaloBotA) || strings.Contains(m.LastError(zaloBotA), "tok-wrong") {
		t.Fatalf("failed=%v err=%q", m.Failed(zaloBotA), m.LastError(zaloBotA))
	}
}

func TestZaloBotChannelFields(t *testing.T) {
	if got := WritableFields(zaloBotA); len(got) != 1 || got[0] != "bot_token" {
		t.Fatalf("tenant zalo-oa writable %v", got)
	}
	if k, ok := FieldKind(zaloBotA, "bot_token"); !ok || k != KindBot {
		t.Fatalf("bot_token kind %q %v", k, ok)
	}
	if _, ok := FieldKind(zaloBotA, "access_token"); ok {
		t.Fatal("tenant zalo-oa took an OA access token")
	}
	if got := WritableFields("zalo-oa"); len(got) != 2 {
		t.Fatalf("legacy zalo-oa writable %v", got)
	}
	parts := splitRunes(strings.Repeat("a ", 1500), zaloBotMaxText)
	if len(parts) != 2 || len([]rune(parts[0])) > zaloBotMaxText {
		t.Fatalf("split %d parts, first %d", len(parts), len([]rune(parts[0])))
	}
}

func TestZaloBotSelfAuth(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		header       bool
		want         bool
	}{
		{http.MethodPost, "/api/channels/zalo-oa/bot/tenant-a/shop", true, true},
		{http.MethodPost, "/api/channels/zalo-oa/bot/tenant-a/shop", false, false},
		{http.MethodGet, "/api/channels/zalo-oa/bot/tenant-a/shop", true, false},
		{http.MethodPost, "/api/channels/zalo-oa/webhook", true, false},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.header {
			r.Header.Set(ZaloBotSecretHeader, "x")
		}
		if got := ZaloBotSelfAuth(r); got != tc.want {
			t.Fatalf("%s %s header=%v: %v", tc.method, tc.path, tc.header, got)
		}
	}
}
