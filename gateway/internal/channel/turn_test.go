// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/contextfile"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// fakeRuntime stands in for the agent runtime: it stores the user message and
// the reply the way pipeline.Runner does, and records what it was given.
type fakeRuntime struct {
	st      store.StoreIface
	mu      sync.Mutex
	userKey []string
	session []string
	res     TurnResult
	err     error
	// storeFirst stores the user message before failing (a model error).
	storeFirst bool
}

func (f *fakeRuntime) run(ctx context.Context, sessionID, text string) (TurnResult, error) {
	f.mu.Lock()
	f.userKey = append(f.userKey, contextfile.UserFrom(ctx))
	f.session = append(f.session, sessionID)
	f.mu.Unlock()
	if f.err != nil {
		if f.storeFirst {
			_, _ = f.st.AddMessage(store.Message{SessionID: sessionID, Role: "user", Content: text})
		}
		return TurnResult{}, f.err
	}
	_, _ = f.st.AddMessage(store.Message{SessionID: sessionID, Role: "user", Content: text})
	_, _ = f.st.AddMessage(store.Message{SessionID: sessionID, Role: "assistant", Content: f.res.Reply})
	return f.res, nil
}

func useRuntime(t *testing.T, f *fakeRuntime) {
	t.Helper()
	SetTurnRunner(f.run)
	t.Cleanup(func() { SetTurnRunner(nil) })
}

func countUser(t *testing.T, st store.StoreIface, sessionID, text string) int {
	t.Helper()
	msgs, err := st.ListMessages(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, m := range msgs {
		if m.Role == "user" && m.Content == text {
			n++
		}
	}
	return n
}

func TestChannelCatalogsMatchStore(t *testing.T) {
	if strings.Join(Names, ",") != strings.Join(store.ChannelCatalogs, ",") {
		t.Fatalf("channel.Names %v != store.ChannelCatalogs %v", Names, store.ChannelCatalogs)
	}
}

func TestContextUserKey_GoClawFormat(t *testing.T) {
	cases := []struct {
		instance, peer, sender, chat, want string
	}{
		{"demo/telegram/minh-an-bot", "direct", "123456789", "123456789", "123456789"},
		{"demo/telegram/minh-an-bot", "group", "123456789", "-1001234", "group:minh-an-bot:-1001234"},
		{"demo/feishu/minh-an-lark", "direct", "ou_abc123", "oc_x", "ou_abc123"},
		{"demo/feishu/minh-an-lark", "group", "ou_abc123", "oc_x", "group:minh-an-lark:oc_x"},
		{"demo/zalo-oa/minh-an-oa", "direct", "8812", "8812", "8812"},
		{"demo/zalo-personal/minh-an-zalo", "group", "71", "g-9", "group:minh-an-zalo:g-9"},
		{"telegram", "group", "1", "-5", "group:telegram:-5"},
		{"zalo-oa", "direct", "u1", "u1", "u1"},
		{"demo/telegram/minh-an-bot", "direct", "", "1", ""},
		{"demo/telegram/minh-an-bot", "group", "1", "", ""},
		{"demo/telegram/minh-an-bot", "direct", "bad id", "1", ""},
		{"demo/telegram/minh-an-bot", "direct", strings.Repeat("9", 129), "1", ""},
	}
	for _, c := range cases {
		if got := ContextUserKey(c.instance, c.peer, c.sender, c.chat); got != c.want {
			t.Errorf("ContextUserKey(%q,%q,%q,%q)=%q want %q", c.instance, c.peer, c.sender, c.chat, got, c.want)
		}
	}
}

func TestCustomerReply_HidesInternalMarkers(t *testing.T) {
	cases := []struct {
		res  TurnResult
		want string
	}{
		{TurnResult{Reply: "Chào Minh An"}, "Chào Minh An"},
		{TurnResult{Reply: "Dạ, em kiểm tra đơn.\npending_approval: ap_123", Pending: true}, "Dạ, em kiểm tra đơn."},
		{TurnResult{Reply: "pending_approval: ap_123", Pending: true}, pendingReply},
		{TurnResult{Reply: "max_iterations"}, pendingReply},
		{TurnResult{Reply: ""}, ""},
	}
	for _, c := range cases {
		if got := customerReply(c.res); got != c.want {
			t.Errorf("customerReply(%q)=%q want %q", c.res.Reply, got, c.want)
		}
	}
}

func TestAnswerTurn_UsesRuntimeWhenWired(t *testing.T) {
	st := store.New()
	ag, _ := st.CreateAgent(store.Agent{AgentKey: "sales", DisplayName: "Sales"})
	sess, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "telegram:42"})
	f := &fakeRuntime{st: st, res: TurnResult{Reply: "runtime-ok", AgentID: ag.ID, Provider: "scripted",
		Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 3}}}
	useRuntime(t, f)
	meter := billing.New()
	direct := &llm.Scripted{Replies: []llm.Reply{{Text: "direct"}}}
	reply := answerTurn(context.Background(), st, ag, sess, direct, meter, "Minh An hỏi giá")
	if reply != "runtime-ok" {
		t.Fatalf("reply %q", reply)
	}
	if len(direct.Recorded) != 0 {
		t.Fatal("direct model was called although the runtime is wired")
	}
	if n := countUser(t, st, sess.ID, "Minh An hỏi giá"); n != 1 {
		t.Fatalf("user message stored %d times", n)
	}
	if f.session[0] != sess.ID {
		t.Fatalf("runtime got session %q", f.session[0])
	}
	sum := meter.Query(billing.Query{AgentID: ag.ID})
	if sum.Calls != 1 || sum.PromptTokens != 10 || sum.CompletionTokens != 3 || sum.Provider != "" && sum.Provider != "scripted" {
		t.Fatalf("usage %+v", sum)
	}
}

