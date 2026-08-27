// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestChatSSE_EchoAcceptTwoFramesThenDONE(t *testing.T) {
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
	if len(deltas) != 2 {
		t.Fatalf("want 2 delta frames, got %d %v", len(deltas), deltas)
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
	if len(deltas) != 2 || !done {
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

func TestChatSSE_ScriptedTwoFramesThenDONE(t *testing.T) {
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
	if len(deltas) != 2 {
		t.Fatalf("want 2 frames, got %d %v", len(deltas), deltas)
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
	if len(deltas) != 2 || !done || strings.Join(deltas, "") != "echo: wrap" {
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
	if len(deltas) != 2 || !done || strings.Join(deltas, "") != "scripted ok" {
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
	if len(deltas) != 2 || !done || strings.Join(deltas, "") != "echo: v1" {
		t.Fatalf("v1 SSE %v done=%v %s", deltas, done, w.Body.String())
	}
}
