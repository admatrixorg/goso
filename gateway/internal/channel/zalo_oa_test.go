// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestZaloOA_HandleUpdate(t *testing.T) {
	st := store.New()
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
	st := store.New()
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
