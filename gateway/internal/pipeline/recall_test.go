// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestIsTrivialQuery(t *testing.T) {
	trivial := []string{"", "hi", "chào bạn", "ok cảm ơn", "yes please", "tiếp tục đi", "?"}
	for _, q := range trivial {
		if !IsTrivialQuery(q) {
			t.Fatalf("%q should be trivial", q)
		}
	}
	real := []string{"Alpha thanh toán BankDemo thế nào", "invoice for Acme last month", "giá gói Pro"}
	for _, q := range real {
		if IsTrivialQuery(q) {
			t.Fatalf("%q should not be trivial", q)
		}
	}
}

func TestBuildRecallNote_Admission(t *testing.T) {
	mk := func(body string, score float64) store.RecallHit {
		return store.RecallHit{Memory: store.Memory{Body: body, CreatedAt: time.Now()}, Score: score}
	}
	if note, n := BuildRecallNote(nil, 0); note != "" || n != 0 {
		t.Fatal("no hits → no note")
	}
	hits := []store.RecallHit{mk("best", 10), mk("ok", 6), mk("weak", 2), mk("noise", 1)}
	note, n := BuildRecallNote(hits, 0)
	if n != 2 || !strings.Contains(note, "best") || !strings.Contains(note, "ok") || strings.Contains(note, "weak") {
		t.Fatalf("relative threshold: n=%d note=%q", n, note)
	}
	if !strings.HasPrefix(note, recallHeader) {
		t.Fatalf("note must start with header: %q", note)
	}
	// Item cap.
	var many []store.RecallHit
	for i := 0; i < 10; i++ {
		many = append(many, mk(strings.Repeat("x", 20), 5))
	}
	if _, n := BuildRecallNote(many, 0); n != RecallMaxItems {
		t.Fatalf("cap: n=%d", n)
	}
	// Rune budget: header + one 400-rune item fits, second does not.
	long := strings.Repeat("y", 1000)
	_, n = BuildRecallNote([]store.RecallHit{mk(long, 5), mk(long, 5)}, len([]rune(recallHeader))+420)
	if n != 1 {
		t.Fatalf("budget: n=%d", n)
	}
}

func TestRunner_RecallAcrossSessions(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "")
	st := store.New()
	ag, _ := st.CreateAgent(store.Agent{AgentKey: "sales"})
	other, _ := st.CreateAgent(store.Agent{AgentKey: "support"})
	s1, _ := st.CreateSession(store.Session{AgentID: ag.ID})
	s2, _ := st.CreateSession(store.Session{AgentID: ag.ID})
	s3, _ := st.CreateSession(store.Session{AgentID: other.ID})

	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "đã ghi nhận"}}, RepeatLast: true}
	r := NewRunner(st, nil, scripted, nil)
	r.ForceSummarize = true

	if _, err := r.Run(context.Background(), s1.ID, "Khách Mẫu Thử Alpha thích thanh toán chuyển khoản BankDemo", ModeFull); err != nil {
		t.Fatal(err)
	}
	if sum, _ := st.LatestSummary(s1.ID); sum == nil || !strings.Contains(sum.Body, "BankDemo") {
		t.Fatalf("session 1 summary missing: %+v", sum)
	}

	lastPrompt := func() []llm.Message { return scripted.Recorded[len(scripted.Recorded)-1] }
	recallMsg := func(msgs []llm.Message) (string, int) {
		for i, m := range msgs {
			if m.Role == "system" && strings.Contains(m.Content, recallHeader) {
				return m.Content, i
			}
		}
		return "", -1
	}

	// Same agent, new session, related question → note injected before history.
	if _, err := r.Run(context.Background(), s2.ID, "Alpha muốn thanh toán BankDemo kiểu gì?", ModeFull); err != nil {
		t.Fatal(err)
	}
	note, idx := recallMsg(lastPrompt())
	if idx < 0 || !strings.Contains(note, "BankDemo") {
		t.Fatalf("recall note not injected: %+v", lastPrompt())
	}
	for i, m := range lastPrompt() {
		if m.Role == "user" && i < idx {
			t.Fatalf("recall note must precede history: idx=%d msgs=%+v", idx, lastPrompt())
		}
	}

	// Trivial turn → zero injection.
	if _, err := r.Run(context.Background(), s2.ID, "ok cảm ơn", ModeFull); err != nil {
		t.Fatal(err)
	}
	if _, idx := recallMsg(lastPrompt()); idx >= 0 {
		t.Fatal("trivial turn must not inject memory")
	}

	// Other agent → nothing to recall.
	if _, err := r.Run(context.Background(), s3.ID, "Alpha muốn thanh toán BankDemo kiểu gì?", ModeFull); err != nil {
		t.Fatal(err)
	}
	if _, idx := recallMsg(lastPrompt()); idx >= 0 {
		t.Fatal("other agent must not see sales memory")
	}

	// Kill switch.
	t.Setenv("GOSO_MEMORY_RECALL", "0")
	if _, err := r.Run(context.Background(), s2.ID, "Alpha thanh toán BankDemo?", ModeFull); err != nil {
		t.Fatal(err)
	}
	if _, idx := recallMsg(lastPrompt()); idx >= 0 {
		t.Fatal("GOSO_MEMORY_RECALL=0 must disable recall")
	}
}

