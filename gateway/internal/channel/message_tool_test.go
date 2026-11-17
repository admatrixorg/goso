// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/builtin"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/skill"
	"github.com/mqglobal/goso/gateway/internal/store"
)

type sentTo struct {
	mu    sync.Mutex
	chats []string
	texts []string
	err   error
}

func (s *sentTo) tg(_ context.Context, chatID int64, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.chats = append(s.chats, strconv.FormatInt(chatID, 10))
	s.texts = append(s.texts, text)
	return nil
}

func (s *sentTo) str(_ context.Context, chatID, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.chats = append(s.chats, chatID)
	s.texts = append(s.texts, text)
	return nil
}

func (s *sentTo) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.texts)
}

type msgFixture struct {
	st             *store.Store
	mgr            *Manager
	tool           *MessageTool
	agentA, agentB *store.Agent
	agentA2        *store.Agent
	sessA, sessB   *store.Session
	outA, outB     *sentTo
}

const (
	botA = "tenant-a/telegram/shop"
	botB = "tenant-b/telegram/shop"
)

func newMsgFixture(t *testing.T) *msgFixture {
	t.Helper()
	f := &msgFixture{st: store.New(), mgr: NewManager(), outA: &sentTo{}, outB: &sentTo{}}
	var err error
	if f.agentA, err = f.st.CreateAgent(store.Agent{AgentKey: "sales-a", DisplayName: "Sales A", TenantID: "tenant-a"}); err != nil {
		t.Fatal(err)
	}
	if f.agentA2, err = f.st.CreateAgent(store.Agent{AgentKey: "support-a", DisplayName: "Support A", TenantID: "tenant-a"}); err != nil {
		t.Fatal(err)
	}
	if f.agentB, err = f.st.CreateAgent(store.Agent{AgentKey: "sales-b", DisplayName: "Sales B", TenantID: "tenant-b"}); err != nil {
		t.Fatal(err)
	}
	openBot(t, f.st, botA, f.agentA.ID)
	openBot(t, f.st, botB, f.agentB.ID)
	f.mgr.bots[botA] = &Telegram{Instance: botA, Store: f.st, Sender: f.outA.tg}
	f.mgr.bots[botB] = &Telegram{Instance: botB, Store: f.st, Sender: f.outB.tg}
	f.mgr.SetRunning(botA, "polling")
	f.mgr.SetRunning(botB, "polling")
	if f.sessA, err = f.st.CreateSession(store.Session{AgentID: f.agentA.ID, TenantID: "tenant-a", Label: botA + ":42"}); err != nil {
		t.Fatal(err)
	}
	if f.sessB, err = f.st.CreateSession(store.Session{AgentID: f.agentB.ID, TenantID: "tenant-b", Label: botB + ":42"}); err != nil {
		t.Fatal(err)
	}
	f.tool = &MessageTool{Store: f.st, Manager: f.mgr}
	return f
}

func callerCtx(tenant, agentID, sessionID string) context.Context {
	return builtin.WithCaller(skill.WithTenant(context.Background(), tenant), agentID, sessionID)
}

func refusal(t *testing.T, res *connector.InvokeResult) string {
	t.Helper()
	if res == nil {
		t.Fatal("expected a refusal")
	}
	if res.Status != "error" {
		t.Fatalf("status %q", res.Status)
	}
	return res.Content.(map[string]any)["error"].(string)
}

func TestMessageTool_DefaultsToCurrentConversation(t *testing.T) {
	f := newMsgFixture(t)
	ctx := callerCtx("tenant-a", f.agentA.ID, f.sessA.ID)
	args, bad := f.tool.Resolve(ctx, map[string]any{"text": "Chào Minh An, đơn đã giao"})
	if bad != nil {
		t.Fatalf("%+v", bad)
	}
	if args["channel"] != botA || args["chat_id"] != "42" {
		t.Fatalf("%+v", args)
	}
	res, err := f.tool.Send(ctx, args)
	if err != nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	if f.outA.count() != 1 || f.outA.chats[0] != "42" || f.outA.texts[0] != "Chào Minh An, đơn đã giao" || f.outB.count() != 0 {
		t.Fatalf("A=%+v B=%+v", f.outA, f.outB)
	}
	msgs, _ := f.st.ListMessages(f.sessA.ID)
	if len(msgs) != 1 || msgs[0].Role != "assistant" {
		t.Fatalf("thread %+v", msgs)
	}
}

