package bridge

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/guardrail"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Fake customer C only; the LLM is fakeLLM. The no-invention guardrail closes
// the ONE system message of every Omni reply, after the agent prompt (or the
// caller's system_prompt), the knowledge block and this customer's memory.
func TestBridgeGuardrailInSystemMessage(t *testing.T) {
	sq, err := store.OpenSQLite(filepath.Join(t.TempDir(), "guard.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	e := newCustEnv(t, sq)

	check := func(sys string, before ...string) {
		t.Helper()
		if !strings.HasSuffix(sys, guardrail.Reply) || strings.Count(sys, guardrail.Reply) != 1 {
			t.Fatalf("guardrail must close the system message once:\n%s", sys)
		}
		g := strings.Index(sys, guardrail.Reply)
		for _, s := range before {
			if i := strings.Index(sys, s); i < 0 || i > g {
				t.Fatalf("%q must come before the guardrail:\n%s", s, sys)
			}
		}
	}

	e.send(tenantOne, "acc-1", "conv-C", "Tôi là khách C, đặt 3 ghế mã ZC-5508", false)
	for _, stream := range []bool{false, true} {
		sys := e.send(tenantOne, "acc-1", "conv-C", "tôi có quà gì không?", stream)
		check(sys, "Bạn là trợ lý bán hàng.", "[knowledge truncated]", "ZC-5508")
	}

	// A caller system_prompt (omni-core reply drafts) and an agent with no
	// prompt or knowledge both still get the guardrail.
	post := func(agentID string, extra map[string]any) string {
		t.Helper()
		m := map[string]any{"agent_id": agentID, "model": "m1", "prompt": "có voucher không?",
			"metadata": map[string]string{"channel_type": "zalo", "channel_account_id": "acc-1", "external_conversation_id": "conv-C2"}}
		for k, v := range extra {
			m[k] = v
		}
		body, _ := json.Marshal(m)
		req, _ := http.NewRequest(http.MethodPost, e.base+"/v1/chat/reply", bytes.NewReader(body))
		stamp(req.Header, e.svc.Secret, tenantOne, body)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("%d %s", res.StatusCode, raw)
		}
		return singleSystem(t, e.fake.snapshot().Messages)
	}
	check(post(memAgent, map[string]any{"system_prompt": "Soạn câu trả lời nháp."}), "Soạn câu trả lời nháp.")

	bare := []byte(`{"name":"bare","harness_type":"GOSO","model_name":"m1"}`)
	if res := doSigned(t, http.MethodPut, e.base+"/v1/tenants/"+tenantOne+"/agents/agent-bare", e.svc.Secret, tenantOne, bare); res.StatusCode != 200 {
		t.Fatalf("agent %d %s", res.StatusCode, res.Body)
	}
	if sys := post("agent-bare", nil); sys != guardrail.Reply {
		t.Fatalf("bare agent system:\n%s", sys)
	}
}
