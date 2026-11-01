// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestChatWithTurn_MentionDoesNotChangeSessionOrEnterEvent(t *testing.T) {
	st := store.New()
	a, _ := st.CreateAgent(store.Agent{AgentKey: "a", DisplayName: "Đầu tư", Model: "grok-4.5", Enabled: true})
	b, _ := st.CreateAgent(store.Agent{AgentKey: "b", DisplayName: "Pháp chế", Model: "gemini-2.5-flash", Enabled: true})
	paused, _ := st.CreateAgent(store.Agent{AgentKey: "p", DisplayName: "Paused"})
	paused.Enabled = false
	paused.UpdatedAt = time.Time{}
	if _, err := st.UpdateAgent(*paused); err != nil {
		t.Fatal(err)
	}
	sess, _ := st.CreateSession(store.Session{AgentID: a.ID})
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "ok-a"}, {Text: "ok-b"}}}
	rt := New(st, connector.NewRegistry(), approval.New(0), eventstore.New(64), scripted)

	if _, err := rt.Chat(context.Background(), sess.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ChatWithTurn(context.Background(), sess.ID, "@Pháp chế xem lại", "", false, ChatTurn{MentionAgentID: b.ID}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetSession(sess.ID)
	if got.AgentID != a.ID {
		t.Fatalf("session agent changed to %s", got.AgentID)
	}
	if len(scripted.Recorded) < 2 {
		t.Fatalf("recorded %d", len(scripted.Recorded))
	}
	blob := scripted.Recorded[1][0].Content
	if !strings.Contains(blob, "Pháp chế") {
		t.Fatalf("mention persona missing: %s", blob)
	}
	if strings.Contains(scripted.Recorded[0][0].Content, "Pháp chế") {
		t.Fatalf("first turn used mention persona: %s", scripted.Recorded[0][0].Content)
	}
	msgs, _ := st.ListMessages(sess.ID)
	var userMention bool
	for _, m := range msgs {
		if m.Role == "event" {
			t.Fatalf("event leaked into turn store %#v", m)
		}
		if m.Role == "user" && m.MentionAgentID == b.ID && m.AgentID == a.ID {
			userMention = true
		}
		if m.Role == "assistant" && m.AgentID == b.ID && m.Model != "scripted" {
			t.Fatalf("caption model %#v", m)
		}
	}
	if !userMention {
		t.Fatalf("user mention stamp %#v", msgs)
	}
	if _, err := rt.ChatWithTurn(context.Background(), sess.ID, "no", "", false, ChatTurn{MentionAgentID: paused.ID}, nil); err != ErrAgentInactive {
		t.Fatalf("paused mention %v", err)
	}
	if _, err := rt.ChatWithTurn(context.Background(), sess.ID, "no", "", false, ChatTurn{MentionAgentID: "missing"}, nil); err != ErrMentionNotFound {
		t.Fatalf("missing mention %v", err)
	}

	upd := *got
	upd.AgentID = b.ID
	if _, err := st.UpdateSession(upd); err != nil {
		t.Fatal(err)
	}
	_, _ = st.AddMessage(store.Message{SessionID: sess.ID, Role: "event", AgentID: b.ID})
	scripted.Recorded = nil
	if _, err := rt.Chat(context.Background(), sess.ID, "after switch"); err != nil {
		t.Fatal(err)
	}
	for _, m := range scripted.Recorded[0] {
		if m.Role == "event" {
			t.Fatalf("event sent to model %#v", scripted.Recorded[0])
		}
	}
}
