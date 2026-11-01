// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// SPEC 132 §bridge: a conversation-scoped session gets an L1 summary after
// SummarizeMessageMin messages and sees it on the next turn; the shared
// fallback session (no conversation id) never does; another conversation of
// the same agent never sees it.
func TestBridgeMemoryPerConversation(t *testing.T) {
	master, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", master)
	t.Setenv("GOSO_MEMORY_RECALL", "")
	fake := &fakeLLM{text: "Khách A: 20 bộ bàn ghế, 150 triệu, hẹn thứ Sáu 9h"}
	st := store.New()
	svc := &Service{Secret: "test-secret", Version: "test", Catalog: newMem(), Store: st, Complete: fake.Complete}
	addr := startBridge(t, svc)
	base := "http://" + addr
	const (
		tenant  = "00000000-0000-0000-0000-000000000001"
		agentID = "agent-mem"
	)
	prov := []byte(`{"name":"p","provider_type":"custom","base_url":"http://9router:20128/v1","api_key":"fixture-key","models":["m1"],"is_active":true}`)
	if res := doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/providers/p1", svc.Secret, tenant, prov); res.StatusCode != 200 {
		t.Fatalf("provider %d %s", res.StatusCode, res.Body)
	}
	agent := []byte(`{"name":"mem","harness_type":"GOSO","model_name":"m1","system_prompt":"Bạn là trợ lý bán hàng."}`)
	if res := doSigned(t, http.MethodPut, base+"/v1/tenants/"+tenant+"/agents/"+agentID, svc.Secret, tenant, agent); res.StatusCode != 200 {
		t.Fatalf("agent %d %s", res.StatusCode, res.Body)
	}
	reply := func(conv string, i int) {
		t.Helper()
		body := `{"agent_id":"` + agentID + `","model":"m1","prompt":"lượt ` + strconv.Itoa(i) + ` khách hỏi giá bàn ghế"`
		if conv != "" {
			body += `,"metadata":{"conversation_id":"` + conv + `"}`
		}
		body += `}`
		res := doSigned(t, http.MethodPost, base+"/v1/chat/reply", svc.Secret, tenant, []byte(body))
		if res.StatusCode != 200 {
			t.Fatalf("reply %s/%d: %d %s", conv, i, res.StatusCode, res.Body)
		}
	}
	memories := func() int {
		n := 0
		for _, sess := range st.ListSessions() {
			if sess == nil {
				continue
			}
			ms, _ := st.ListMemories(sess.ID)
			n += len(ms)
		}
		return n
	}
	turns := pipeline.SummarizeMessageMin/2 + 1

	// 1) Shared fallback session: never summarised, nothing injected.
	for i := 1; i <= turns; i++ {
		reply("", i)
	}
	time.Sleep(150 * time.Millisecond)
	if n := memories(); n != 0 {
		t.Fatalf("shared session must not be summarised, memories=%d", n)
	}
	for _, m := range fake.last.Messages {
		if strings.Contains(m.Content, pipeline.PreviousSummaryPrefix) {
			t.Fatalf("shared session got a memory note: %q", m.Content)
		}
	}

	// 2) Conversation c1: summary written once the session is long enough.
	for i := 1; i <= pipeline.SummarizeMessageMin/2; i++ {
		reply("c1", i)
	}
	deadline := time.Now().Add(3 * time.Second)
	for memories() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := memories(); n != 1 {
		t.Fatalf("c1 summary expected, memories=%d", n)
	}
	reply("c1", pipeline.SummarizeMessageMin/2+1)
	var note string
	for _, m := range fake.last.Messages {
		if i := strings.Index(m.Content, pipeline.PreviousSummaryPrefix); m.Role == "system" && i >= 0 {
			note = m.Content[i:]
		}
	}
	if !strings.Contains(note, "150 triệu") {
		t.Fatalf("c1 next turn must carry the summary note, got messages=%+v", fake.last.Messages)
	}
	if m0 := fake.last.Messages[0]; m0.Role != "system" || !strings.HasSuffix(m0.Content, "\n\n"+note) ||
		!strings.HasPrefix(m0.Content, "Bạn là trợ lý bán hàng.") || fake.last.Messages[1].Role == "system" {
		t.Fatalf("note must be merged into the single system prompt, got %+v", fake.last.Messages[:2])
	}

	// 3) Conversation c2 of the same agent: no note from c1.
	reply("c2", 1)
	for _, m := range fake.last.Messages {
		if strings.Contains(m.Content, pipeline.PreviousSummaryPrefix) {
			t.Fatalf("c2 must not see c1 memory: %q", m.Content)
		}
	}

	// 4) GOSO_MEMORY_RECALL=0 turns the bridge memory off.
	t.Setenv("GOSO_MEMORY_RECALL", "0")
	reply("c1", pipeline.SummarizeMessageMin/2+2)
	for _, m := range fake.last.Messages {
		if strings.Contains(m.Content, pipeline.PreviousSummaryPrefix) {
			t.Fatalf("recall off must not inject: %q", m.Content)
		}
	}
}

func TestMemoryScoped(t *testing.T) {
	cases := []struct {
		name string
		in   ReplyInput
		want bool
	}{
		{"empty", ReplyInput{AgentID: "a"}, false},
		{"session equals agent (omni grpc)", ReplyInput{AgentID: "a", SessionID: "a"}, false},
		{"conversation metadata", ReplyInput{AgentID: "a", Metadata: map[string]string{"conversation_id": "c"}}, true},
		{"camel session metadata", ReplyInput{AgentID: "a", Metadata: map[string]string{"sessionId": "s"}}, true},
		{"distinct session id", ReplyInput{AgentID: "a", SessionID: "s"}, true},
		{"contact id", ReplyInput{AgentID: "a", ContactID: "z1"}, true},
	}
	for _, c := range cases {
		if got := memoryScoped(c.in); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
