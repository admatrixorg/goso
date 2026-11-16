// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIImagePartsOnlyWhenPresent(t *testing.T) {
	plain := openaiMessages([]Message{{Role: "user", Content: "hi"}})
	if s, ok := plain[0]["content"].(string); !ok || s != "hi" {
		t.Fatalf("text-only content must stay a string: %#v", plain[0]["content"])
	}
	msgs := openaiMessages([]Message{{Role: "user", Content: "what is this", Images: []ImagePart{{MIME: "image/png", Data: []byte{1, 2, 3}}}}})
	parts, ok := msgs[0]["content"].([]map[string]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("parts %#v", msgs[0]["content"])
	}
	if parts[0]["type"] != "text" || parts[1]["type"] != "image_url" {
		t.Fatalf("order %#v", parts)
	}
	u := parts[1]["image_url"].(map[string]any)["url"].(string)
	if u != "data:image/png;base64,AQID" {
		t.Fatalf("data url %q", u)
	}
}

func TestMessageImagesNeverSerialized(t *testing.T) {
	b, err := json.Marshal(Message{Role: "user", Content: "x", Images: []ImagePart{{MIME: "image/png", Data: []byte("secret")}}})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"role":"user","content":"x"}` {
		t.Fatalf("json %s", b)
	}
}

func TestAnthropicImageBlock(t *testing.T) {
	t.Setenv("GOSO_SSRF", "0")
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"a cat"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	a := &Anthropic{APIKey: "test", BaseURL: srv.URL, Model: "m"}
	out, err := a.Chat(context.Background(), []Message{{Role: "user", Content: "describe", Images: []ImagePart{{MIME: "image/jpeg", Data: []byte{0xff}}}}})
	if err != nil || out != "a cat" {
		t.Fatalf("%q %v", out, err)
	}
	msgs := got["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].([]any)
	img := content[0].(map[string]any)
	src := img["source"].(map[string]any)
	if img["type"] != "image" || src["media_type"] != "image/jpeg" || src["data"] != "/w==" {
		t.Fatalf("image block %#v", img)
	}
	if content[1].(map[string]any)["text"] != "describe" {
		t.Fatalf("text block %#v", content[1])
	}
}

func TestSupportsImages(t *testing.T) {
	if SupportsImages(Echo{}) || !SupportsImages(&OpenAI{}) || !SupportsImages(&Anthropic{}) {
		t.Fatal("SupportsImages")
	}
}
