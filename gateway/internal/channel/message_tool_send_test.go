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

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// fakeAPI stands in for the Discord, Slack and WhatsApp APIs: it keeps each
// call and answers slackAnswer on Slack's endpoint.
type fakeAPI struct {
	srv         *httptest.Server
	mu          sync.Mutex
	calls       []apiCall
	slackAnswer string
}

type apiCall struct {
	path, auth string
	body       map[string]any
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{slackAnswer: `{"ok":true}`}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		f.mu.Lock()
		f.calls = append(f.calls, apiCall{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: m})
		answer := f.slackAnswer
		f.mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/slack/") {
			_, _ = w.Write([]byte(answer))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) got() []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]apiCall(nil), f.calls...)
}

const (
	discordA  = "tenant-a/discord/dc"
	slackA    = "tenant-a/slack/sl"
	whatsappA = "tenant-a/whatsapp/wa"
	discordB  = "tenant-b/discord/dc"
)

// token is a stand-in credential, built at run time.
func token(name string) string { return "tok-" + strings.ReplaceAll(name, "/", "-") }

type sendFixture struct {
	st             *store.Store
	api            *fakeAPI
	tool           *MessageTool
	agentA, agentB *store.Agent
	dcSess, slSess *store.Session
	waSess         *store.Session
}

func newSendFixture(t *testing.T) *sendFixture {
	t.Helper()
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	f := &sendFixture{st: store.New(), api: newFakeAPI(t)}
	f.agentA, _ = f.st.CreateAgent(store.Agent{AgentKey: "sales-a", DisplayName: "Minh An", TenantID: "tenant-a"})
	f.agentB, _ = f.st.CreateAgent(store.Agent{AgentKey: "sales-b", DisplayName: "Sales B", TenantID: "tenant-b"})
	for _, c := range []struct {
		name, kind, agent string
		settings          map[string]string
	}{
		{discordA, KindBot, f.agentA.ID, nil},
		{slackA, KindBot, f.agentA.ID, nil},
		{whatsappA, KindAccess, f.agentA.ID, map[string]string{SettingPhoneNumberID: "pn-1"}},
		{discordB, KindBot, f.agentB.ID, nil},
	} {
		if err := f.st.PutChannelConfig(store.ChannelConfig{Name: c.name, Enabled: true, AgentID: c.agent, Settings: c.settings}); err != nil {
			t.Fatal(err)
		}
		if err := secrets.Put(f.st, SecretName(c.name, c.kind), []byte(token(c.name))); err != nil {
			t.Fatal(err)
		}
	}
	f.dcSess, _ = f.st.CreateSession(store.Session{AgentID: f.agentA.ID, TenantID: "tenant-a", Label: discordA + ":c-1"})
	f.slSess, _ = f.st.CreateSession(store.Session{AgentID: f.agentA.ID, TenantID: "tenant-a", Label: slackA + ":C1:thread:1700000000.000100"})
	// Imported from GoClaw: labelled with GoClaw's session key.
	f.waSess, _ = f.st.CreateSession(store.Session{AgentID: f.agentA.ID, TenantID: "tenant-a", Label: "agent:sales-a:wa:direct:15551234567@s.whatsapp.net"})
	_, _ = f.st.CreateSession(store.Session{AgentID: f.agentB.ID, TenantID: "tenant-b", Label: discordB + ":c-1"})
	f.tool = &MessageTool{Store: f.st, HTTPClient: f.api.srv.Client(), APIBase: map[string]string{
		"discord": f.api.srv.URL, "slack": f.api.srv.URL + "/slack", "whatsapp": f.api.srv.URL,
	}}
	return f
}

// send resolves and sends as the tool does after approval.
func (f *sendFixture) send(t *testing.T, ctx context.Context, args map[string]any) (map[string]any, string) {
	t.Helper()
	resolved, bad := f.tool.Resolve(ctx, args)
	if bad != nil {
		return nil, refusal(t, bad)
	}
	res, err := f.tool.Send(ctx, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "ok" {
		return resolved, refusal(t, res)
	}
	return resolved, ""
}

func lastAssistant(t *testing.T, st store.StoreIface, sessionID string) string {
	t.Helper()
	msgs, _ := st.ListMessages(sessionID)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			return msgs[i].Content
		}
	}
	return ""
}

func TestMessageToolSendsOnDiscord(t *testing.T) {
	f := newSendFixture(t)
	ctx := callerCtx("tenant-a", f.agentA.ID, f.dcSess.ID)
	if _, why := f.send(t, ctx, map[string]any{"text": "Đơn của anh đã giao"}); why != "" {
		t.Fatal(why)
	}
	calls := f.api.got()
	if len(calls) != 1 || calls[0].path != "/channels/c-1/messages" || calls[0].auth != "Bot "+token(discordA) || calls[0].body["content"] != "Đơn của anh đã giao" {
		t.Fatalf("calls %+v", calls)
	}
	if got := lastAssistant(t, f.st, f.dcSess.ID); got != "Đơn của anh đã giao" {
		t.Fatalf("thread %q", got)
	}
}

