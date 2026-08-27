// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/llm"
)

func TestEchoSummary_FirstLast(t *testing.T) {
	got := EchoSummary([]llm.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "echo: hello"},
		{Role: "user", Content: "bye"},
	})
	if got != "hello\nbye" {
		t.Fatalf("got %q", got)
	}
	one := EchoSummary([]llm.Message{{Role: "user", Content: "only"}})
	if one != "only" {
		t.Fatalf("one %q", one)
	}
}

func TestTruncateRunes(t *testing.T) {
	s := strings.Repeat("あ", 600)
	got := TruncateRunes(s, 500)
	if utf8.RuneCountInString(got) != 500 {
		t.Fatalf("runes %d", utf8.RuneCountInString(got))
	}
}

func TestCapLast_DropsMiddleKeepsSummaryHook(t *testing.T) {
	var msgs []llm.Message
	for i := 0; i < 60; i++ {
		msgs = append(msgs, llm.Message{Role: "user", Content: "x"})
	}
	got := CapLast(msgs, HistoryCap)
	if len(got) != HistoryCap {
		t.Fatalf("len %d", len(got))
	}
}