func TestPruneTTL(t *testing.T) {
	t.Setenv("GOSO_MEMORY_TTL_DAYS", "")
	if PruneTTL() != PruneDefaultDays*24*time.Hour {
		t.Fatal("default ttl")
	}
	t.Setenv("GOSO_MEMORY_TTL_DAYS", "0")
	if PruneTTL() != 0 {
		t.Fatal("0 disables")
	}
	t.Setenv("GOSO_MEMORY_TTL_DAYS", "7")
	if PruneTTL() != 7*24*time.Hour {
		t.Fatal("7 days")
	}
	st := store.New()
	if n, err := PruneOnce(st); err != nil || n != 0 {
		t.Fatalf("prune empty store: %d %v", n, err)
	}
}

func TestMemoryExpand_SupersededIsNotCurrent(t *testing.T) {
	st := store.New()
	e, err := st.PutKGEntity(store.KGEntity{Name: "Old Price", Kind: "fact", Body: "gói Pro 500k"})
	if err != nil {
		t.Fatal(err)
	}
	call := llm.ToolCall{ID: "x", Name: ToolMemoryExpand, Arguments: map[string]any{"id": e.ID}}
	if out, err := DispatchMemoryTool(st, store.DefaultTenant, call); err != nil || !strings.Contains(out, "Old Price") {
		t.Fatalf("current entity must expand: %v %s", err, out)
	}
	if err := st.SupersedeKGEntity(store.DefaultTenant, e.ID, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if out, err := DispatchMemoryTool(st, store.DefaultTenant, call); err == nil || !strings.Contains(out, "superseded") {
		t.Fatalf("superseded entity must not be served to the agent: %v %s", err, out)
	}
}

// The recall note must reach the model inside the single system message,
// after the agent's own prompt and before the history: 9router/gcli drops
// every system message after the first.
func TestRunner_RecallRidesInOneSystemMessageAfterAgentPrompt(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "")
	st := store.New()
	ag, _ := st.CreateAgent(store.Agent{AgentKey: "sale-shadow", Instructions: "Bạn chỉ tư vấn nội thất."})
	s1, _ := st.CreateSession(store.Session{AgentID: ag.ID})
	s2, _ := st.CreateSession(store.Session{AgentID: ag.ID})

	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "đã ghi nhận"}}, RepeatLast: true}
	r := NewRunner(st, nil, scripted, nil)
	r.ForceSummarize = true
	if _, err := r.Run(context.Background(), s1.ID, "Khách Mẫu muốn sofa gỗ sồi màu walnut, mã đơn SOFA-7731", ModeFull); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), s2.ID, "Đơn SOFA-7731 sofa gỗ sồi màu gì?", ModeFull); err != nil {
		t.Fatal(err)
	}
	msgs := scripted.Recorded[len(scripted.Recorded)-1]
	systems := 0
	for i, m := range msgs {
		if m.Role == "system" {
			systems++
			if i != 0 {
				t.Fatalf("system message at %d, want only index 0: %+v", i, msgs)
			}
		}
	}
	if systems != 1 {
		t.Fatalf("want exactly one system message, got %d: %+v", systems, msgs)
	}
	sys := msgs[0].Content
	ins := strings.Index(sys, "Bạn chỉ tư vấn nội thất.")
	hdr := strings.Index(sys, recallHeader)
	body := strings.Index(sys, "SOFA-7731")
	if ins < 0 || hdr < 0 || body < 0 || !(ins < hdr && hdr < body) {
		t.Fatalf("want agent prompt, then recall header, then note; ins=%d hdr=%d body=%d sys=%q", ins, hdr, body, sys)
	}
	if last := msgs[len(msgs)-1]; last.Role != "user" || !strings.Contains(last.Content, "SOFA-7731") {
		t.Fatalf("last message must be the user turn: %+v", last)
	}
}