func TestMessageToolSendsIntoSlackThread(t *testing.T) {
	f := newSendFixture(t)
	ctx := callerCtx("tenant-a", f.agentA.ID, f.slSess.ID)
	resolved, why := f.send(t, ctx, map[string]any{"text": "đã xử lý"})
	if why != "" {
		t.Fatal(why)
	}
	if resolved["channel"] != slackA || resolved["chat_id"] != "C1:thread:1700000000.000100" {
		t.Fatalf("resolved %v", resolved)
	}
	calls := f.api.got()
	if len(calls) != 1 || calls[0].path != "/slack/chat.postMessage" || calls[0].auth != "Bearer "+token(slackA) {
		t.Fatalf("calls %+v", calls)
	}
	if b := calls[0].body; b["channel"] != "C1" || b["thread_ts"] != "1700000000.000100" || b["text"] != "đã xử lý" {
		t.Fatalf("body %v", b)
	}
	// Slack's "ok": false is a failed send, and nothing is recorded.
	f.api.mu.Lock()
	f.api.slackAnswer = `{"ok":false,"error":"not_in_channel"}`
	f.api.mu.Unlock()
	if _, why := f.send(t, ctx, map[string]any{"text": "lần hai"}); !strings.Contains(why, "send failed") {
		t.Fatalf("refusal %q", why)
	}
	if got := lastAssistant(t, f.st, f.slSess.ID); got != "đã xử lý" {
		t.Fatalf("thread %q", got)
	}
}

// A WhatsApp chat imported from GoClaw (session key label, JID chat) is the
// current conversation; the Cloud API gets the bare number.
func TestMessageToolSendsOnWhatsAppToImportedChat(t *testing.T) {
	f := newSendFixture(t)
	ctx := callerCtx("tenant-a", f.agentA.ID, f.waSess.ID)
	resolved, why := f.send(t, ctx, map[string]any{"text": "Xin chào"})
	if why != "" {
		t.Fatal(why)
	}
	if resolved["channel"] != whatsappA || resolved["chat_id"] != "15551234567@s.whatsapp.net" {
		t.Fatalf("resolved %v", resolved)
	}
	calls := f.api.got()
	if len(calls) != 1 || calls[0].path != "/pn-1/messages" || calls[0].auth != "Bearer "+token(whatsappA) {
		t.Fatalf("calls %+v", calls)
	}
	if b := calls[0].body; b["to"] != "15551234567" || b["messaging_product"] != "whatsapp" {
		t.Fatalf("body %v", b)
	}
	if got := lastAssistant(t, f.st, f.waSess.ID); got != "Xin chào" {
		t.Fatalf("thread %q", got)
	}
	// Without the business number id the channel cannot send.
	cfg, _ := f.st.GetChannelConfig(whatsappA)
	cfg.Settings = nil
	_ = f.st.PutChannelConfig(*cfg)
	if _, why := f.send(t, ctx, map[string]any{"text": "lần hai"}); why != "channel is not running" {
		t.Fatalf("refusal %q", why)
	}
}

func TestMessageToolSendRefusals(t *testing.T) {
	f := newSendFixture(t)
	ctxA := callerCtx("tenant-a", f.agentA.ID, f.dcSess.ID)
	ctxB := callerCtx("tenant-b", f.agentB.ID, "")
	cases := []struct {
		name string
		ctx  context.Context
		args map[string]any
		want string
	}{
		{"another tenant's channel", ctxA, map[string]any{"text": "x", "channel": discordB, "chat_id": "c-1"}, "channel not found"},
		{"another tenant's channel, seen from it", ctxB, map[string]any{"text": "x", "channel": discordA, "chat_id": "c-1"}, "channel not found"},
		{"a chat that never talked to the agent", ctxA, map[string]any{"text": "x", "channel": discordA, "chat_id": "c-2"}, "unknown chat"},
	}
	for _, tc := range cases {
		if _, why := f.send(t, tc.ctx, tc.args); !strings.Contains(why, tc.want) {
			t.Fatalf("%s: refusal %q, want %q", tc.name, why, tc.want)
		}
	}
	// No credential, no sending.
	_ = secrets.Delete(f.st, SecretName(discordA, KindBot))
	if _, why := f.send(t, ctxA, map[string]any{"text": "x"}); why != "channel is not running" {
		t.Fatalf("no token: %q", why)
	}
	if calls := f.api.got(); len(calls) != 0 {
		t.Fatalf("calls %+v, want none", calls)
	}
}

