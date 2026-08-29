// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func eventsServer(t *testing.T) (*store.Store, *eventstore.Store, http.Handler) {
	t.Helper()
	st := store.New()
	ev := eventstore.New(64)
	h := NewRouter(Options{Store: st, Version: "t", Events: ev})
	return st, ev, h
}

func TestEvents_ListRedactsPayloadsAndV1(t *testing.T) {
	_, ev, h := eventsServer(t)
	ev.Append(eventstore.Event{
		Type:    eventstore.TypeMessage,
		Kind:    eventstore.KindSuccess,
		Actor:   "ag1",
		Summary: `{"action":"create","body":"secret-chat-body","token":"super-secret","from_agent_id":"ag1"}`,
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/events", nil))
	if w.Code != 200 {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "secret-chat-body") || strings.Contains(body, "super-secret") {
		t.Fatalf("leaked %s", body)
	}
	if strings.Contains(body, `"body"`) || strings.Contains(strings.ToLower(body), `"token"`) {
		t.Fatalf("payload keys %s", body)
	}
	if !strings.Contains(body, "ag1") {
		t.Fatalf("kept actor %s", body)
	}
	assertSameGET(t, h, "/api/events", "/v1/events")
}

func TestEvents_AgentTeamTaskMessageLink(t *testing.T) {
	st, _, h := eventsServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewBufferString(`{"agent_key":"a1","display_name":"A1"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("agent %d %s", w.Code, w.Body.String())
	}
	var agent map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &agent)
	aid := agent["id"].(string)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewBufferString(`{"agent_key":"a2","display_name":"A2"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("agent2 %d %s", w.Code, w.Body.String())
	}
	var agent2 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &agent2)
	aid2 := agent2["id"].(string)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/teams", bytes.NewBufferString(`{"name":"Ops","lead_agent_id":"`+aid+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("team %d %s", w.Code, w.Body.String())
	}
	var team map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &team)
	tid := team["id"].(string)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/teams/"+tid+"/tasks", bytes.NewBufferString(`{"title":"T1","status":"todo"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("task %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/teams/"+tid+"/messages", bytes.NewBufferString(`{"from_agent_id":"`+aid+`","body":"secret-team-body Bearer abcdefghijklmnop"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("msg %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agents/"+aid+"/links", bytes.NewBufferString(`{"to_agent_id":"`+aid2+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("link %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/events?type=message", nil))
	if w.Code != 200 {
		t.Fatalf("list msg %d %s", w.Code, w.Body.String())
	}
	got := w.Body.String()
	if strings.Contains(got, "secret-team-body") || strings.Contains(got, "Bearer abcdefghijklmnop") {
		t.Fatalf("message body leaked %s", got)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/events?type=agent", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"type":"agent"`) {
		t.Fatalf("agent events %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/events?type=team", nil))
	if !strings.Contains(w.Body.String(), `"type":"team"`) {
		t.Fatalf("team events %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/events?type=task", nil))
	if !strings.Contains(w.Body.String(), `"type":"task"`) {
		t.Fatalf("task events %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/events?type=agent_link", nil))
	if !strings.Contains(w.Body.String(), `"type":"agent_link"`) {
		t.Fatalf("link events %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/events?actor="+aid, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), aid) {
		t.Fatalf("actor %s", w.Body.String())
	}
	_ = st
}

func TestEvents_StreamLiveAndReconnect(t *testing.T) {
	_, ev, h := eventsServer(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	ready := make(chan struct{})
	got := make(chan string, 4)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 64*1024)
		var block []string
		flush := func() {
			joined := strings.Join(block, "\n")
			block = nil
			if strings.Contains(joined, "event: ready") {
				select {
				case <-ready:
				default:
					close(ready)
				}
			}
			if strings.Contains(joined, "event: ops") {
				got <- joined
			}
		}
		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				flush()
				continue
			}
			block = append(block, line)
		}
		if len(block) > 0 {
			flush()
		}
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("ready timeout")
	}

	ev.Append(eventstore.Event{
		Type: eventstore.TypeAgent, Kind: eventstore.KindSuccess, Actor: "operator", AgentID: "live-1",
		Summary: `{"action":"create","token":"super-secret","agent_id":"live-1"}`,
	})

	var payload string
	select {
	case payload = <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("ops timeout")
	}
	if strings.Contains(payload, "super-secret") {
		t.Fatalf("stream leaked %s", payload)
	}
	if !strings.Contains(payload, "live-1") {
		t.Fatalf("missing live event %s", payload)
	}

	// Last-Event-ID replay
	first := ev.Append(eventstore.Event{Type: eventstore.TypeTeam, Kind: eventstore.KindSuccess, TeamID: "tm-replay", Action: "create"})
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	req2, _ := http.NewRequestWithContext(ctx2, http.MethodGet, srv.URL+"/api/events/stream?type=team", nil)
	req2.Header.Set("Last-Event-ID", eventstore.SeqString(first.Seq-1))
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp2.Body, 8192))
	if !strings.Contains(string(b), "tm-replay") {
		t.Fatalf("replay %s", b)
	}
}

func TestEvents_ViewTokenGET(t *testing.T) {
	_, ev, inner := eventsServer(t)
	ev.Append(eventstore.Event{Type: eventstore.TypeConnector, Kind: eventstore.KindSuccess, Connector: "c1"})
	h := auth.RequireTokens("admin-109", "view-109", []string{"/healthz"})(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req.Header.Set("Authorization", "Bearer view-109")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("view GET %d %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/events", nil)
	req.Header.Set("Authorization", "Bearer view-109")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("view v1 %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/events", nil)
	req.Header.Set("Authorization", "Bearer view-109")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("view POST %d", w.Code)
	}
}
