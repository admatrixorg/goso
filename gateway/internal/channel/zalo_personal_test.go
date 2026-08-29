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

func TestZaloPersonal_HandleUpdate(t *testing.T) {
	st := store.New()
	_ = st.PutChannelConfig(store.ChannelConfig{Name: "zalo-personal", DMPolicy: "allowlist", AllowFrom: []string{"t123"}})
	var sentThread, sentText string
	z := &ZaloPersonal{
		Store: st, LLM: llm.Echo{},
		Sender: func(_ context.Context, threadID, text string) error {
			sentThread, sentText = threadID, text
			return nil
		},
	}
	body, _ := json.Marshal(map[string]any{
		"thread_id": "t123",
		"message":   map[string]any{"text": "hi personal"},
	})
	req := httptest.NewRequest("POST", "/api/channels/zalo-personal/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	z.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if sentThread != "t123" || sentText != "echo: hi personal" {
		t.Fatalf("sent %q %q", sentThread, sentText)
	}
	sessions := st.ListSessions()
	if len(sessions) != 1 || sessions[0].Label != "zalo-personal:t123" {
		t.Fatalf("sessions %v", sessions)
	}
}

func TestZaloPersonal_FromIDFallback(t *testing.T) {
	st := store.New()
	_ = st.PutChannelConfig(store.ChannelConfig{Name: "zalo-personal", DMPolicy: "allowlist", AllowFrom: []string{"f999"}})
	z := &ZaloPersonal{Store: st, LLM: llm.Echo{}, Sender: func(_ context.Context, _, _ string) error { return nil }}
	body, _ := json.Marshal(map[string]any{
		"from_id": "f999",
		"message": map[string]any{"text": "hello"},
	})
	req := httptest.NewRequest("POST", "/api/channels/zalo-personal/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	z.HandleUpdate(w, req)
	if w.Code != 200 || len(st.ListSessions()) != 1 {
		t.Fatalf("from_id fallback: %d %d", w.Code, len(st.ListSessions()))
	}
}
