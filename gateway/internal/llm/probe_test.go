// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelsURL(t *testing.T) {
	if got := modelsURL("http://127.0.0.1:9/v1"); got != "http://127.0.0.1:9/v1/models" {
		t.Fatalf("v1 suffix %s", got)
	}
	if got := modelsURL("https://api.groq.com/openai"); got != "https://api.groq.com/openai/v1/models" {
		t.Fatalf("groq %s", got)
	}
}

func TestProbe_ModelsOKAnd401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer k-test" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "gpt-test"}, {"id": "gpt-other"}},
		})
	}))
	defer srv.Close()

	okP := &OpenAI{APIKey: "k-test", BaseURL: srv.URL, Label: "acme", Client: srv.Client()}
	got := Probe(context.Background(), okP, "models")
	if !got.OK || len(got.Models) != 2 || got.Models[0] != "gpt-test" {
		t.Fatalf("ok %+v", got)
	}

	bad := &OpenAI{APIKey: "nope", BaseURL: srv.URL, Label: "acme", Client: srv.Client()}
	fail := Probe(context.Background(), bad, "models")
	if fail.OK || fail.Error == "" {
		t.Fatalf("want failed probe, got %+v", fail)
	}
}

func TestProbe_EchoChat(t *testing.T) {
	got := Probe(context.Background(), Echo{}, "chat")
	if !got.OK || got.Reply != "echo: ping" {
		t.Fatalf("echo chat %+v", got)
	}
}

func TestBuild_UnknownType(t *testing.T) {
	if _, err := Build("x", "nope", "", "", ""); err == nil {
		t.Fatal("want error")
	}
}

func TestProbe_ModelsErrorRedactsSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad","api_key":"k-secret","authorization":"Bearer k-secret"}`))
	}))
	defer srv.Close()
	fail := Probe(context.Background(), &OpenAI{APIKey: "k-secret", BaseURL: srv.URL, Label: "acme", Client: srv.Client()}, "models")
	if fail.OK || fail.Error == "" {
		t.Fatalf("want failed probe, got %+v", fail)
	}
	if strings.Contains(fail.Error, "k-secret") {
		t.Fatalf("leaked key: %s", fail.Error)
	}
	if strings.Contains(strings.ToLower(fail.Error), "bearer k-secret") {
		t.Fatalf("leaked bearer: %s", fail.Error)
	}
}

func TestRedactProbeError(t *testing.T) {
	got := redactProbeError(`401: {"api_key":"abc","authorization":"Bearer xyz"} Authorization: Bearer xyz sk-live-ABCDEF`, "abc")
	if strings.Contains(got, "abc") || strings.Contains(got, "xyz") || strings.Contains(got, "sk-live-ABCDEF") {
		t.Fatalf("leaked: %s", got)
	}
}

func TestAPIKeySecretName(t *testing.T) {
	if got := APIKeySecretName("acme"); got != "provider:acme:api_key" {
		t.Fatalf("%s", got)
	}
}

func TestProbeAndChat_SSRFBlocksLoopback(t *testing.T) {
	t.Setenv("GOSO_SSRF", "1")
	p := &OpenAI{APIKey: "k", BaseURL: "http://127.0.0.1:9/v1", Label: "local"}
	got := Probe(context.Background(), p, "models")
	if got.OK || got.Error == "" {
		t.Fatalf("probe should fail SSRF, got %+v", got)
	}
	_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("chat should fail SSRF")
	}
}
