// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestParseInstance(t *testing.T) {
	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{"demo/telegram/minh-an-bot", true},
		{"demo/zalo-personal/minh-an-7697c8b5", true},
		{"telegram", false},
		{"master/telegram", false},
		{"master/unknown/x", false},
		{"bad tenant/telegram/x", false},
		{"master/telegram/a b", false},
		{"master/telegram/..", false},
		{"master/telegram/a%2Fb", false},
		{"a/b/c/d", false},
	} {
		if _, _, _, ok := ParseInstance(tc.name); ok != tc.ok {
			t.Errorf("%q: ok=%v want %v", tc.name, ok, tc.ok)
		}
	}
	if CatalogOf("demo/telegram/minh-an-bot") != "telegram" || CatalogOf("zalo-oa") != "zalo-oa" || CatalogOf("x/y") != "" {
		t.Fatal("CatalogOf")
	}
	if !KnownName("demo/telegram/minh-an-bot") || !KnownName("telegram") || KnownName("master/telegram") {
		t.Fatal("KnownName")
	}
}

func tgUpdate(t *testing.T, chatID int64, text string) TelegramUpdate {
	t.Helper()
	var u TelegramUpdate
	raw := fmt.Sprintf(`{"update_id":1,"message":{"message_id":1,"chat":{"id":%d},"from":{"id":%d},"text":%q}}`, chatID, chatID, text)
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatal(err)
	}
	return u
}

type sent struct {
	mu   sync.Mutex
	msgs []string
}

func (s *sent) fn(ctx context.Context, chatID int64, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, text)
	return nil
}

func (s *sent) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.msgs)
}

func openBot(t *testing.T, st store.StoreIface, name, agentID string) {
	t.Helper()
	if err := st.PutChannelConfig(store.ChannelConfig{Name: name, Enabled: true, AgentID: agentID, DMPolicy: "open"}); err != nil {
		t.Fatal(err)
	}
}

// Two tenants each pair a Telegram bot. A message to a bot is answered by the
// agent of the tenant that owns that bot, in a session of that tenant.
func TestTwoTenantsTwoTelegramBots(t *testing.T) {
	st := store.New()
	agentA, err := st.CreateAgent(store.Agent{AgentKey: "sales", DisplayName: "Sales A", TenantID: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	agentB, err := st.CreateAgent(store.Agent{AgentKey: "sales-b", DisplayName: "Sales B", TenantID: "tenant-b"})
	if err != nil {
		t.Fatal(err)
	}
	botA, botB := "tenant-a/telegram/shop-bot", "tenant-b/telegram/shop-bot"
	openBot(t, st, botA, agentA.ID)
	openBot(t, st, botB, agentB.ID)

	var outA, outB sent
	tgA := &Telegram{Instance: botA, Store: st, LLM: llm.Echo{}, Sender: outA.fn}
	tgB := &Telegram{Instance: botB, Store: st, LLM: llm.Echo{}, Sender: outB.fn}
	if w := tgA.ingest(context.Background(), tgUpdate(t, 42, "hello A")); w != "" {
		t.Fatalf("bot A warning %q", w)
	}
	if w := tgB.ingest(context.Background(), tgUpdate(t, 42, "hello B")); w != "" {
		t.Fatalf("bot B warning %q", w)
	}
	if outA.count() != 1 || outB.count() != 1 {
		t.Fatalf("replies A=%d B=%d", outA.count(), outB.count())
	}
	byLabel := map[string]*store.Session{}
	for _, s := range st.ListSessions() {
		byLabel[s.Label] = s
	}
	sa, sb := byLabel[botA+":42"], byLabel[botB+":42"]
	if sa == nil || sa.AgentID != agentA.ID || !store.SameTenant(sa.TenantID, "tenant-a") {
		t.Fatalf("bot A session %+v", sa)
	}
	if sb == nil || sb.AgentID != agentB.ID || !store.SameTenant(sb.TenantID, "tenant-b") {
		t.Fatalf("bot B session %+v", sb)
	}
	msgs, _ := st.ListMessages(sa.ID)
	if len(msgs) != 2 || msgs[0].Content != "hello A" {
		t.Fatalf("bot A thread %+v", msgs)
	}
	for _, a := range st.ListAgents() {
		if a.AgentKey == "telegram" {
			t.Fatal("a tenant bot created the shared telegram agent")
		}
	}
}

func TestTenantBotRefusesForeignOrMissingAgent(t *testing.T) {
	st := store.New()
	foreign, err := st.CreateAgent(store.Agent{AgentKey: "spy", DisplayName: "Spy", TenantID: "tenant-b"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		cfg     *store.ChannelConfig
		warning string
	}{
		{"other tenant's agent", &store.ChannelConfig{Enabled: true, AgentID: foreign.ID, DMPolicy: "open"}, "no agent bound"},
		{"no agent", &store.ChannelConfig{Enabled: true, DMPolicy: "open"}, "no agent bound"},
		{"disabled", &store.ChannelConfig{Enabled: false, AgentID: foreign.ID, DMPolicy: "open"}, ""},
		{"no row", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := "tenant-a/telegram/" + strings.ReplaceAll(tc.name, " ", "-")
			if tc.cfg != nil {
				cfg := *tc.cfg
				cfg.Name = name
				if err := st.PutChannelConfig(cfg); err != nil {
					t.Fatal(err)
				}
			}
			var out sent
			tg := &Telegram{Instance: name, Store: st, LLM: llm.Echo{}, Sender: out.fn}
			w := tg.ingest(context.Background(), tgUpdate(t, 7, "hi"))
			if !strings.HasPrefix(w, tc.warning) || out.count() != 0 {
				t.Fatalf("warning %q replies %d", w, out.count())
			}
		})
	}
	if len(st.ListSessions()) != 0 {
		t.Fatal("a refused bot opened a session")
	}
}

// The older single bot bound to a tenant's agent used to fail: its session had
// no tenant, the store refused it, and the turn crashed on a nil session.
func TestLegacyBotBoundToTenantAgent(t *testing.T) {
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "")
	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "cskh", DisplayName: "CSKH", TenantID: "master"})
	if err != nil {
		t.Fatal(err)
	}
	openBot(t, st, "telegram", a.ID)
	var out sent
	tg := &Telegram{Store: st, LLM: llm.Echo{}, Sender: out.fn}
	if w := tg.ingest(context.Background(), tgUpdate(t, 9, "xin chao")); w != "" || out.count() != 1 {
		t.Fatalf("warning %q replies %d", w, out.count())
	}
	for _, s := range st.ListSessions() {
		if s.Label == "telegram:9" && store.SameTenant(s.TenantID, "master") {
			return
		}
	}
	t.Fatal("session missing or in the wrong tenant")
}

