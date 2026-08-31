// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestDiscord_HandleUpdate(t *testing.T) {
	st := store.New()
	var sentDest, sentText string
	d := &Discord{
		Store: st, LLM: llm.Echo{},
		Sender: func(_ context.Context, channelID, text string) error {
			sentDest, sentText = channelID, text
			return nil
		},
	}
	// Fixture: Discord MESSAGE_CREATE-shaped JSON.
	body, _ := json.Marshal(map[string]any{
		"channel_id": "c123",
		"content":    "hello discord",
	})
	req := httptest.NewRequest("POST", "/api/channels/discord/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	d.HandleUpdate(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if sentDest != "c123" || sentText != "echo: hello discord" {
		t.Fatalf("sent %q %q", sentDest, sentText)
	}
	sessions := st.ListSessions()
	if len(sessions) != 1 || sessions[0].Label != "discord:c123" {
		t.Fatalf("sessions %v", sessions)
	}
}

func TestDiscord_GatewayWrap(t *testing.T) {
	st := store.New()
	d := &Discord{Store: st, LLM: llm.Echo{}, Sender: func(_ context.Context, _, _ string) error { return nil }}
	body, _ := json.Marshal(map[string]any{
		"t": "MESSAGE_CREATE",
		"d": map[string]any{"channel_id": "c9", "content": "hi"},
	})
	req := httptest.NewRequest("POST", "/api/channels/discord/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	d.HandleUpdate(w, req)
	if w.Code != 200 || len(st.ListSessions()) != 1 || st.ListSessions()[0].Label != "discord:c9" {
		t.Fatalf("wrap: %d sessions %v", w.Code, st.ListSessions())
	}
}

func TestDiscord_ResolvesAgentModel(t *testing.T) {
	base, gotModel := startChatCapture(t)
	setRouter9Capture(t, base)
	st := store.New()
	if _, err := st.CreateAgent(store.Agent{
		AgentKey: "discord", DisplayName: "Discord",
		LLMProvider: "router9", Model: agentModel,
	}); err != nil {
		t.Fatal(err)
	}
	var sentText string
	d := &Discord{
		Store: st,
		LLM:   fallbackOpenAI(base, fallbackModel),
		Sender: func(_ context.Context, _, text string) error {
			sentText = text
			return nil
		},
	}
	body, _ := json.Marshal(map[string]any{
		"channel_id": "c123",
		"content":    "hello grok",
	})
	req := httptest.NewRequest("POST", "/api/channels/discord/webhook", bytes.NewReader(body))
	w := httptest.NewRecorder()
	d.HandleUpdate(w, req)
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

func TestDiscord_SendHttptest(t *testing.T) {
	var gotAuth, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"m1"}`))
	}))
	t.Cleanup(srv.Close)
	d := &Discord{BotToken: "test-placeholder", APIBase: srv.URL, HTTPClient: srv.Client()}
	if err := d.sendMessage(context.Background(), "c1", "hi"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bot test-placeholder" || gotPath != "/channels/c1/messages" {
		t.Fatalf("auth %q path %q", gotAuth, gotPath)
	}
	if !bytes.Contains([]byte(gotBody), []byte(`"content":"hi"`)) {
		t.Fatalf("body %s", gotBody)
	}
}

func TestDiscord_BadJSON(t *testing.T) {
	d := &Discord{Store: store.New()}
	req := httptest.NewRequest("POST", "/api/channels/discord/webhook", bytes.NewReader([]byte("{bad")))
	w := httptest.NewRecorder()
	d.HandleUpdate(w, req)
	if w.Code != 400 {
		t.Fatalf("status %d", w.Code)
	}
}
