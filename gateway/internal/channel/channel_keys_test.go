// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func sessionLabels(st store.StoreIface, agentID string) []string {
	var out []string
	for _, s := range st.ListSessions() {
		if s.AgentID == agentID {
			out = append(out, s.Label)
		}
	}
	sort.Strings(out)
	return out
}

func agentByKey(t *testing.T, st store.StoreIface, key string) *store.Agent {
	t.Helper()
	for _, a := range st.ListAgents() {
		if a.AgentKey == key {
			return a
		}
	}
	t.Fatalf("no agent %q", key)
	return nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A Telegram forum group has one session per topic (General is topic 1) and
// one user key for the whole group; each answer goes back into its topic.
func TestTelegramForumTopicsAreSessions(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	st := store.New()
	ag, _ := st.CreateAgent(store.Agent{AgentKey: "sales", DisplayName: "Sales", TenantID: "demo"})
	bot := "demo/telegram/minh-an-bot"
	if err := st.PutChannelConfig(store.ChannelConfig{Name: bot, Enabled: true, AgentID: ag.ID, DMPolicy: "open", GroupPolicy: "open"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeRuntime{st: st, res: TurnResult{Reply: "ok"}}
	useRuntime(t, f)
	var mu sync.Mutex
	var threads []int64
	tg := &Telegram{Instance: bot, Store: st, LLM: llm.Echo{}, Sender: func(ctx context.Context, _ int64, _ string) error {
		mu.Lock()
		defer mu.Unlock()
		threads = append(threads, telegramThreadFrom(ctx))
		return nil
	}}
	updates := []string{
		`{"update_id":1,"message":{"message_id":1,"message_thread_id":9,"is_topic_message":true,"chat":{"id":-100777,"type":"supergroup","is_forum":true},"from":{"id":4242},"text":"@bot giá gói A?"}}`,
		`{"update_id":2,"message":{"message_id":2,"message_thread_id":9,"is_topic_message":true,"chat":{"id":-100777,"type":"supergroup","is_forum":true},"from":{"id":4343},"text":"@bot còn hàng?"}}`,
		`{"update_id":3,"message":{"message_id":3,"chat":{"id":-100777,"type":"supergroup","is_forum":true},"from":{"id":4242},"text":"@bot chào cả nhà"}}`,
		`{"update_id":4,"message":{"message_id":4,"message_thread_id":5,"chat":{"id":4242,"type":"private"},"from":{"id":4242},"text":"hỏi riêng"}}`,
	}
	for _, raw := range updates {
		var u TelegramUpdate
		if err := json.Unmarshal([]byte(raw), &u); err != nil {
			t.Fatal(err)
		}
		if w := tg.ingest(context.Background(), u); w != "" {
			t.Fatalf("warning %q", w)
		}
	}
	want := []string{bot + ":-100777:topic:1", bot + ":-100777:topic:9", bot + ":4242:thread:5"}
	if got := sessionLabels(st, ag.ID); !sameStrings(got, want) {
		t.Fatalf("sessions %q, want %q", got, want)
	}
	if !sameStrings(f.userKey, []string{"group:minh-an-bot:-100777", "group:minh-an-bot:-100777", "group:minh-an-bot:-100777", "4242"}) {
		t.Fatalf("user keys %q", f.userKey)
	}
	if len(threads) != 4 || threads[0] != 9 || threads[1] != 9 || threads[2] != 0 || threads[3] != 5 {
		t.Fatalf("reply threads %v", threads)
	}
}

// The Bot API call carries message_thread_id only for a topic or thread.
func TestTelegramSendCarriesThread(t *testing.T) {
	var mu sync.Mutex
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	tg := &Telegram{BotToken: "tok-" + "demo", APIBase: srv.URL, HTTPClient: srv.Client()}
	if err := tg.sendMessage(withTelegramThread(context.Background(), 9), -100777, "a"); err != nil {
		t.Fatal(err)
	}
	if err := tg.sendMessage(context.Background(), -100777, "b"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["message_thread_id"] != float64(9) {
		t.Fatalf("calls %v", got)
	}
	if _, ok := got[1]["message_thread_id"]; ok {
		t.Fatalf("plain chat sent a thread: %v", got[1])
	}
}

// larkTopicEvent is a Lark group message inside the topic rooted at root.
func larkTopicEvent(t *testing.T, msgID, root, text string) []byte {
	t.Helper()
	var ev map[string]any
	if err := json.Unmarshal(larkEventJSON(msgID, "group", text, true), &ev); err != nil {
		t.Fatal(err)
	}
	msg := ev["event"].(map[string]any)["message"].(map[string]any)
	msg["root_id"] = root
	msg["thread_id"] = "omt_" + root
	b, _ := json.Marshal(ev)
	return b
}

// In topic_session_mode each Lark topic is its own session, keyed as GoClaw
// keyed it; a session imported from GoClaw for a topic is continued.
func TestLarkTopicSessionMode(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	st := larkStore(t, map[string]string{SettingAppID: larkAppID, SettingTopicSessionMode: "enabled"})
	agent := agentByKey(t, st, "minh-an")
	imported, _ := st.CreateSession(store.Session{AgentID: agent.ID, TenantID: "tenant-a", Label: "agent:minh-an:team:group:oc_group:topic:om_r1"})
	f := &fakeRuntime{st: st, res: TurnResult{Reply: "ok"}}
	useRuntime(t, f)
	api := newFakeLark(t)
	l := &Lark{Instance: larkA, Store: st, LLM: llm.Echo{}, APIBase: api.srv.URL, run: func(f func()) { f() }}
	if err := l.ProbeToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	l.dispatchEvent(larkTopicEvent(t, "m1", "om_r1", "báo giá"))
	l.dispatchEvent(larkTopicEvent(t, "m2", "om_r2", "đổi trả"))
	want := []string{"agent:minh-an:team:group:oc_group:topic:om_r1", larkA + ":oc_group:topic:om_r2"}
	if got := sessionLabels(st, agent.ID); !sameStrings(got, want) {
		t.Fatalf("sessions %q, want %q", got, want)
	}
	if len(f.session) != 2 || f.session[0] != imported.ID {
		t.Fatalf("turn sessions %q, want the imported one first", f.session)
	}
	if !sameStrings(f.userKey, []string{"group:team:oc_group:topic:om_r1", "group:team:oc_group:topic:om_r2"}) {
		t.Fatalf("user keys %q", f.userKey)
	}
	if replies, _, _ := api.got(); len(replies) != 2 {
		t.Fatalf("replies %v", replies)
	}
}

func postJSONTo(t *testing.T, h func(http.ResponseWriter, *http.Request), body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(b)))
	return w
}

// The legacy Discord webhook reads the author: a server member has GoClaw's
// guild user key; a bot's own message is not answered.
func TestDiscordWebhookIdentity(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	st := store.New()
	f := &fakeRuntime{st: st, res: TurnResult{Reply: "ok"}}
	useRuntime(t, f)
	var sent []string
	d := &Discord{Store: st, LLM: llm.Echo{}, Sender: func(_ context.Context, ch, _ string) error {
		sent = append(sent, ch)
		return nil
	}}
	postJSONTo(t, d.HandleUpdate, map[string]any{"channel_id": "c-1", "guild_id": "g-9", "content": "chào", "author": map[string]any{"id": "u-77"}})
	postJSONTo(t, d.HandleUpdate, map[string]any{"d": map[string]any{"channel_id": "dm-1", "content": "chào", "author": map[string]any{"id": "u-77"}}})
	postJSONTo(t, d.HandleUpdate, map[string]any{"channel_id": "c-1", "guild_id": "g-9", "content": "bot", "author": map[string]any{"id": "b-1", "bot": true}})
	if !sameStrings(f.userKey, []string{"guild:g-9:user:u-77", "u-77"}) {
		t.Fatalf("user keys %q", f.userKey)
	}
	if !sameStrings(sent, []string{"c-1", "dm-1"}) {
		t.Fatalf("sent %q", sent)
	}
}

// The legacy Slack webhook reads the sender, keeps a thread in its own
// session, answers inside that thread and skips bot messages.
func TestSlackWebhookIdentity(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	st := store.New()
	f := &fakeRuntime{st: st, res: TurnResult{Reply: "ok"}}
	useRuntime(t, f)
	var threads []string
	s := &Slack{Store: st, LLM: llm.Echo{}, Sender: func(ctx context.Context, _, _ string) error {
		threads = append(threads, slackThreadFrom(ctx))
		return nil
	}}
	ev := func(e map[string]any) map[string]any { return map[string]any{"type": "event_callback", "event": e} }
	postJSONTo(t, s.HandleUpdate, ev(map[string]any{"type": "message", "channel": "D1", "channel_type": "im", "user": "U1", "text": "hỏi"}))
	postJSONTo(t, s.HandleUpdate, ev(map[string]any{"type": "message", "channel": "C1", "channel_type": "channel", "user": "U1", "text": "hỏi", "thread_ts": "1700000000.000100"}))
	postJSONTo(t, s.HandleUpdate, ev(map[string]any{"type": "message", "channel": "C1", "channel_type": "channel", "bot_id": "B1", "text": "bot"}))
	if !sameStrings(f.userKey, []string{"U1", "group:slack:C1"}) {
		t.Fatalf("user keys %q", f.userKey)
	}
	if !sameStrings(threads, []string{"", "1700000000.000100"}) {
		t.Fatalf("threads %q", threads)
	}
	agent := agentByKey(t, st, "slack")
	if got := sessionLabels(st, agent.ID); !sameStrings(got, []string{"slack:C1:thread:1700000000.000100", "slack:D1"}) {
		t.Fatalf("sessions %q", got)
	}
}

// The legacy Feishu webhook reads the sender's open_id and the chat type.
func TestFeishuWebhookIdentity(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	st := store.New()
	f := &fakeRuntime{st: st, res: TurnResult{Reply: "ok"}}
	useRuntime(t, f)
	fs := &Feishu{Store: st, LLM: llm.Echo{}, Sender: func(context.Context, string, string) error { return nil }}
	ev := func(chat, chatType string) map[string]any {
		return map[string]any{
			"header": map[string]any{"event_type": "im.message.receive_v1"},
			"event": map[string]any{
				"sender":  map[string]any{"sender_id": map[string]any{"open_id": "ou_demo"}},
				"message": map[string]any{"chat_id": chat, "chat_type": chatType, "content": `{"text":"chào"}`},
			},
		}
	}
	postJSONTo(t, fs.HandleUpdate, ev("oc_p", "p2p"))
	postJSONTo(t, fs.HandleUpdate, ev("oc_g", "group"))
	if !sameStrings(f.userKey, []string{"ou_demo", "group:feishu:oc_g"}) {
		t.Fatalf("user keys %q", f.userKey)
	}
}

// A ZaloCRM conversation imported from GoClaw is continued, with GoClaw's
// user key.
func TestZCRMBridgeContinuesImportedSession(t *testing.T) {
	fx := newBridgeFixture(t)
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	imported, _ := fx.st.CreateSession(store.Session{AgentID: fx.agentA.ID, TenantID: "tenant-a", Label: "agent:minh-an:shop:direct:conv-1"})
	f := &fakeRuntime{st: fx.st, res: TurnResult{Reply: "ok"}}
	useRuntime(t, f)
	if w := fx.post(t, bridgeCall{text: "chào shop"}); w.Code != http.StatusAccepted {
		t.Fatalf("bridge call %d %s", w.Code, w.Body.String())
	}
	if len(f.session) != 1 || f.session[0] != imported.ID || f.userKey[0] != "u1" {
		t.Fatalf("sessions %q user keys %q", f.session, f.userKey)
	}
	cfg, _ := fx.st.GetChannelConfig(bridgeA)
	cfg.Settings[SettingGroupEnabled] = "true"
	_ = fx.st.PutChannelConfig(*cfg)
	if w := fx.post(t, bridgeCall{text: "nhóm", conv: "conv-g", thread: "group"}); w.Code != http.StatusAccepted {
		t.Fatalf("group call %d", w.Code)
	}
	if len(f.userKey) != 2 || f.userKey[1] != "group:shop:conv-g" {
		t.Fatalf("user keys %q", f.userKey)
	}
}

// slackSend posts into the thread and treats Slack's "ok": false as a failure.
func TestSlackSendThreadAndRefusal(t *testing.T) {
	var body map[string]any
	answer := `{"ok":true}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = nil
		_ = json.Unmarshal(b, &body)
		if r.URL.Path != "/chat.postMessage" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer tok-") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	tok := "tok-" + "slack"
	if err := slackSend(context.Background(), srv.Client(), srv.URL, tok, "C1", "1700000000.000100", "chào"); err != nil {
		t.Fatal(err)
	}
	if body["channel"] != "C1" || body["thread_ts"] != "1700000000.000100" || body["text"] != "chào" {
		t.Fatalf("body %v", body)
	}
	answer = `{"ok":false,"error":"channel_not_found"}`
	if err := slackSend(context.Background(), srv.Client(), srv.URL, tok, "C1", "", "chào"); err == nil {
		t.Fatal("ok:false accepted")
	}
	if _, ok := body["thread_ts"]; ok {
		t.Fatalf("plain post sent a thread: %v", body)
	}
}
