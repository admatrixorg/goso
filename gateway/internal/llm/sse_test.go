// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
