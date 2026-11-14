// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func userSaved(t *testing.T, st store.StoreIface, sessionID, text string) {
	t.Helper()
	msgs, err := st.ListMessages(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if m.Role == "user" && m.Content == text {
			return
		}
	}
	t.Fatalf("user message %q missing from %s: %#v", text, sessionID, msgs)
}

func TestAnswerTurn_SavesUserTextWhenArchived(t *testing.T) {
	st := store.New()
	ag, err := st.CreateAgent(store.Agent{AgentKey: "telegram", DisplayName: "Telegram Bot"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: ag.ID, Label: "telegram:1"})
	if err != nil {
		t.Fatal(err)
	}
	upd := *sess
	upd.Archived = true
	upd.TouchArchive = true
	if _, err := st.UpdateSession(upd); err != nil {
		t.Fatal(err)
	}
	sess, _ = st.GetSession(sess.ID)
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "model-ok"}}}
	const text = "khach gui tin archived"
	reply := answerTurn(context.Background(), st, ag, sess, scripted, nil, text)
	if reply == "" {
		t.Fatal("live archived session skipped the model")
	}
	userSaved(t, st, sess.ID, text)
}

func TestAnswerTurn_SavesUserTextWhenAgentDisabled(t *testing.T) {
	st := store.New()
	ag, err := st.CreateAgent(store.Agent{AgentKey: "zalo-oa", DisplayName: "Zalo OA"})
	if err != nil {
		t.Fatal(err)
	}
	ag.Enabled = false
	ag.UpdatedAt = time.Time{}
	if _, err := st.UpdateAgent(*ag); err != nil {
		t.Fatal(err)
	}
	ag, _ = st.GetAgent(ag.ID)
	sess, err := st.CreateSession(store.Session{AgentID: ag.ID, Label: "zalo-oa:u1"})
	if err != nil {
		t.Fatal(err)
	}
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "model-ok"}}}
	const text = "khach gui tin disabled"
	reply := answerTurn(context.Background(), st, ag, sess, scripted, nil, text)
	if reply == "" {
		t.Fatal("disabled agent skipped the model from answerTurn")
	}
	userSaved(t, st, sess.ID, text)
}

func TestTelegram_ArchivedSessionStillSavesInbound(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_TELEGRAM_WEBHOOK_SECRET", "")
	st := store.New()
	ag, err := st.CreateAgent(store.Agent{AgentKey: "telegram", DisplayName: "Telegram Bot"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: ag.ID, Label: "telegram:12345"})
	if err != nil {
		t.Fatal(err)
	}
	upd := *sess
	upd.Archived = true
	upd.TouchArchive = true
	if _, err := st.UpdateSession(upd); err != nil {
		t.Fatal(err)
	}
	if err := st.PutChannelConfig(store.ChannelConfig{Name: "telegram", AgentID: ag.ID, Enabled: true, DMPolicy: "open"}); err != nil {
		t.Fatal(err)
	}
	// A disabled import placeholder is not the telegram agent. Real traffic
	// must not be written onto it.
	placeholder, err := st.CreateAgent(store.Agent{AgentKey: "import-unresolved.minh-an", DisplayName: "Unresolved import"})
	if err != nil {
		t.Fatal(err)
	}
	placeholder.Enabled = false
	placeholder.UpdatedAt = time.Time{}
	if _, err := st.UpdateAgent(*placeholder); err != nil {
		t.Fatal(err)
	}
	held, err := st.CreateSession(store.Session{AgentID: placeholder.ID, Label: "Minh An khong agent"})
	if err != nil {
		t.Fatal(err)
	}
	heldUpd := *held
	heldUpd.Archived = true
	heldUpd.TouchArchive = true
	if _, err := st.UpdateSession(heldUpd); err != nil {
		t.Fatal(err)
	}
	var sent string
	tg := &Telegram{
		Store: st,
		LLM:   llm.Echo{},
		Sender: func(_ context.Context, _ int64, text string) error {
			sent = text
			return nil
		},
	}
	body, _ := json.Marshal(map[string]any{
		"update_id": 1,
		"message": map[string]any{
			"message_id": 1,
			"chat":       map[string]any{"id": 12345},
			"text":       "tin that cua khach",
		},
	})
	req := httptest.NewRequest("POST", "/api/channels/telegram/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	tg.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	if sent == "" {
		t.Fatal("archived live session produced no reply")
	}
	userSaved(t, st, sess.ID, "tin that cua khach")
	heldMsgs, err := st.ListMessages(held.ID)
	if err != nil || len(heldMsgs) != 0 {
		t.Fatalf("placeholder session received traffic: %#v %v", heldMsgs, err)
	}
}
