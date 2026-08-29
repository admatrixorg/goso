// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestTelegram_HandleUpdate_EchoAndStore(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_TELEGRAM_WEBHOOK_SECRET", "")
	st := store.New()
	var sentChatID int64
	var sentText string
	tg := &Telegram{
		Store: st,
		LLM:   llm.Echo{},
		Sender: func(_ context.Context, chatID int64, text string) error {
			sentChatID = chatID
			sentText = text
			return nil
		},
	}
	body, _ := json.Marshal(map[string]any{
		"update_id": 1,
		"message": map[string]any{
			"message_id": 1,
			"chat":       map[string]any{"id": 12345},
			"text":       "hello bot",
		},
	})
	req := httptest.NewRequest("POST", "/api/channels/telegram/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	tg.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if sentChatID != 12345 {
		t.Fatalf("chat_id %d", sentChatID)
	}
	if sentText != "echo: hello bot" {
		t.Fatalf("text %q", sentText)
	}
	// messages persisted
	sessions := st.ListSessions()
	if len(sessions) != 1 {
		t.Fatalf("sessions %d", len(sessions))
	}
	msgs, _ := st.ListMessages(sessions[0].ID)
	if len(msgs) != 2 || msgs[0].Content != "hello bot" || msgs[1].Content != "echo: hello bot" {
		t.Fatalf("messages %v", msgs)
	}
}

func TestTelegram_RecordsUsage(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_TELEGRAM_WEBHOOK_SECRET", "")
	st := store.New()
	meter := billing.New()
	tg := &Telegram{
		Store: st,
		LLM:   llm.Echo{},
		Meter: meter,
		Sender: func(_ context.Context, _ int64, _ string) error {
			return nil
		},
	}
	body, _ := json.Marshal(map[string]any{
		"update_id": 1,
		"message": map[string]any{
			"message_id": 1,
			"chat":       map[string]any{"id": 99},
			"text":       "abcd",
		},
	})
	req := httptest.NewRequest("POST", "/api/channels/telegram/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	tg.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	sum := meter.Query(billing.Query{Provider: "echo"})
	if sum.Calls != 1 || sum.TotalTokens < 1 {
		t.Fatalf("usage %+v", sum)
	}
	sessions := st.ListSessions()
	if len(sessions) != 1 || sum.AgentID == "" && sessions[0].AgentID == "" {
		t.Fatalf("agent linkage sessions=%v usage=%+v", sessions, sum)
	}
	got := meter.Query(billing.Query{AgentID: sessions[0].AgentID})
	if got.Calls != 1 {
		t.Fatalf("by agent %+v", got)
	}
}

func TestTelegram_HandleUpdate_IgnoresEmpty(t *testing.T) {
	st := store.New()
	tg := &Telegram{Store: st, LLM: llm.Echo{}, Sender: func(_ context.Context, _ int64, _ string) error { return nil }}
	body := `{"update_id":1}`
	req := httptest.NewRequest("POST", "/api/channels/telegram/webhook", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	tg.HandleUpdate(w, req)
	if w.Code != 200 || len(st.ListSessions()) != 0 {
		t.Fatalf("expected ignore empty: %d sessions %d", w.Code, len(st.ListSessions()))
	}
}

func TestTelegram_HandleUpdate_BadJSON(t *testing.T) {
	st := store.New()
	tg := &Telegram{Store: st}
	req := httptest.NewRequest("POST", "/api/channels/telegram/webhook", bytes.NewReader([]byte("{bad")))
	w := httptest.NewRecorder()
	tg.HandleUpdate(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTelegram_DisabledAgentBuffers(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_TELEGRAM_WEBHOOK_SECRET", "")
	prev := DefaultPending()
	buf := NewPending()
	SetDefaultPending(buf)
	t.Cleanup(func() { SetDefaultPending(prev) })

	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "telegram", DisplayName: "Telegram Bot"})
	if err != nil {
		t.Fatal(err)
	}
	off := *a
	off.Enabled = false
	off.UpdatedAt = a.Stamp()
	if _, err := st.UpdateAgent(off); err != nil {
		t.Fatal(err)
	}
	sent := 0
	tg := &Telegram{
		Store: st,
		LLM:   llm.Echo{},
		Sender: func(_ context.Context, _ int64, _ string) error {
			sent++
			return nil
		},
	}
	body, _ := json.Marshal(map[string]any{
		"update_id": 1,
		"message": map[string]any{
			"message_id": 1,
			"chat":       map[string]any{"id": 777},
			"text":       "bot_token=12345:AA hold this",
		},
	})
	req := httptest.NewRequest("POST", "/api/channels/telegram/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	tg.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	if sent != 0 {
		t.Fatalf("must not send while buffered, sent %d", sent)
	}
	if len(st.ListSessions()) != 0 {
		t.Fatalf("must not persist session while buffered")
	}
	list := buf.List("", time.Time{})
	if len(list) != 1 || list[0].Count != 1 || list[0].Dest != "777" {
		t.Fatalf("buffer %#v", list)
	}
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), "bot_token") || strings.Contains(string(raw), "12345:AA") {
		t.Fatalf("payload in listing %s", raw)
	}
}