func fakeTelegramAPI(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/getUpdates") {
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestManagerReconcileRunsEnabledTenantBots(t *testing.T) {
	t.Setenv("GOSO_TELEGRAM_MODE", "")
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	st := store.New()
	agentA, _ := st.CreateAgent(store.Agent{AgentKey: "a", DisplayName: "A", TenantID: "tenant-a"})
	agentB, _ := st.CreateAgent(store.Agent{AgentKey: "b", DisplayName: "B", TenantID: "tenant-b"})
	botA, botB, off := "tenant-a/telegram/one", "tenant-b/telegram/two", "tenant-b/telegram/off"
	openBot(t, st, botA, agentA.ID)
	openBot(t, st, botB, agentB.ID)
	if err := st.PutChannelConfig(store.ChannelConfig{Name: off, AgentID: agentB.ID}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{botA, botB, off} {
		if err := secrets.Put(st, SecretName(n, KindBot), []byte("tok-"+n)); err != nil {
			t.Fatal(err)
		}
	}
	api := fakeTelegramAPI(t)
	m := NewManager()
	m.Store = st
	m.newTelegram = func(name string) *Telegram {
		return &Telegram{Instance: name, Store: st, LLM: llm.Echo{}, APIBase: api.URL}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.StartAll(ctx)
	defer m.StopAll()

	waitFor(t, "both bots running", func() bool { return m.Running(botA) && m.Running(botB) })
	if m.TelegramBot(off) != nil || m.Running(off) {
		t.Fatal("disabled bot started")
	}
	if m.TelegramBot(botA).resolveToken() != "tok-"+botA {
		t.Fatal("bot A uses another bot's token")
	}

	cfg, _ := st.GetChannelConfig(botA)
	cfg.Enabled = false
	if err := st.PutChannelConfig(*cfg); err != nil {
		t.Fatal(err)
	}
	m.Reconcile()
	if m.TelegramBot(botA) != nil || m.Running(botA) {
		t.Fatal("disabled bot still running")
	}
	if m.TelegramBot(botB) == nil {
		t.Fatal("other bot stopped")
	}
	if InstanceInfo(st, m, botA).Health != "stopped" || InstanceInfo(st, m, botB).Health != "running" {
		t.Fatalf("health A=%s B=%s", InstanceInfo(st, m, botA).Health, InstanceInfo(st, m, botB).Health)
	}
}

func TestTenantBotIgnoresEnvToken(t *testing.T) {
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "shared-env-token")
	tg := &Telegram{Instance: "tenant-a/telegram/x", Store: store.New()}
	if tg.resolveToken() != "" {
		t.Fatal("tenant bot fell back to the shared env token")
	}
	if (&Telegram{Store: store.New()}).resolveToken() != "shared-env-token" {
		t.Fatal("legacy bot lost its env token")
	}
	if got := tg.webhookPath(); got != "/api/channels/telegram/webhook/tenant-a/x" {
		t.Fatalf("webhook path %q", got)
	}
}
