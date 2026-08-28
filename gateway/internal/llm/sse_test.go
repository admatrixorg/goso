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
