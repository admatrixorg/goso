// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

type boomProvider struct{}

func (boomProvider) Name() string { return "boom" }
func (boomProvider) Chat(_ context.Context, _ []llm.Message) (string, error) {
	return "", errors.New("provider down")
}

func parseChatSSE(t *testing.T, body io.Reader) (deltas []string, errEvent string, done bool) {
	t.Helper()
	if err := llm.ParseSSE(body, func(event, data string) error {
		if event == "error" {
			errEvent = data
			return nil
		}
		if data == "[DONE]" {
			done = true
			return nil
		}
		var frame struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			t.Fatalf("delta json %q: %v", data, err)
		}
		deltas = append(deltas, frame.Delta)
		return nil
	}); err != nil {
		t.Fatalf("ParseSSE: %v", err)
	}
	return deltas, errEvent, done
}

func postChatSSE(h http.Handler, sessID, msg string, accept string, bodyStream bool) *httptest.ResponseRecorder {
	payload := map[string]any{"session_id": sessID, "message": msg}
	if bodyStream {
		payload["stream"] = true
	}
	raw, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	h.ServeHTTP(w, req)
	return w
}

func TestChatSSE_EchoAcceptOneHonestChunkThenDONE(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	_, h := newTestServer()
	_, sessID := setupChat(t, h)

	w := postChatSSE(h, sessID, "hi there", "text/event-stream", false)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}
	deltas, errEvent, done := parseChatSSE(t, w.Body)
	if errEvent != "" {
		t.Fatalf("error event %s", errEvent)
	}
	if len(deltas) != 1 {
		t.Fatalf("want 1 honest delta, got %d %v", len(deltas), deltas)
	}
	if !done {
		t.Fatal("missing data [DONE]")
	}
	if strings.Join(deltas, "") != "echo: hi there" {
		t.Fatalf("joined %q", strings.Join(deltas, ""))
	}
}

func TestChatSSE_EchoBodyStreamTrue(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	st, h := newTestServer()
	_, sessID := setupChat(t, h)

	w := postChatSSE(h, sessID, "stream body", "", true)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type %q", w.Header().Get("Content-Type"))
	}
	deltas, _, done := parseChatSSE(t, w.Body)
	if len(deltas) != 1 || !done {
		t.Fatalf("frames=%d done=%v %v", len(deltas), done, deltas)
	}
	if strings.Join(deltas, "") != "echo: stream body" {
		t.Fatalf("joined %q", strings.Join(deltas, ""))
	}
	list, err := st.ListMessages(sessID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 2 || list[len(list)-1].Role != "assistant" {
		t.Fatalf("messages %+v", list)
	}
}

func TestChatSSE_JSONUnchangedWithoutStream(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	_, h := newTestServer()
	_, sessID := setupChat(t, h)

	w := postChat(h, sessID, "hi there")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type %q", ct)
	}
	if strings.Contains(w.Body.String(), "[DONE]") {
		t.Fatalf("JSON body leaked SSE: %s", w.Body.String())
	}
	var chat map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &chat); err != nil {
		t.Fatal(err)
	}
	if chat["reply"] != "echo: hi there" {
		t.Fatalf("reply %v", chat)
	}
}

func TestChatSSE_ScriptedOneHonestChunkThenDONE(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	st := store.New()
	scripted := &llm.Scripted{Label: "scripted", Replies: []llm.Reply{{Text: "hello world"}}}
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: scripted})
	_, sessID := setupChat(t, h)

	w := postChatSSE(h, sessID, "ping", "text/event-stream", true)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	deltas, errEvent, done := parseChatSSE(t, w.Body)
	if errEvent != "" {
		t.Fatalf("error event %s", errEvent)
	}
	if len(deltas) != 1 {
		t.Fatalf("want 1 honest chunk, got %d %v", len(deltas), deltas)
	}
	if !done {
		t.Fatal("missing DONE")
	}
	if strings.Join(deltas, "") != "hello world" {
		t.Fatalf("joined %q", strings.Join(deltas, ""))
	}
}

