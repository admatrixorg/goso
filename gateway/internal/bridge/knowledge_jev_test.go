// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/jev"
)

func TestKnowledgeBlock_JevOffMatchesPrompt(t *testing.T) {
	t.Setenv("GOSO_JEV_ENABLED", "")
	t.Setenv("GOSO_JEV_API_KEY", "")
	docs := []KnowledgeDoc{
		{ID: "a", FileName: "binh-duong.txt", Content: "Đại lý Bình Dương. Minh An (demo) · đại lý mẫu"},
		{ID: "b", FileName: "noi-chien.txt", Content: "Họp kế hoạch nồi chiên — Minh An (demo)"},
	}
	got := knowledgeBlock(context.Background(), "t", "agent", "Đại lý Bình Dương còn nợ bao nhiêu?", Agent{}, docs)
	want := knowledgePrompt(docs, defaultKnowledgeBytes)
	if got != want {
		t.Fatalf("off path changed\n%s\n%s", got, want)
	}
}

func TestKnowledgeBlock_DropsConfidentIrrelevant(t *testing.T) {
	jev.ResetForTest()
	t.Cleanup(jev.ResetForTest)
	const key = "bridge-jev-key"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			State     string `json:"state"`
			Questions map[string]struct {
				Instructions map[string]string `json:"instructions"`
			} `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(req.State, "nồi chiên") || strings.Contains(req.State, "Bình Dương") {
			t.Fatal("passage was placed in state")
		}
		answers := map[string]any{}
		for id, q := range req.Questions {
			noul := 0.95
			if strings.Contains(q.Instructions["passage"], "nồi chiên") {
				noul = 0.04
			}
			answers[id] = map[string]any{"type": "noul", "noul": noul}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-1.14.0", "answers": answers,
			"usage": map[string]int{"input_tokens": 80, "output_tokens": 4},
		})
	}))
	defer srv.Close()
	t.Setenv("GOSO_JEV_ENABLED", "1")
	t.Setenv("GOSO_JEV_API_KEY", key)
	t.Setenv("GOSO_JEV_URL", srv.URL)
	t.Setenv("GOSO_JEV_MODEL", "")
	docs := []KnowledgeDoc{
		{ID: "bd", FileName: "binh-duong.txt", Content: "Đại lý Bình Dương. Minh An (demo) · đại lý mẫu"},
		{ID: "nc", FileName: "noi-chien.txt", Content: "Họp kế hoạch nồi chiên — Minh An (demo)"},
	}
	got := knowledgeBlock(context.Background(), "tenant-demo", "agent-1", "Đại lý Bình Dương còn nợ bao nhiêu?", Agent{}, docs)
	if !strings.Contains(got, "Bình Dương") || strings.Contains(got, "nồi chiên") {
		t.Fatalf("prompt %s", got)
	}
}

func TestKnowledgeBlock_HTTPErrorKeepsAll(t *testing.T) {
	jev.ResetForTest()
	t.Cleanup(jev.ResetForTest)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	t.Setenv("GOSO_JEV_ENABLED", "1")
	t.Setenv("GOSO_JEV_API_KEY", "k")
	t.Setenv("GOSO_JEV_URL", srv.URL)
	docs := []KnowledgeDoc{
		{ID: "bd", FileName: "binh-duong.txt", Content: "Đại lý Bình Dương. Minh An (demo) · đại lý mẫu"},
		{ID: "nc", FileName: "noi-chien.txt", Content: "Họp kế hoạch nồi chiên — Minh An (demo)"},
	}
	got := knowledgeBlock(context.Background(), "t", "a", "Đại lý Bình Dương còn nợ bao nhiêu?", Agent{}, docs)
	want := knowledgePrompt(docs, defaultKnowledgeBytes)
	if got != want {
		t.Fatalf("fallback changed the prompt\n%s", got)
	}
}
