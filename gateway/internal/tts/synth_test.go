// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package tts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func clearSynthEnv(t *testing.T) {
	t.Helper()
	clearTTSEnv(t)
	t.Setenv("GOSO_SSRF", "0")
}

var fakeMP3 = append([]byte("ID3"), make([]byte, 32)...)

func TestSynthesize_NotConfigured(t *testing.T) {
	clearSynthEnv(t)
	s := New()
	if s.Configured() {
		t.Fatal("configured without provider")
	}
	if _, err := s.Synthesize(context.Background(), "xin chào", SynthOptions{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err %v", err)
	}
	t.Setenv("GOSO_TTS_PROVIDER", "google")
	t.Setenv("GOSO_TTS_API_KEY", "test-key")
	if s.Configured() {
		t.Fatal("google has no synthesis path")
	}
	if _, err := s.Synthesize(context.Background(), "x", SynthOptions{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err %v", err)
	}
}

func TestSynthesize_OpenAICompat(t *testing.T) {
	clearSynthEnv(t)
	var got map[string]any
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(fakeMP3)
	}))
	defer srv.Close()
	t.Setenv("GOSO_TTS_PROVIDER", "openai")
	t.Setenv("GOSO_TTS_API_KEY", "test-key")
	t.Setenv("GOSO_TTS_ENDPOINT", srv.URL+"/v1")
	s := New()
	if !s.Configured() {
		t.Fatal("not configured")
	}
	a, err := s.Synthesize(context.Background(), "Chào Minh An", SynthOptions{Voice: "nova"})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/audio/speech" || auth != "Bearer test-key" {
		t.Fatalf("path %q auth set=%v", path, auth != "")
	}
	if got["input"] != "Chào Minh An" || got["voice"] != "nova" || got["model"] != "gpt-4o-mini-tts" {
		t.Fatalf("payload %+v", got)
	}
	if a.MIME != "audio/mpeg" || a.Ext != ".mp3" || len(a.Data) != len(fakeMP3) {
		t.Fatalf("audio %+v", a)
	}
}

func TestSynthesize_LimitsAndErrors(t *testing.T) {
	clearSynthEnv(t)
	status := http.StatusOK
	body := []byte("<html>not audio</html>")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	t.Setenv("GOSO_TTS_PROVIDER", "openai")
	t.Setenv("GOSO_TTS_API_KEY", "test-key")
	t.Setenv("GOSO_TTS_ENDPOINT", srv.URL)
	t.Setenv("GOSO_TTS_MAX_CHARS", "5")
	s := New()
	if _, err := s.Synthesize(context.Background(), "123456", SynthOptions{}); !errors.Is(err, ErrTooLong) {
		t.Fatalf("too long %v", err)
	}
	if _, err := s.Synthesize(context.Background(), "  ", SynthOptions{}); !errors.Is(err, ErrEmptyText) {
		t.Fatalf("empty %v", err)
	}
	if _, err := s.Synthesize(context.Background(), "hi", SynthOptions{}); err == nil || !strings.Contains(err.Error(), "mp3") {
		t.Fatalf("non-audio body must fail: %v", err)
	}
	status = http.StatusBadRequest
	body = []byte(`{"error":"bad","api_key":"sk-abcdefghijklmnop"}`)
	_, err := s.Synthesize(context.Background(), "hi", SynthOptions{})
	if err == nil || strings.Contains(err.Error(), "sk-abcdefghijklmnop") {
		t.Fatalf("error must be redacted: %v", err)
	}
}

func TestSynthesize_ElevenLabs(t *testing.T) {
	clearSynthEnv(t)
	var path, key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		key = r.Header.Get("xi-api-key")
		_, _ = w.Write([]byte{0xff, 0xfb, 0x90, 0x00})
	}))
	defer srv.Close()
	t.Setenv("GOSO_TTS_PROVIDER", "elevenlabs")
	t.Setenv("GOSO_TTS_API_KEY", "test-key")
	t.Setenv("GOSO_TTS_ENDPOINT", srv.URL)
	s := New()
	if _, err := s.Synthesize(context.Background(), "hi", SynthOptions{}); !errors.Is(err, ErrVoiceRequired) {
		t.Fatalf("voice %v", err)
	}
	if _, err := s.Synthesize(context.Background(), "hi", SynthOptions{Voice: "v1"}); err != nil {
		t.Fatal(err)
	}
	if path != "/v1/text-to-speech/v1" || key != "test-key" {
		t.Fatalf("path %q", path)
	}
}
