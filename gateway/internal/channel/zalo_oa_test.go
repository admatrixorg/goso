// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestZaloOA_HandleUpdate(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_ZALO_OA_SECRET", "")
	st := store.New()
	_ = st.PutChannelConfig(store.ChannelConfig{Name: "zalo-oa", DMPolicy: "open"})
	var sentUserID, sentText string
	z := &ZaloOA{
		Store: st, LLM: llm.Echo{},
		Sender: func(_ context.Context, userID, text string) error {
			sentUserID, sentText = userID, text
			return nil
		},
	}
	body, _ := json.Marshal(map[string]any{
		"sender":  map[string]any{"id": "u123"},
		"message": map[string]any{"text": "hello oa"},
	})
	req := httptest.NewRequest("POST", "/api/channels/zalo-oa/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	z.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if sentUserID != "u123" || sentText != "echo: hello oa" {
		t.Fatalf("sent %q %q", sentUserID, sentText)
	}
	sessions := st.ListSessions()
	if len(sessions) != 1 || sessions[0].Label != "zalo-oa:u123" {
		t.Fatalf("sessions %v", sessions)
	}
	msgs, _ := st.ListMessages(sessions[0].ID)
	if len(msgs) != 2 {
		t.Fatalf("messages %v", msgs)
	}
}

func TestZaloOA_UserIDFallback(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_ZALO_OA_SECRET", "")
	st := store.New()
	_ = st.PutChannelConfig(store.ChannelConfig{Name: "zalo-oa", DMPolicy: "open"})
	z := &ZaloOA{Store: st, LLM: llm.Echo{}, Sender: func(_ context.Context, _, _ string) error { return nil }}
	body, _ := json.Marshal(map[string]any{
		"user_id": "u999",
		"message": map[string]any{"text": "hi"},
	})
	req := httptest.NewRequest("POST", "/api/channels/zalo-oa/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	z.HandleUpdate(w, req)
	if w.Code != 200 || len(st.ListSessions()) != 1 {
		t.Fatalf("status %d sessions %d", w.Code, len(st.ListSessions()))
	}
}

func TestZaloOA_VerifyMatrix(t *testing.T) {
	st := store.New()
	z := &ZaloOA{Store: st, LLM: llm.Echo{}, Sender: func(_ context.Context, _, _ string) error { return nil }}
	body := `{"user_id":"u1","message":{"text":"hi"}}`
	t.Setenv("GOSO_ZALO_OA_SECRET", "sec")
	t.Setenv("GOSO_ENV", "demo")
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	w := httptest.NewRecorder()
	z.HandleUpdate(w, req)
	if w.Code != 401 {
		t.Fatalf("secret set missing header %d", w.Code)
	}
	_ = st.PutChannelConfig(store.ChannelConfig{Name: "zalo-oa", DMPolicy: "open"})
	req = httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("X-Goso-OA-Secret", "sec")
	w = httptest.NewRecorder()
	z.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("good secret %d", w.Code)
	}
	t.Setenv("GOSO_ZALO_OA_SECRET", "")
	t.Setenv("GOSO_ENV", "production")
	req = httptest.NewRequest("POST", "/", strings.NewReader(body))
	w = httptest.NewRecorder()
	z.HandleUpdate(w, req)
	if w.Code != 401 {
		t.Fatalf("prod no secret %d", w.Code)
	}
}

func TestZaloOA_IgnoreEmpty(t *testing.T) {
	st := store.New()
	z := &ZaloOA{Store: st, LLM: llm.Echo{}, Sender: func(_ context.Context, _, _ string) error { return nil }}
	req := httptest.NewRequest("POST", "/api/channels/zalo-oa/webhook", bytes.NewReader([]byte(`{"event_name":"user_send_text"}`)))
	w := httptest.NewRecorder()
	z.HandleUpdate(w, req)
	if w.Code != 200 || len(st.ListSessions()) != 0 {
		t.Fatalf("expected ignore")
	}
}

func TestZaloOA_ResolvesAgentModel(t *testing.T) {
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_ZALO_OA_SECRET", "")
	base, gotModel := startChatCapture(t)
	setRouter9Capture(t, base)
	st := store.New()
	if err := st.PutChannelConfig(store.ChannelConfig{Name: "zalo-oa", DMPolicy: "open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAgent(store.Agent{
		AgentKey: "zalo-oa", DisplayName: "Zalo OA",
		LLMProvider: "router9", Model: agentModel,
	}); err != nil {
		t.Fatal(err)
	}
	var sentText string
	z := &ZaloOA{
		Store: st,
		LLM:   fallbackOpenAI(base, fallbackModel),
		Sender: func(_ context.Context, _, text string) error {
			sentText = text
			return nil
		},
	}
	body, _ := json.Marshal(map[string]any{
		"sender":  map[string]any{"id": "u123"},
		"message": map[string]any{"text": "hello grok"},
	})
	req := httptest.NewRequest("POST", "/api/channels/zalo-oa/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	z.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if sentText != "from-compat" {
		t.Fatalf("text %q", sentText)
	}
	if *gotModel != agentModel {
		t.Fatalf("model %q want %q (must not use fallback %q)", *gotModel, agentModel, fallbackModel)
	}
}
