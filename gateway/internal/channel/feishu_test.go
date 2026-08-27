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

func TestFeishu_HandleUpdate(t *testing.T) {
	st := store.New()
	var sentDest, sentText string
	f := &Feishu{
		Store: st, LLM: llm.Echo{},
		Sender: func(_ context.Context, chatID, text string) error {
			sentDest, sentText = chatID, text
			return nil
		},
	}
	// Fixture: Feishu im.message.receive_v1 with JSON-string content.
	body, _ := json.Marshal(map[string]any{
		"header": map[string]any{"event_type": "im.message.receive_v1"},
		"event": map[string]any{
			"message": map[string]any{
				"chat_id": "oc1",
				"content": `{"text":"hello feishu"}`,
			},
		},
	})
	req := httptest.NewRequest("POST", "/api/channels/feishu/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	f.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if sentDest != "oc1" || sentText != "echo: hello feishu" {
		t.Fatalf("sent %q %q", sentDest, sentText)
	}
	sessions := st.ListSessions()
	if len(sessions) != 1 || sessions[0].Label != "feishu:oc1" {
		t.Fatalf("sessions %v", sessions)
	}
}

func TestFeishu_Flat(t *testing.T) {
	st := store.New()
	f := &Feishu{Store: st, LLM: llm.Echo{}, Sender: func(_ context.Context, _, _ string) error { return nil }}
	body, _ := json.Marshal(map[string]any{"chat_id": "oc9", "text": "hi"})
	req := httptest.NewRequest("POST", "/api/channels/feishu/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	f.HandleUpdate(w, req)
	if w.Code != 200 || len(st.ListSessions()) != 1 {
		t.Fatalf("flat: %d %d", w.Code, len(st.ListSessions()))
	}
}
