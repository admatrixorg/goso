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
	p, err := s.CreateLLMProvider(LLMProvider{Name: "acme", Type: "openai-compat", BaseURL: "http://127.0.0.1:9", Model: "m1", Enabled: true})
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
	if !p.Enabled {
		t.Fatal("create defaults enabled")
	}
	upd, err := s.UpdateLLMProvider(LLMProvider{Name: "acme", Type: "anthropic", BaseURL: "http://127.0.0.1:10", Model: "m2", Enabled: false})
	if err != nil || upd.Type != "anthropic" || upd.Model != "m2" || upd.Enabled {
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
	if _, err := s1.CreateLLMProvider(LLMProvider{Name: "box", Type: "openai-compat", BaseURL: "http://example", Model: "x", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_ = s1.Close()
	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.GetLLMProvider("box")
	if err != nil || got.Type != "openai-compat" || got.Model != "x" || !got.Enabled {
		t.Fatalf("persist %v %+v", err, got)
	}
	off, err := s2.UpdateLLMProvider(LLMProvider{Name: "box", Type: got.Type, BaseURL: got.BaseURL, Model: got.Model, Enabled: false})
	if err != nil || off.Enabled {
		t.Fatalf("disable %v %+v", err, off)
	}
	_ = s2.Close()
	s3, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s3.Close()
	got, err = s3.GetLLMProvider("box")
	if err != nil || got.Enabled {
		t.Fatalf("persist disabled %v %+v", err, got)
	}
	if _, err := s3.CreateLLMProvider(LLMProvider{Name: "box", Type: "echo"}); err != ErrExists {
		t.Fatalf("dup sqlite %v", err)
	}
}
