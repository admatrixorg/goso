// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseSSE_OpenAICompat(t *testing.T) {
	body := "event: message\ndata: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n" +
		"data: {\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\n" +
		"data: [DONE]\n\n"
	text, u, err := ReadOpenAIStream(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if text != "Hello" {
		t.Fatalf("text %q", text)
	}
	if u.PromptTokens != 3 || u.CompletionTokens != 2 {
		t.Fatalf("usage %+v", u)
	}
}

func TestOpenAI_StreamHttptest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["stream"] != true {
			t.Errorf("stream %v", req["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi sse\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()
	p := &OpenAI{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client(), Stream: true}
	reply, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}})
	if err != nil || reply != "hi sse" {
		t.Fatalf("stream %v %q", err, reply)
	}
}

func TestOpenAI_ChatStreamCallbacksBeforeSecondChunk(t *testing.T) {
	first := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n")
		fl.Flush()
		select {
		case <-first:
		case <-time.After(5 * time.Second):
			t.Error("first delta was not observed before second chunk")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
	defer srv.Close()
	p := &OpenAI{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client()}
	var got []string
	text, err := p.ChatStream(t.Context(), []Message{{Role: "user", Content: "hi"}}, func(d string) {
		got = append(got, d)
		if len(got) == 1 {
			close(first)
		}
	})
	if err != nil || text != "Hello" {
		t.Fatalf("stream %v %q", err, text)
	}
	if len(got) != 2 || got[0] != "Hel" || got[1] != "lo" {
		t.Fatalf("deltas %v", got)
	}
}

func TestAnthropic_ChatStreamContentBlockDelta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["stream"] != true {
			t.Errorf("stream %v", req["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hel\"}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"lo\"}}\n\n")
		fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":2}}\n\n")
	}))
	defer srv.Close()
	p := &Anthropic{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client()}
	var got []string
	text, err := p.ChatStream(t.Context(), []Message{{Role: "user", Content: "hi"}}, func(d string) {
		got = append(got, d)
	})
	if err != nil || text != "Hello" {
		t.Fatalf("anthropic stream %v %q", err, text)
	}
	if len(got) != 2 || got[0] != "Hel" || got[1] != "lo" {
		t.Fatalf("deltas %v", got)
	}
}

func TestOpenAI_ChatStreamJSONBodyOneHonestChunk(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"content":"pong"}}]}` + "\n\ndata: [DONE]\n")
	for _, ct := range []string{"application/json", "text/event-stream"} {
		ct := ct
		t.Run(ct, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req map[string]any
				_ = json.NewDecoder(r.Body).Decode(&req)
				if req["stream"] != true {
					t.Errorf("stream %v", req["stream"])
				}
				w.Header().Set("Content-Type", ct)
				_, _ = w.Write(body)
			}))
			defer srv.Close()
			p := &OpenAI{
				APIKey: "", BaseURL: srv.URL + "/v1", Client: srv.Client(),
				Label: "router9", AllowEmptyKey: true,
			}
			var got []string
			reply, err := p.ChatStream(t.Context(), []Message{{Role: "user", Content: "hi"}}, func(d string) {
				got = append(got, d)
			})
			if err != nil || reply != "pong" {
				t.Fatalf("stream %v %q", err, reply)
			}
			if len(got) != 1 || got[0] != "pong" {
				t.Fatalf("want one honest chunk, got %v", got)
			}
		})
	}
}

func TestOpenAI_ChatStreamToolsAccumulatesCallsAndDeltas(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.Header().Set("Content-Type", "text/event-stream")
		if n == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\\\"q\\\":\"}}]}}]}\n\n")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"hi\\\"}\"}}]}}]}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	p := &OpenAI{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client()}
	first, err := p.ChatStreamTools(t.Context(), []Message{{Role: "user", Content: "hi"}}, []ToolSpec{{Name: "lookup"}}, nil)
	if err != nil || len(first.ToolCalls) != 1 || first.ToolCalls[0].Name != "lookup" {
		t.Fatalf("tool_use %v %+v", err, first)
	}
	if q, _ := first.ToolCalls[0].Arguments["q"].(string); q != "hi" {
		t.Fatalf("args %+v", first.ToolCalls[0].Arguments)
	}
	var got []string
	second, err := p.ChatStreamTools(t.Context(), []Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: first.ToolCalls},
		{Role: "tool", ToolCallID: "call_1", Content: "ok"},
	}, []ToolSpec{{Name: "lookup"}}, func(d string) { got = append(got, d) })
	if err != nil || second.Text != "done" {
		t.Fatalf("text %v %+v", err, second)
	}
	if len(got) != 1 || got[0] != "done" {
		t.Fatalf("deltas %v", got)
	}
}
