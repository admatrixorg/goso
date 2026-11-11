// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/contextfile"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	minhAnRule = "Chiết khấu tối đa của Minh An là 9 phần trăm."
	minhAnNote = "Lan chỉ nhận báo giá một dòng."
	minhAnHoa  = "Hoa giữ bản báo giá dài."
	minhAnIns  = "Tra loi ngan gon."
)

type answerFromFile struct {
	seen []llm.Message
}

func (a *answerFromFile) Name() string      { return "obey" }
func (a *answerFromFile) ModelName() string { return "obey" }

func (a *answerFromFile) Chat(_ context.Context, msgs []llm.Message) (string, error) {
	a.seen = append([]llm.Message(nil), msgs...)
	sys := systemText(msgs)
	if strings.Contains(sys, minhAnRule) && strings.Contains(sys, minhAnNote) {
		return minhAnRule + " " + minhAnNote, nil
	}
	if strings.Contains(sys, minhAnRule) {
		return minhAnRule, nil
	}
	return "không rõ", nil
}

func systemText(msgs []llm.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Role == "system" {
			b.WriteString(m.Content)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

type listFail struct {
	store.StoreIface
	calls int
}

func (f *listFail) ListContextFiles(tenant, scope, owner string) ([]*store.ContextFile, error) {
	f.calls++
	return nil, errors.New("disk I/O")
}

func TestRun_ContextFileAnswersForTenantUser(t *testing.T) {
	st := store.New()
	ag, err := st.CreateAgent(store.Agent{
		AgentKey: "tu-van-minh-an", DisplayName: "Cố vấn Minh An",
		TenantID: "minh-an-sales", Instructions: minhAnIns,
	})
	if err != nil {
		t.Fatal(err)
	}
	legal, err := st.CreateAgent(store.Agent{
		AgentKey: "phap-che-minh-an", DisplayName: "Pháp chế Minh An",
		TenantID: "minh-an-legal", Instructions: minhAnIns,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutContextFile(store.ContextFile{
		TenantID: "minh-an-sales", Scope: store.ContextScopeAgent, AgentID: ag.ID,
		Name: "AGENTS.md", Body: minhAnRule,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutContextFile(store.ContextFile{
		TenantID: "minh-an-sales", Scope: store.ContextScopeUser, UserKey: "lan",
		Name: "USER.md", Body: minhAnNote,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutContextFile(store.ContextFile{
		TenantID: "minh-an-sales", Scope: store.ContextScopeUser, UserKey: "hoa",
		Name: "USER.md", Body: minhAnHoa,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutContextFile(store.ContextFile{
		TenantID: "minh-an-legal", Scope: store.ContextScopeAgent, AgentID: legal.ID,
		Name: "AGENTS.md", Body: "Pháp chế Minh An không báo chiết khấu.",
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: ag.ID, TenantID: "minh-an-sales"})
	if err != nil {
		t.Fatal(err)
	}
	legalSess, err := st.CreateSession(store.Session{AgentID: legal.ID, TenantID: "minh-an-legal"})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("flag off leaves the prompt alone", func(t *testing.T) {
		t.Setenv("GOSO_CONTEXT_FILES", "")
		fail := &listFail{StoreIface: st}
		p := &answerFromFile{}
		r := NewRunner(fail, nil, p, nil)
		res, err := r.Run(contextfile.WithUser(context.Background(), "lan"), sess.ID, "Chiết khấu tối đa là bao nhiêu?", ModeFull)
		if err != nil {
			t.Fatal(err)
		}
		if fail.calls != 0 {
			t.Fatalf("flag off read the store %d times", fail.calls)
		}
		sys := systemText(p.seen)
		if strings.Contains(sys, minhAnRule) || strings.Contains(sys, minhAnNote) {
			t.Fatalf("flag off injected files: %q", sys)
		}
		if !strings.Contains(sys, minhAnIns) {
			t.Fatalf("instructions missing: %q", sys)
		}
		if res.Reply != "không rõ" {
			t.Fatalf("reply %q", res.Reply)
		}
	})

	t.Run("flag on answers from the agent and user files", func(t *testing.T) {
		t.Setenv("GOSO_CONTEXT_FILES", "1")
		p := &answerFromFile{}
		r := NewRunner(st, nil, p, nil)
		res, err := r.Run(contextfile.WithUser(context.Background(), "lan"), sess.ID, "Chiết khấu tối đa là bao nhiêu?", ModeFull)
		if err != nil {
			t.Fatal(err)
		}
		sys := systemText(p.seen)
		if !strings.Contains(sys, "AGENTS.md:") || !strings.Contains(sys, minhAnRule) {
			t.Fatalf("agent file missing: %q", sys)
		}
		if !strings.Contains(sys, "USER.md:") || !strings.Contains(sys, minhAnNote) {
			t.Fatalf("user file missing: %q", sys)
		}
		if strings.Contains(sys, minhAnHoa) || strings.Contains(sys, "không báo chiết khấu") {
			t.Fatalf("other owner leaked: %q", sys)
		}
		if !strings.Contains(sys, minhAnIns) || !strings.Contains(sys, "Cố vấn Minh An") {
			t.Fatalf("existing prompt dropped: %q", sys)
		}
		if res.Reply != minhAnRule+" "+minhAnNote {
			t.Fatalf("reply %q", res.Reply)
		}
		fresh, err := st.GetAgent(ag.ID)
		if err != nil || fresh.Instructions != minhAnIns || fresh.DisplayName != "Cố vấn Minh An" {
			t.Fatalf("agent mutated: %+v %v", fresh, err)
		}
	})

	t.Run("other user and other tenant", func(t *testing.T) {
		t.Setenv("GOSO_CONTEXT_FILES", "1")
		p := &answerFromFile{}
		r := NewRunner(st, nil, p, nil)
		if _, err := r.Run(contextfile.WithUser(context.Background(), "hoa"), sess.ID, "Chiết khấu tối đa là bao nhiêu?", ModeFull); err != nil {
			t.Fatal(err)
		}
		sys := systemText(p.seen)
		if !strings.Contains(sys, minhAnHoa) || strings.Contains(sys, minhAnNote) {
			t.Fatalf("user scope: %q", sys)
		}
		p2 := &answerFromFile{}
		r2 := NewRunner(st, nil, p2, nil)
		res, err := r2.Run(contextfile.WithUser(context.Background(), "lan"), legalSess.ID, "Chiết khấu tối đa là bao nhiêu?", ModeFull)
		if err != nil {
			t.Fatal(err)
		}
		sys = systemText(p2.seen)
		if strings.Contains(sys, minhAnRule) || strings.Contains(sys, minhAnNote) {
			t.Fatalf("legal saw sales files: %q", sys)
		}
		if !strings.Contains(sys, "không báo chiết khấu") {
			t.Fatalf("legal file missing: %q", sys)
		}
		if res.Reply != "không rõ" {
			t.Fatalf("legal reply %q", res.Reply)
		}
	})

	t.Run("mode none skips files", func(t *testing.T) {
		t.Setenv("GOSO_CONTEXT_FILES", "1")
		fail := &listFail{StoreIface: st}
		p := &answerFromFile{}
		r := NewRunner(fail, nil, p, nil)
		if _, err := r.Run(contextfile.WithUser(context.Background(), "lan"), sess.ID, "Chiết khấu tối đa là bao nhiêu?", ModeNone); err != nil {
			t.Fatal(err)
		}
		if fail.calls != 0 {
			t.Fatalf("mode none read files %d", fail.calls)
		}
		if strings.Contains(systemText(p.seen), minhAnRule) {
			t.Fatal("mode none injected the file")
		}
	})

	t.Run("store failure is returned", func(t *testing.T) {
		t.Setenv("GOSO_CONTEXT_FILES", "1")
		fail := &listFail{StoreIface: st}
		p := &answerFromFile{}
		r := NewRunner(fail, nil, p, nil)
		fresh, err := st.CreateSession(store.Session{AgentID: ag.ID, TenantID: "minh-an-sales"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = r.Run(context.Background(), fresh.ID, "Chiết khấu tối đa là bao nhiêu?", ModeFull)
		if !errors.Is(err, contextfile.ErrUnavailable) {
			t.Fatalf("err %v", err)
		}
		if fail.calls == 0 {
			t.Fatal("store was not called")
		}
		if len(p.seen) != 0 {
			t.Fatal("model ran after a store failure")
		}
		msgs, err := st.ListMessages(fresh.ID)
		if err != nil || len(msgs) != 0 {
			t.Fatalf("partial turn stored: %v %+v", err, msgs)
		}
	})
}