// Zalo through ZaloCRM: the message tool sends under the agent lease the
// bridge last got for the conversation, and refuses a conversation it holds
// no lease on.
func TestMessageToolSendsThroughZaloCRM(t *testing.T) {
	fx := newBridgeFixture(t)
	if w := fx.post(t, bridgeCall{text: "chào shop"}); w.Code != http.StatusAccepted {
		t.Fatalf("bridge call %d", w.Code)
	}
	var sess *store.Session
	for _, s := range fx.st.ListSessions() {
		if s.Label == bridgeA+":conv-1" {
			sess = s
		}
	}
	if sess == nil {
		t.Fatal("no conversation session")
	}
	tool := &MessageTool{Store: fx.st, Bridge: fx.bridge}
	ctx := callerCtx("tenant-a", fx.agentA.ID, sess.ID)
	resolved, bad := tool.Resolve(ctx, map[string]any{"text": "Mai shop giao hàng nhé"})
	if bad != nil {
		t.Fatal(refusal(t, bad))
	}
	if resolved["channel"] != bridgeA || resolved["chat_id"] != "conv-1" {
		t.Fatalf("resolved %v", resolved)
	}
	res, err := tool.Send(ctx, resolved)
	if err != nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	replies, bad2 := fx.crm.got()
	if bad2 != 0 || len(replies) != 2 {
		t.Fatalf("replies %v bad %d", replies, bad2)
	}
	r := replies[1]
	if r["conversationId"] != "conv-1" || r["content"] != "Mai shop giao hàng nhé" || r["agentLease"] != float64(7) {
		t.Fatalf("reply %v", r)
	}
	if k, _ := r["idempotencyKey"].(string); !strings.HasPrefix(k, "goso:tool:") {
		t.Fatalf("idempotency key %q", r["idempotencyKey"])
	}

	// A conversation with no lease since start is refused before sending.
	other, _ := fx.st.CreateSession(store.Session{AgentID: fx.agentA.ID, TenantID: "tenant-a", Label: bridgeA + ":conv-2"})
	_, bad = tool.Resolve(callerCtx("tenant-a", fx.agentA.ID, other.ID), map[string]any{"text": "x"})
	if why := refusal(t, bad); why != noLeaseMsg {
		t.Fatalf("refusal %q", why)
	}
	// Tenant B's agent cannot use tenant A's bridge.
	var agentB *store.Agent
	for _, a := range fx.st.ListAgents() {
		if a.TenantID == "tenant-b" {
			agentB = a
		}
	}
	_, bad = tool.Resolve(callerCtx("tenant-b", agentB.ID, ""), map[string]any{"text": "x", "channel": bridgeA, "chat_id": "conv-1"})
	if why := refusal(t, bad); why != "channel not found" {
		t.Fatalf("cross tenant %q", why)
	}
	if replies, _ := fx.crm.got(); len(replies) != 2 {
		t.Fatalf("replies %d, want 2", len(replies))
	}
}

// A Telegram forum topic chat is sent into its topic; the General topic
// takes no thread id.
func TestMessageToolSendsIntoTelegramTopic(t *testing.T) {
	f := newMsgFixture(t)
	var mu sync.Mutex
	var got [][2]int64
	f.mgr.bots[botA].(*Telegram).Sender = func(ctx context.Context, chatID int64, _ string) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, [2]int64{chatID, telegramThreadFrom(ctx)})
		return nil
	}
	topic, _ := f.st.CreateSession(store.Session{AgentID: f.agentA.ID, TenantID: "tenant-a", Label: botA + ":-100777:topic:9"})
	general, _ := f.st.CreateSession(store.Session{AgentID: f.agentA.ID, TenantID: "tenant-a", Label: botA + ":-100777:topic:1"})
	for _, s := range []*store.Session{topic, general} {
		ctx := callerCtx("tenant-a", f.agentA.ID, s.ID)
		args, bad := f.tool.Resolve(ctx, map[string]any{"text": "thông báo"})
		if bad != nil {
			t.Fatal(refusal(t, bad))
		}
		if res, err := f.tool.Send(ctx, args); err != nil || res.Status != "ok" {
			t.Fatalf("%v %+v", err, res)
		}
	}
	if len(got) != 2 || got[0] != [2]int64{-100777, 9} || got[1] != [2]int64{-100777, 0} {
		t.Fatalf("sent %v", got)
	}
}

func TestTelegramChatForms(t *testing.T) {
	for _, tc := range []struct {
		in           string
		chat, thread int64
		ok           bool
	}{
		{"42", 42, 0, true},
		{"-100777:topic:9", -100777, 9, true},
		{"-100777:topic:1", -100777, 0, true},
		{"42:thread:3", 42, 3, true},
		{"42:topic:x", 0, 0, false},
		{"42:other:3", 0, 0, false},
		{"abc", 0, 0, false},
	} {
		chat, thread, err := telegramChat(tc.in)
		if (err == nil) != tc.ok || chat != tc.chat || thread != tc.thread {
			t.Errorf("%q: %d %d %v", tc.in, chat, thread, err)
		}
	}
}

// A Lark topic chat is answered inside the topic.
func TestLarkSendToTopicChat(t *testing.T) {
	var got []LarkMessage
	l := &Lark{Sender: func(_ context.Context, m LarkMessage, _ string) error {
		got = append(got, m)
		return nil
	}}
	_ = l.sendToChat(context.Background(), "oc_group:topic:om_r1", "a")
	_ = l.sendToChat(context.Background(), "oc_group", "b")
	if len(got) != 2 || got[0].ChatID != "oc_group" || got[0].RootID != "om_r1" || got[1].ChatID != "oc_group" || got[1].RootID != "" {
		t.Fatalf("sent %+v", got)
	}
}
