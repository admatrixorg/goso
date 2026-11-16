// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package serve

import (
	"context"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/contextfile"
	"github.com/mqglobal/goso/gateway/internal/guardrail"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// A channel turn on a tenant instance session runs the full agent runtime:
// tools advertised, the user's context files, this customer's memory only, the
// guardrail closing the system message, and the user message stored once.
// An archived session still answers, as channels always did.
func TestChannelTurnRunner_FullRuntimeOnTenantSession(t *testing.T) {
	t.Setenv("GOSO_CONTEXT_FILES", "1")
	st := store.New()
	ag, err := st.CreateAgent(store.Agent{AgentKey: "sales", DisplayName: "Sales", TenantID: "demo", Instructions: "Bạn là trợ lý bán hàng."})
	if err != nil {
		t.Fatal(err)
	}
	const bot = "demo/telegram/minh-an-bot"
	old, _ := st.CreateSession(store.Session{AgentID: ag.ID, TenantID: "demo", Label: bot + ":42"})
	other, _ := st.CreateSession(store.Session{AgentID: ag.ID, TenantID: "demo", Label: bot + ":99"})
	if _, err := st.PutMemory(store.Memory{TenantID: "demo", SessionID: old.ID, Kind: store.KindDurable, Body: "Minh An thích sofa walnut"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutMemory(store.Memory{TenantID: "demo", SessionID: other.ID, Kind: store.KindDurable, Body: "khách khác hỏi sofa walnut giá sỉ"}); err != nil {
		t.Fatal(err)
	}
	if m, _ := st.PutMemory(store.Memory{TenantID: "demo", SessionID: old.ID, Kind: store.KindDurable, Body: "x"}); m == nil || m.CustomerKey != bot+":42" {
		t.Fatalf("tenant instance note not customer-stamped: %+v", m)
	}
	if _, err := st.PutContextFile(store.ContextFile{TenantID: "demo", Scope: store.ContextScopeUser, UserKey: "42", Name: "USER.md", Body: "Khách Minh An ở Đà Nẵng."}); err != nil {
		t.Fatal(err)
	}
	sess, _ := st.CreateSession(store.Session{AgentID: ag.ID, TenantID: "demo", Label: bot + ":42"})
	upd := *sess
	upd.Archived = true
	upd.TouchArchive = true
	if _, err := st.UpdateSession(upd); err != nil {
		t.Fatal(err)
	}

	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "Dạ còn ạ"}}}
	rt := agent.New(st, nil, nil, nil, usageProvider{scripted})
	run := ChannelTurnRunner(rt)
	ctx := contextfile.WithUser(context.Background(), "42")
	res, err := run(ctx, sess.ID, "sofa walnut còn không?")
	if err != nil {
		t.Fatal(err)
	}
	if res.Reply != "Dạ còn ạ" || res.Pending || res.Usage.PromptTokens != 5 {
		t.Fatalf("result %+v", res)
	}
	if len(scripted.Recorded) != 1 || len(scripted.RecordedTools) != 1 {
		t.Fatalf("model calls %d / tool lists %d", len(scripted.Recorded), len(scripted.RecordedTools))
	}
	if len(scripted.RecordedTools[0]) == 0 {
		t.Fatal("no tools advertised on a channel turn")
	}
	sys := scripted.Recorded[0][0]
	if sys.Role != "system" {
		t.Fatalf("first message %+v", sys)
	}
	for _, want := range []string{"Bạn là trợ lý bán hàng.", "Khách Minh An ở Đà Nẵng.", "Minh An thích sofa walnut"} {
		if !strings.Contains(sys.Content, want) {
			t.Errorf("system message lacks %q", want)
		}
	}
	if strings.Contains(sys.Content, "giá sỉ") {
		t.Error("another customer's note reached this customer")
	}
	if !strings.HasSuffix(strings.TrimSpace(sys.Content), strings.TrimSpace(guardrail.Reply)) {
		t.Error("guardrail does not close the channel system message")
	}
	msgs, _ := st.ListMessages(sess.ID)
	users := 0
	for _, m := range msgs {
		if m.Role == "user" && m.Content == "sofa walnut còn không?" {
			users++
		}
	}
	if users != 1 {
		t.Fatalf("user message stored %d times", users)
	}
}

// usageProvider reports fixed token usage on every call.
type usageProvider struct{ *llm.Scripted }

func (u usageProvider) ChatTools(ctx context.Context, msgs []llm.Message, tools []llm.ToolSpec) (llm.Reply, error) {
	r, err := u.Scripted.ChatTools(ctx, msgs, tools)
	r.Usage = llm.Usage{PromptTokens: 5, CompletionTokens: 2}
	return r, err
}

// Web sessions do not get the channel guardrail from the runner.
func TestRunnerGuardrailOnlyOnChannelSessions(t *testing.T) {
	st := store.New()
	ag, _ := st.CreateAgent(store.Agent{AgentKey: "web", DisplayName: "Web"})
	sess, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "web chat"})
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "ok"}}}
	rt := agent.New(st, nil, nil, nil, scripted)
	if _, err := rt.Chat(context.Background(), sess.ID, "hi"); err != nil {
		t.Fatal(err)
	}
	for _, m := range scripted.Recorded[0] {
		if strings.Contains(m.Content, guardrail.Reply) {
			t.Fatal("web turn got the channel guardrail")
		}
	}
}
