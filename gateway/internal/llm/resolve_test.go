// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"errors"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestResolve_EmptyUsesFallbackAndClonesModel(t *testing.T) {
	t.Setenv("GOSO_LLM_PROVIDER", "router9")
	t.Setenv("GOSO_ROUTER9_BASE_URL", "http://127.0.0.1:20127/v1")
	orig := &OpenAI{Label: "echo-compat", Model: "orig", BaseURL: "http://127.0.0.1:9"}
	got, err := Resolve(store.New(), "", "agent-model", orig)
	if err != nil {
		t.Fatal(err)
	}
	o, ok := got.(*OpenAI)
	if !ok || o.Model != "agent-model" {
		t.Fatalf("got %#v", got)
	}
	if orig.Model != "orig" {
		t.Fatalf("mutated singleton %q", orig.Model)
	}
}

func TestResolve_EnvHasWinsOverSQLite(t *testing.T) {
	t.Setenv("GOSO_LLM_PROVIDER", "router9")
	t.Setenv("GOSO_ROUTER9_BASE_URL", "http://127.0.0.1:20127/v1")
	t.Setenv("GOSO_ROUTER9_MODEL", "ocg/deepseek-v4-flash")
	st := store.New()
	if _, err := st.CreateLLMProvider(store.LLMProvider{Name: "router9", Type: TypeOpenAICompat, BaseURL: "http://steal", Model: "hijack"}); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(st, "router9", "session-model", Echo{})
	if err != nil {
		t.Fatal(err)
	}
	o, ok := got.(*OpenAI)
	if !ok || o.Label != "router9" {
		t.Fatalf("want env router9, got %#v", got)
	}
	if o.Model != "session-model" {
		t.Fatalf("model %q", o.Model)
	}
	if o.BaseURL == "http://steal" {
		t.Fatal("sqlite stole env router9")
	}
}

func TestResolve_SQLiteNamedNotStolenByPreferred(t *testing.T) {
	t.Setenv("GOSO_LLM_PROVIDER", "router9")
	t.Setenv("GOSO_ROUTER9_BASE_URL", "http://127.0.0.1:20127/v1")
	st := store.New()
	if _, err := st.CreateLLMProvider(store.LLMProvider{Name: "p-a", Type: TypeRouter9, BaseURL: "http://127.0.0.1:9/v1", Model: "row-a"}); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(st, "p-a", "agent-a", Echo{})
	if err != nil {
		t.Fatal(err)
	}
	o, ok := got.(*OpenAI)
	if !ok {
		t.Fatalf("type %#v", got)
	}
	if o.Label != "p-a" || o.Model != "agent-a" || o.BaseURL != "http://127.0.0.1:9/v1" {
		t.Fatalf("sqlite row %#v", o)
	}
}

func TestResolve_NamedMiss(t *testing.T) {
	_, err := Resolve(store.New(), "nope", "", Echo{})
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("got %v", err)
	}
}
