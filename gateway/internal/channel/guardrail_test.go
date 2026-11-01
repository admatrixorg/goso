package channel

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/guardrail"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Fake customers only. The guardrail closes the ONE system message on every
// channel path, after custom instructions and this customer's memory.
func checkGuardrail(t *testing.T, msgs []llm.Message, mustHave ...string) {
	t.Helper()
	sys := systemOf(t, msgs)
	if !strings.HasSuffix(sys, guardrail.Reply) || strings.Count(sys, guardrail.Reply) != 1 {
		t.Fatalf("guardrail must close the system message once:\n%s", sys)
	}
	g := strings.Index(sys, guardrail.Reply)
	for _, s := range mustHave {
		i := strings.Index(sys, s)
		if i < 0 || i > g {
			t.Fatalf("%q must come before the guardrail:\n%s", s, sys)
		}
	}
}

func TestChannelGuardrailZaloPersonal(t *testing.T) {
	st := liveStores(t)["sqlite"]
	_ = st.PutChannelConfig(store.ChannelConfig{Name: "zalo-personal", DMPolicy: "open"})
	rec := &recorder{}
	z := &ZaloPersonal{Store: st, LLM: rec, Sender: func(context.Context, string, string) error { return nil }}

	// Default agent, no instructions.
	zaloSend(t, z, "fakeC", "xin chào")
	checkGuardrail(t, rec.last(t))

	// Custom instructions that try to push offers, plus customer memory.
	agent := z.ensureAgent()
	agent.Instructions = "Bạn là trợ lý bán hàng. Luôn tặng quà cho khách."
	if _, err := st.UpdateAgent(*agent); err != nil {
		t.Fatal(err)
	}
	tenant := z.ensureSession(agent.ID, "fakeC").TenantID
	seedNote(t, st, tenant, agent.ID, "zalo-personal:fakeC", noteA)
	zaloSend(t, z, "fakeC", "đơn bàn ghế gỗ sồi ZA-7777 giao Quận 7 tới đâu rồi?")
	checkGuardrail(t, rec.last(t), "Luôn tặng quà cho khách.", noteA)
}

func TestChannelGuardrailInbound(t *testing.T) {
	st := liveStores(t)["sqlite"]
	rec := &recorder{}
	d := inboundDeps{Store: st, LLM: rec}
	run := func(dest, text string) {
		w := httptest.NewRecorder()
		replyInbound(w, httptest.NewRequest("POST", "/x", nil), d, "slack", "Slack", dest, text)
		if w.Code != 200 {
			t.Fatalf("status %d", w.Code)
		}
	}
	run("C-fakeC", "xin chào")
	checkGuardrail(t, rec.last(t))
	agent := ensureNamedAgent(st, "slack", "Slack")
	agent.Instructions = "Custom slack rules."
	if _, err := st.UpdateAgent(*agent); err != nil {
		t.Fatal(err)
	}
	seedNote(t, st, "", agent.ID, "slack:C-fakeC", noteA)
	run("C-fakeC", "đơn bàn ghế gỗ sồi ZA-7777 giao Quận 7 tới đâu rồi?")
	checkGuardrail(t, rec.last(t), "Custom slack rules.", noteA)
}