func TestAnswerTurn_RuntimeErrorStoresUserOnce(t *testing.T) {
	for _, storeFirst := range []bool{false, true} {
		t.Run(fmt.Sprint("storeFirst=", storeFirst), func(t *testing.T) {
			st := store.New()
			ag, _ := st.CreateAgent(store.Agent{AgentKey: "sales", DisplayName: "Sales"})
			sess, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "zalo-oa:u1"})
			useRuntime(t, &fakeRuntime{st: st, err: errors.New("upstream 502"), storeFirst: storeFirst})
			reply := answerTurn(context.Background(), st, ag, sess, nil, nil, "khách hỏi")
			if reply != "LLM error: upstream 502" {
				t.Fatalf("answerTurn reply %q", reply)
			}
			if n := countUser(t, st, sess.ID, "khách hỏi"); n != 1 {
				t.Fatalf("user message stored %d times", n)
			}
			got, err := answerTurnErr(context.Background(), st, ag, sess, nil, nil, "khách hỏi lần 2")
			if err == nil || got != "" {
				t.Fatalf("answerTurnErr %q %v", got, err)
			}
			if n := countUser(t, st, sess.ID, "khách hỏi lần 2"); n != 1 {
				t.Fatalf("second user message stored %d times", n)
			}
		})
	}
}

func TestAnswerTurn_PendingApprovalHidden(t *testing.T) {
	st := store.New()
	ag, _ := st.CreateAgent(store.Agent{AgentKey: "sales", DisplayName: "Sales"})
	sess, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "telegram:42"})
	useRuntime(t, &fakeRuntime{st: st, res: TurnResult{Reply: "pending_approval: ap_1", Pending: true}})
	reply, err := answerTurnErr(context.Background(), st, ag, sess, nil, nil, "đặt đơn")
	if err != nil || reply != pendingReply {
		t.Fatalf("reply %q err %v", reply, err)
	}
}

// A tenant Telegram bot passes GoClaw's user key to the runtime: the sender id
// in a direct chat, group:<instance>:<chat> in a group.
func TestTelegramTenantBot_ContextUserKey(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	st := store.New()
	ag, err := st.CreateAgent(store.Agent{AgentKey: "sales", DisplayName: "Sales", TenantID: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	bot := "demo/telegram/minh-an-bot"
	if err := st.PutChannelConfig(store.ChannelConfig{Name: bot, Enabled: true, AgentID: ag.ID, DMPolicy: "open", GroupPolicy: "open"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeRuntime{st: st, res: TurnResult{Reply: "ok"}}
	useRuntime(t, f)
	var out sent
	tg := &Telegram{Instance: bot, Store: st, LLM: llm.Echo{}, Sender: out.fn}
	if w := tg.ingest(context.Background(), tgUpdate(t, 42, "xin chào")); w != "" {
		t.Fatalf("dm warning %q", w)
	}
	var grp TelegramUpdate
	if err := json.Unmarshal([]byte(`{"update_id":2,"message":{"message_id":2,"chat":{"id":-100777},"from":{"id":42},"text":"@bot giá?"}}`), &grp); err != nil {
		t.Fatal(err)
	}
	if w := tg.ingest(context.Background(), grp); w != "" {
		t.Fatalf("group warning %q", w)
	}
	if len(f.userKey) != 2 || f.userKey[0] != "42" || f.userKey[1] != "group:minh-an-bot:-100777" {
		t.Fatalf("user keys %q", f.userKey)
	}
	if out.count() != 2 {
		t.Fatalf("replies %d", out.count())
	}
}

func TestTelegram_NoContextUserKeyWhenFlagOff(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "")
	st := store.New()
	ag, _ := st.CreateAgent(store.Agent{AgentKey: "telegram", DisplayName: "Telegram Bot"})
	if err := st.PutChannelConfig(store.ChannelConfig{Name: "telegram", AgentID: ag.ID, Enabled: true, DMPolicy: "open"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeRuntime{st: st, res: TurnResult{Reply: "ok"}}
	useRuntime(t, f)
	var out sent
	tg := &Telegram{Store: st, LLM: llm.Echo{}, Sender: out.fn}
	_ = tg.ingest(context.Background(), tgUpdate(t, 7, "hi"))
	if len(f.userKey) != 1 || f.userKey[0] != "" {
		t.Fatalf("user keys %q", f.userKey)
	}
}
