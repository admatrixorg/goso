// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func goclawDial(t *testing.T, st store.StoreIface) *websocket.Conn {
	t.Helper()
	mux := http.NewServeMux()
	RegisterWS(mux, st, llm.Echo{})
	u, closer := wsURL(t, mux)
	t.Cleanup(closer)
	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := c.WriteJSON(map[string]any{"type": "req", "id": "c1", "method": "connect", "params": map[string]any{"token": ""}}); err != nil {
		t.Fatal(err)
	}
	var res goclawRes
	if err := c.ReadJSON(&res); err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("connect %+v", res)
	}
	return c
}

func goclawRPC(t *testing.T, c *websocket.Conn, id, method string, params any) (goclawRes, []map[string]any) {
	t.Helper()
	if err := c.WriteJSON(map[string]any{"type": "req", "id": id, "method": method, "params": params}); err != nil {
		t.Fatal(err)
	}
	events := []map[string]any{}
	for {
		var raw map[string]any
		if err := c.ReadJSON(&raw); err != nil {
			t.Fatal(err)
		}
		switch raw["type"] {
		case "event":
			events = append(events, raw)
		case "res":
			b, _ := json.Marshal(raw)
			var res goclawRes
			_ = json.Unmarshal(b, &res)
			return res, events
		default:
			t.Fatalf("unexpected frame %+v", raw)
		}
	}
}

func TestGoClawWS_ChatSendHistory(t *testing.T) {
	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "desk", DisplayName: "Desk", Model: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	_ = a
	c := goclawDial(t, st)
	sessionKey := "agent:desk:ws:direct:conv1"
	res, events := goclawRPC(t, c, "s1", "chat.send", map[string]any{
		"agentId": "desk", "sessionKey": sessionKey, "message": "hello", "stream": true,
	})
	if !res.OK {
		t.Fatalf("chat.send %+v", res)
	}
	payload, _ := res.Payload.(map[string]any)
	if payload["content"] != "echo: hello" {
		t.Fatalf("content %+v", res.Payload)
	}
	got := map[string]bool{}
	for _, ev := range events {
		if ev["event"] != "agent" {
			continue
		}
		pl, _ := ev["payload"].(map[string]any)
		got[fmtString(pl["type"])] = true
	}
	if !got["run.started"] || !got["chunk"] || !got["run.completed"] {
		t.Fatalf("events %v", got)
	}
	hist, _ := goclawRPC(t, c, "h1", "chat.history", map[string]any{"agentId": "desk", "sessionKey": sessionKey})
	if !hist.OK {
		t.Fatalf("history %+v", hist)
	}
	hp, _ := hist.Payload.(map[string]any)
	msgs, _ := hp["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages %d %+v", len(msgs), hp)
	}
	list, _ := goclawRPC(t, c, "l1", "sessions.list", map[string]any{"agentId": "desk", "channel": "ws"})
	if !list.OK {
		t.Fatalf("sessions.list %+v", list)
	}
	lp, _ := list.Payload.(map[string]any)
	sessions, _ := lp["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("sessions %d", len(sessions))
	}
}

func fmtString(v any) string {
	s, _ := v.(string)
	return s
}