// The core isolation check: tenant A's agent can never reach tenant B's bot,
// by naming it, by a forged context tenant, or as tenant B's own chat.
func TestMessageTool_NeverCrossesTenants(t *testing.T) {
	f := newMsgFixture(t)
	ctxA := callerCtx("tenant-a", f.agentA.ID, f.sessA.ID)
	for _, args := range []map[string]any{
		{"text": "x", "channel": botB, "chat_id": "42"},
		{"text": "x", "channel": botB},
		{"text": "x", "channel": "telegram", "chat_id": "42"},
		{"text": "x", "channel": "tenant-b/telegram/other", "chat_id": "42"},
	} {
		_, bad := f.tool.Resolve(ctxA, args)
		if msg := refusal(t, bad); msg != "channel not found" && !strings.Contains(msg, "required") {
			t.Fatalf("%v: %q", args, msg)
		}
		res, _ := f.tool.Send(ctxA, args)
		refusal(t, res)
	}
	// A context claiming tenant B does not turn agent A into a tenant B caller.
	forged := callerCtx("tenant-b", f.agentA.ID, f.sessB.ID)
	if _, bad := f.tool.Resolve(forged, map[string]any{"text": "x", "channel": botB, "chat_id": "42"}); bad == nil {
		t.Fatal("forged tenant accepted")
	}
	// Agent A's default target never resolves from a tenant B session.
	if _, bad := f.tool.Resolve(callerCtx("tenant-a", f.agentA.ID, f.sessB.ID), map[string]any{"text": "x"}); bad == nil {
		t.Fatal("foreign session used as default target")
	}
	if f.outB.count() != 0 || f.outA.count() != 0 {
		t.Fatalf("sent A=%d B=%d", f.outA.count(), f.outB.count())
	}
}

func TestMessageTool_Refusals(t *testing.T) {
	f := newMsgFixture(t)
	ctx := callerCtx("tenant-a", f.agentA.ID, f.sessA.ID)
	cases := []struct {
		name string
		ctx  context.Context
		args map[string]any
		want string
	}{
		{"no agent", context.Background(), map[string]any{"text": "x", "channel": botA, "chat_id": "42"}, "calling agent"},
		{"no text", ctx, map[string]any{}, "text is required"},
		{"too long", ctx, map[string]any{"text": strings.Repeat("a", MessageMaxRunes+1)}, "longer than"},
		{"other agent same tenant", callerCtx("tenant-a", f.agentA2.ID, ""), map[string]any{"text": "x", "channel": botA, "chat_id": "42"}, "not bound"},
		{"unknown chat", ctx, map[string]any{"text": "x", "channel": botA, "chat_id": "99"}, "unknown chat"},
		{"web session no target", callerCtx("tenant-a", f.agentA.ID, ""), map[string]any{"text": "x"}, "required"},
	}
	for _, tc := range cases {
		_, bad := f.tool.Resolve(tc.ctx, tc.args)
		if msg := refusal(t, bad); !strings.Contains(msg, tc.want) {
			t.Fatalf("%s: %q", tc.name, msg)
		}
	}
	if f.outA.count() != 0 {
		t.Fatal("refused calls must not send")
	}
}

func TestMessageTool_DisabledNotRunningUnsupported(t *testing.T) {
	f := newMsgFixture(t)
	ctx := callerCtx("tenant-a", f.agentA.ID, f.sessA.ID)
	args := map[string]any{"text": "x", "channel": botA, "chat_id": "42"}

	stopped := &MessageTool{Store: f.st, Manager: NewManager()}
	if _, bad := stopped.Resolve(ctx, args); !strings.Contains(refusal(t, bad), "not running") {
		t.Fatal("stopped bot")
	}

	slack := "tenant-a/slack/team"
	openBot(t, f.st, slack, f.agentA.ID)
	f.mgr.SetRunning(slack, "socket")
	// Slack is send-only: without its bot token it is not running.
	if _, bad := f.tool.Resolve(ctx, map[string]any{"text": "x", "channel": slack, "chat_id": "C1"}); !strings.Contains(refusal(t, bad), "not running") {
		t.Fatal("slack without a token must not be running")
	}

	// Approved while enabled, disabled before the send: Send refuses.
	frozen, bad := f.tool.Resolve(ctx, args)
	if bad != nil {
		t.Fatalf("%+v", bad)
	}
	if err := f.st.PutChannelConfig(store.ChannelConfig{Name: botA, Enabled: false, AgentID: f.agentA.ID, DMPolicy: "open"}); err != nil {
		t.Fatal(err)
	}
	res, _ := f.tool.Send(ctx, frozen)
	if !strings.Contains(refusal(t, res), "disabled") || f.outA.count() != 0 {
		t.Fatal("disabled channel sent")
	}
}

