// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"strings"
	"testing"
)

func TestStubs_FailClosed(t *testing.T) {
	cases := []Provider{ClaudeCLI{}, Codex{}, ACP{}}
	want := []string{"claude-cli", "codex", "acp"}
	for i, p := range cases {
		if p.Name() != want[i] {
			t.Fatalf("name %s", p.Name())
		}
		_, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}})
		if err == nil || !strings.Contains(err.Error(), "not_configured") {
			t.Fatalf("%s err %v", p.Name(), err)
		}
	}
}

func TestStubs_NotInEmptyRegistry(t *testing.T) {
	clearProviderEnv(t)
	r := NewRegistry()
	for _, name := range []string{"claude-cli", "codex", "acp"} {
		if r.Get(name).Name() != "echo" {
			t.Fatalf("%s should be absent", name)
		}
	}
}
