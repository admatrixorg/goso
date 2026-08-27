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

func TestSlack_HandleUpdate(t *testing.T) {
	st := store.New()
	var sentDest, sentText string
	s := &Slack{
		Store: st, LLM: llm.Echo{},
		Sender: func(_ context.Context, channelID, text string) error {
			sentDest, sentText = channelID, text
			return nil
		},
	}
	// Fixture: Slack event_callback message.
	body, _ := json.Marshal(map[string]any{
		"type": "event_callback",
		"event": map[string]any{
			"type":    "message",
			"channel": "C123",
			"text":    "hello slack",
		},
	})
	req := httptest.NewRequest("POST", "/api/channels/slack/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if sentDest != "C123" || sentText != "echo: hello slack" {
		t.Fatalf("sent %q %q", sentDest, sentText)
	}
	sessions := st.ListSessions()
	if len(sessions) != 1 || sessions[0].Label != "slack:C123" {
		t.Fatalf("sessions %v", sessions)
	}
}

func TestSlack_IgnoreEmpty(t *testing.T) {
	st := store.New()
	s := &Slack{Store: st, LLM: llm.Echo{}, Sender: func(_ context.Context, _, _ string) error { return nil }}
	req := httptest.NewRequest("POST", "/api/channels/slack/webhook", bytes.NewReader([]byte(`{"type":"url_verification"}`)))
	w := httptest.NewRecorder()
	s.HandleUpdate(w, req)
	if w.Code != 200 || len(st.ListSessions()) != 0 {
		t.Fatalf("expected ignore")
	}
}