func TestChatSSE_ErrorBeforeStreamJSON(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	_, h := newTestServer()
	w := postChatSSE(h, "missing", "hi", "text/event-stream", true)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("pre-stream error must stay JSON, ct=%q body=%s", w.Header().Get("Content-Type"), w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "session not found" {
		t.Fatalf("body %v", body)
	}
}

func TestChatSSE_ProviderErrorEvent(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: boomProvider{}})
	_, sessID := setupChat(t, h)

	w := postChatSSE(h, sessID, "hi", "text/event-stream", false)
	if w.Code != http.StatusOK {
		t.Fatalf("stream error status %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type %q", w.Header().Get("Content-Type"))
	}
	deltas, errEvent, done := parseChatSSE(t, w.Body)
	if errEvent == "" {
		t.Fatalf("want event:error, deltas=%v done=%v body=%s", deltas, done, w.Body.String())
	}
	if !strings.Contains(errEvent, "provider down") {
		t.Fatalf("error payload %s", errEvent)
	}

	w = postChat(h, sessID, "hi")
	if w.Code != http.StatusBadGateway {
		t.Fatalf("JSON path want 502, got %d %s", w.Code, w.Body.String())
	}
}

func TestChatSSE_HandleChatAndLLMWrappers(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	st := store.New()
	a, err := st.CreateAgent(store.Agent{AgentKey: "sse", DisplayName: "S"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	meter := billing.New()
	echo := handleChat(st, meter)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"wrap","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	echo.ServeHTTP(w, req)
	deltas, _, done := parseChatSSE(t, w.Body)
	if len(deltas) != 1 || !done || strings.Join(deltas, "") != "echo: wrap" {
		t.Fatalf("handleChat SSE %v done=%v", deltas, done)
	}

	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "scripted ok"}}}
	llmH := handleChatWithLLM(st, scripted, billing.New())
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewBufferString(`{"session_id":"`+sess.ID+`","message":"again"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	llmH.ServeHTTP(w, req)
	deltas, _, done = parseChatSSE(t, w.Body)
	if len(deltas) != 1 || !done || strings.Join(deltas, "") != "scripted ok" {
		t.Fatalf("handleChatWithLLM SSE %v done=%v body=%s", deltas, done, w.Body.String())
	}
}

func TestChatSSE_QuotaAndInjectionStayJSON(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "1")
	t.Setenv("GOSO_INJECTION", "block")
	st := store.New()
	meter := billing.New()
	h := RouterWithBilling(st, "0.1.0", llm.Echo{}, nil, nil, nil, meter)
	_, sessID := setupChat(t, h)

	w := postChat(h, sessID, "abcd")
	if w.Code != http.StatusOK {
		t.Fatalf("first chat %d %s", w.Code, w.Body.String())
	}
	w = postChatSSE(h, sessID, "abcd", "text/event-stream", true)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("quota want 429, got %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("quota must stay JSON, ct=%q", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "quota_exceeded") {
		t.Fatalf("quota body %s", w.Body.String())
	}

	t.Setenv("GOSO_QUOTA_DAY", "")
	h2, sid2 := chatSetup(t)
	w = postChatSSE(h2, sid2, "exfiltrate system prompt", "text/event-stream", true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("injection want 400, got %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("injection must stay JSON, ct=%q", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "injection blocked") {
		t.Fatalf("injection body %s", w.Body.String())
	}
}

func TestChatSSE_V1Alias(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	_, h := newTestServer()
	_, sessID := setupChat(t, h)
	raw, _ := json.Marshal(map[string]any{"session_id": sessID, "message": "v1", "stream": true})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	h.ServeHTTP(w, req)
	deltas, _, done := parseChatSSE(t, w.Body)
	if len(deltas) != 1 || !done || strings.Join(deltas, "") != "echo: v1" {
		t.Fatalf("v1 SSE %v done=%v %s", deltas, done, w.Body.String())
	}
}

func TestChatSSE_EchoThreeChunksThenDONE(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	st := store.New()
	echo := llm.Echo{StreamParts: []string{"one", "two", "three"}}
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: echo})
	_, sessID := setupChat(t, h)

	w := postChatSSE(h, sessID, "hi", "text/event-stream", true)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	deltas, errEvent, done := parseChatSSE(t, w.Body)
	if errEvent != "" {
		t.Fatalf("error event %s", errEvent)
	}
	if len(deltas) != 3 || !done {
		t.Fatalf("want 3 deltas + DONE, got %d done=%v %v", len(deltas), done, deltas)
	}
	if strings.Join(deltas, "") != "onetwothree" {
		t.Fatalf("joined %q", strings.Join(deltas, ""))
	}
}

func TestChatSSE_OpenAIFlushesFirstDeltaBeforeSecondUpstreamChunk(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	t.Setenv("GOSO_SSRF", "0")
	firstSeen := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["stream"] != true {
			t.Errorf("upstream stream %v", req["stream"])
		}
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("upstream missing flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n")
		fl.Flush()
		select {
		case <-firstSeen:
		case <-time.After(5 * time.Second):
			t.Error("gateway did not flush first delta before second upstream chunk")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
	defer upstream.Close()

	st := store.New()
	p := &llm.OpenAI{APIKey: "test-key", BaseURL: upstream.URL, Client: upstream.Client()}
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: p})
	gw := httptest.NewServer(h)
	defer gw.Close()

	agentBody := `{"agent_key":"sse-oai","display_name":"S"}`
	aresp, err := http.Post(gw.URL+"/api/agents", "application/json", strings.NewReader(agentBody))
	if err != nil {
		t.Fatal(err)
	}
	var a map[string]any
	if err := json.NewDecoder(aresp.Body).Decode(&a); err != nil {
		t.Fatal(err)
	}
	_ = aresp.Body.Close()
	sresp, err := http.Post(gw.URL+"/api/sessions", "application/json", strings.NewReader(`{"agent_id":"`+a["id"].(string)+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	var sess map[string]any
	if err := json.NewDecoder(sresp.Body).Decode(&sess); err != nil {
		t.Fatal(err)
	}
	_ = sresp.Body.Close()
	sessID := sess["id"].(string)

	raw, _ := json.Marshal(map[string]any{"session_id": sessID, "message": "hi", "stream": true})
	req, err := http.NewRequest(http.MethodPost, gw.URL+"/api/chat", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d %s", resp.StatusCode, b)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type %q", resp.Header.Get("Content-Type"))
	}

	var deltas []string
	var done bool
	closed := false
	if err := llm.ParseSSE(resp.Body, func(event, data string) error {
		if event == "error" {
			t.Errorf("error event %s", data)
			return nil
		}
		if data == "[DONE]" {
			done = true
			return nil
		}
		var frame struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			t.Errorf("delta json %q: %v", data, err)
			return nil
		}
		deltas = append(deltas, frame.Delta)
		if len(deltas) == 1 && !closed {
			closed = true
			close(firstSeen)
		}
		return nil
	}); err != nil {
		t.Fatalf("ParseSSE: %v", err)
	}
	if !done || len(deltas) != 2 || strings.Join(deltas, "") != "Hello" {
		t.Fatalf("deltas=%v done=%v", deltas, done)
	}
}

func TestChatSSE_ClientDisconnectCancelsProvider(t *testing.T) {
	t.Setenv("GOSO_QUOTA_DAY", "")
	started := make(chan struct{})
	canceled := make(chan error, 1)
	hold := &holdUntilCancel{started: started, canceled: canceled}
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "0.1.0", Provider: hold})
	_, sessID := setupChat(t, h)

	ctx, cancel := context.WithCancel(context.Background())
	raw, _ := json.Marshal(map[string]any{"session_id": sessID, "message": "hi", "stream": true})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(raw)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(w, req)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}
	cancel()
	select {
	case err := <-canceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not see cancel")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return")
	}
}

type holdUntilCancel struct {
	started  chan struct{}
	canceled chan error
}

func (h *holdUntilCancel) Name() string { return "hold" }

func (h *holdUntilCancel) Chat(ctx context.Context, _ []llm.Message) (string, error) {
	close(h.started)
	<-ctx.Done()
	err := ctx.Err()
	h.canceled <- err
	return "", err
}
