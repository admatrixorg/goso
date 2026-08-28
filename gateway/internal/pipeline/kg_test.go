// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestExtractEntityNames(t *testing.T) {
	got := ExtractEntityNames("hello\nName: Acme Billing\nEntity: Zeta\nname:\nskip")
	if len(got) != 2 || got[0] != "Acme Billing" || got[1] != "Zeta" {
		t.Fatalf("got %#v", got)
	}
}

func TestDispatchMemoryTool_FailClosed(t *testing.T) {
	st := store.New()
	_, err := DispatchMemoryTool(st, store.DefaultTenant, llm.ToolCall{Name: ToolMemorySearch, Arguments: map[string]any{"query": "  "}})
	if err == nil {
		t.Fatal("empty query must fail")
	}
	_, err = DispatchMemoryTool(st, store.DefaultTenant, llm.ToolCall{Name: ToolMemoryExpand, Arguments: map[string]any{"id": ""}})
	if err == nil {
		t.Fatal("empty id must fail")
	}
}

func TestKGExtractEnabledDefaultOff(t *testing.T) {
	t.Setenv("GOSO_KG_EXTRACT", "")
	if KGExtractEnabled() {
		t.Fatal("default must be off")
	}
	t.Setenv("GOSO_KG_EXTRACT", "1")
	if !KGExtractEnabled() {
		t.Fatal("1 must enable")
	}
}
