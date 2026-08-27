// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func wsURL(t *testing.T, h http.Handler) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	return u, srv.Close
}

func TestWS_PingPongAndChat(t *testing.T) {
	st := store.New()
	mux := http.NewServeMux()
	RegisterWS(mux, st, llm.Echo{})
	u, closer := wsURL(t, mux)
	t.Cleanup(closer)

	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if err := c.WriteJSON(map[string]any{"op": "ping"}); err != nil {
		t.Fatal(err)
	}
	var pong wsFrame
	if err := c.ReadJSON(&pong); err != nil {
		t.Fatal(err)
	}
	if pong.Op != "pong" {
		t.Fatalf("pong %+v", pong)
	}

	agent, err := st.CreateAgent(store.Agent{AgentKey: "ws", DisplayName: "WS"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: agent.ID, Label: "ws-test"})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"session_id": sess.ID, "message": "hello ws"})
	if err := c.WriteJSON(map[string]any{"op": "chat", "payload": json.RawMessage(payload)}); err != nil {
		t.Fatal(err)
	}
	var chat wsFrame
	if err := c.ReadJSON(&chat); err != nil {
		t.Fatal(err)
	}
	if chat.Op != "chat" {
		t.Fatalf("chat op %s %s", chat.Op, string(chat.Payload))
	}
	var out struct {
		Reply     string `json:"reply"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(chat.Payload, &out); err != nil {
		t.Fatal(err)
	}
	if out.Reply != "echo: hello ws" || out.SessionID != sess.ID {
		t.Fatalf("chat %+v", out)
	}
	msgs, _ := st.ListMessages(sess.ID)
	if len(msgs) != 2 {
		t.Fatalf("messages %v", msgs)
	}
}

func TestWS_NotEchoOnlyPlainText(t *testing.T) {
	mux := http.NewServeMux()
	RegisterWS(mux, store.New(), llm.Echo{})
	u, closer := wsURL(t, mux)
	t.Cleanup(closer)
	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := c.WriteMessage(websocket.TextMessage, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	var frame wsFrame
	if err := c.ReadJSON(&frame); err != nil {
		t.Fatal(err)
	}
	if frame.Op != "error" {
		t.Fatalf("want error for non-JSON, got %+v", frame)
	}
	if strings.Contains(string(frame.Payload), "echo:") {
		t.Fatal("must not echo-prefix raw text")
	}
}

func TestWS_OriginAllowlist(t *testing.T) {
	t.Setenv("GOSO_WS_ORIGINS", "https://allowed.example")
	mux := http.NewServeMux()
	RegisterWS(mux, store.New(), llm.Echo{})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	_, resp, err := websocket.DefaultDialer.Dial(u, http.Header{"Origin": []string{"https://evil.example"}})
	if err == nil {
		t.Fatal("evil origin should fail")
	}
	if resp != nil && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d", resp.StatusCode)
	}

	c, _, err := websocket.DefaultDialer.Dial(u, http.Header{"Origin": []string{"https://allowed.example"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
}

func TestWS_ReadLimit(t *testing.T) {
	mux := http.NewServeMux()
	RegisterWS(mux, store.New(), llm.Echo{})
	u, closer := wsURL(t, mux)
	t.Cleanup(closer)
	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	big := strings.Repeat("x", 512*1024+32)
	if err := c.WriteMessage(websocket.TextMessage, []byte(`{"op":"ping","payload":"`+big+`"}`)); err != nil {
		t.Fatal(err)
	}
	_, _, err = c.ReadMessage()
	if err == nil {
		t.Fatal("expected read limit error")
	}
}
