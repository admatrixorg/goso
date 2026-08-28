// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"path/filepath"
	"testing"
)

func TestStore_LLMProviderCRUD(t *testing.T) {
	s := New()
	if len(s.ListLLMProviders()) != 0 {
		t.Fatal("empty")
	}
	p, err := s.CreateLLMProvider(LLMProvider{Name: "acme", Type: "openai-compat", BaseURL: "http://127.0.0.1:9", Model: "m1"})
	if err != nil || p.Name != "acme" || p.Type != "openai-compat" {
		t.Fatalf("create %v %+v", err, p)
	}
	if _, err := s.CreateLLMProvider(LLMProvider{Name: "acme", Type: "echo"}); err != ErrExists {
		t.Fatalf("dup %v", err)
	}
	got, err := s.GetLLMProvider("acme")
	if err != nil || got.Model != "m1" {
		t.Fatalf("get %v %+v", err, got)
	}
	upd, err := s.UpdateLLMProvider(LLMProvider{Name: "acme", Type: "anthropic", BaseURL: "http://127.0.0.1:10", Model: "m2"})
	if err != nil || upd.Type != "anthropic" || upd.Model != "m2" {
		t.Fatalf("update %v %+v", err, upd)
	}
	if _, err := s.UpdateLLMProvider(LLMProvider{Name: "missing"}); err != ErrNotFound {
		t.Fatalf("missing update %v", err)
	}
	if _, err := s.GetLLMProvider("missing"); err != ErrNotFound {
		t.Fatalf("missing get %v", err)
	}
}

func TestSQLiteStore_LLMProviderPersist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.db")
	s1, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.CreateLLMProvider(LLMProvider{Name: "box", Type: "openai-compat", BaseURL: "http://example", Model: "x"}); err != nil {
		t.Fatal(err)
	}
	_ = s1.Close()
	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.GetLLMProvider("box")
	if err != nil || got.Type != "openai-compat" || got.Model != "x" {
		t.Fatalf("persist %v %+v", err, got)
	}
	if _, err := s2.CreateLLMProvider(LLMProvider{Name: "box", Type: "echo"}); err != ErrExists {
		t.Fatalf("dup sqlite %v", err)
	}
}