func TestMessageTool_SendErrorIsGeneric(t *testing.T) {
	f := newMsgFixture(t)
	f.outA.err = errors.New(`Post "https://api.telegram.org/bot123:SECRET/sendMessage": boom`)
	ctx := callerCtx("tenant-a", f.agentA.ID, f.sessA.ID)
	res, err := f.tool.Send(ctx, map[string]any{"text": "x"})
	if err != nil {
		t.Fatal(err)
	}
	msg := refusal(t, res)
	if strings.Contains(msg, "SECRET") || !strings.Contains(msg, "send failed") {
		t.Fatalf("%q", msg)
	}
	if msgs, _ := f.st.ListMessages(f.sessA.ID); len(msgs) != 0 {
		t.Fatal("failed send must not be recorded")
	}
}

func TestMessageTool_ZaloBotAndLark(t *testing.T) {
	f := newMsgFixture(t)
	zalo, lark := "tenant-a/zalo-oa/shop", "tenant-a/feishu/team"
	zout, lout := &sentTo{}, &sentTo{}
	openBot(t, f.st, zalo, f.agentA.ID)
	openBot(t, f.st, lark, f.agentA.ID)
	f.mgr.bots[zalo] = &ZaloBot{Instance: zalo, Store: f.st, Sender: zout.str}
	f.mgr.bots[lark] = &Lark{Instance: lark, Store: f.st, Sender: func(ctx context.Context, m LarkMessage, text string) error {
		return lout.str(ctx, m.ChatID, text)
	}}
	f.mgr.SetRunning(zalo, "polling")
	f.mgr.SetRunning(lark, "webhook")
	if _, err := f.st.CreateSession(store.Session{AgentID: f.agentA.ID, TenantID: "tenant-a", Label: zalo + ":u-7"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.CreateSession(store.Session{AgentID: f.agentA.ID, TenantID: "tenant-a", Label: lark + ":oc_1"}); err != nil {
		t.Fatal(err)
	}
	ctx := callerCtx("tenant-a", f.agentA.ID, "")
	long := strings.Repeat("ạ", zaloBotMaxText+10)
	res, _ := f.tool.Send(ctx, map[string]any{"text": long, "channel": zalo, "chat_id": "u-7"})
	if res.Status != "ok" || zout.count() != 2 || zout.chats[0] != "u-7" || res.Content.(map[string]any)["parts"] != 2 {
		t.Fatalf("%+v %+v", res, zout)
	}
	res, _ = f.tool.Send(ctx, map[string]any{"text": "Chào Minh An", "channel": lark, "chat_id": "oc_1"})
	if res.Status != "ok" || lout.count() != 1 || lout.chats[0] != "oc_1" {
		t.Fatalf("%+v %+v", res, lout)
	}
}

func TestInstanceOfLabel_LongestPrefix(t *testing.T) {
	st := store.New()
	for _, n := range []string{"tenant-a/telegram/shop", "tenant-a/telegram/shop-2"} {
		if err := st.PutChannelConfig(store.ChannelConfig{Name: n, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	if n, c := instanceOfLabel(st, "tenant-a", "", "tenant-a/telegram/shop-2:5"); n != "tenant-a/telegram/shop-2" || c != "5" {
		t.Fatalf("%s %s", n, c)
	}
	if n, _ := instanceOfLabel(st, "tenant-b", "", "tenant-a/telegram/shop:5"); n != "" {
		t.Fatal("other tenant's instance matched")
	}
}
