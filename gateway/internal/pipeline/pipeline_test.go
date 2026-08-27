// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestParseMode(t *testing.T) {
	m, err := ParseMode("")
	if err != nil || m != ModeFull {
		t.Fatalf("default %v %v", m, err)
	}
	m, err = ParseMode("task")
	if err != nil || m != ModeTask {
		t.Fatalf("task %v %v", m, err)
	}
	if _, err := ParseMode("weird"); err == nil {
		t.Fatal("expected unknown mode error")
	}
}

func TestSystemPrompt_Modes(t *testing.T) {
	if SystemPrompt(ModeNone, "A") != "" {
		t.Fatalf("none should be empty")
	}
	full := SystemPrompt(ModeFull, "A")
	if full == "" {
		t.Fatal("full empty")
	}
	task := SystemPrompt(ModeTask, "A")
	if task == "" || task == full {
		t.Fatalf("task %q", task)
	}
	min := SystemPrompt(ModeMinimal, "A")
	if min == "" || min == full {
		t.Fatalf("minimal %q", min)
	}
}

func TestIsOrchestrationTool(t *testing.T) {
	if !IsOrchestrationTool("delegate") || !IsOrchestrationTool("spawn") || !IsOrchestrationTool("team_tasks") {
		t.Fatal("expected orchestration names")
	}
	if IsOrchestrationTool("zalocrm__contact_search") {
		t.Fatal("connector tool is not orchestration")
	}
}

func TestAdvertiseAndResolve(t *testing.T) {
	name := AdvertiseName("zalocrm", "contact_search")
	if name != "zalocrm__contact_search" {
		t.Fatalf("name %s", name)
	}
	c, tool := ResolveCall(llm.ToolCall{Name: name})
	if c != "zalocrm" || tool != "contact_search" {
		t.Fatalf("resolve %s %s", c, tool)
	}
	c, tool = ResolveCall(llm.ToolCall{Name: "contact_search", Connector: "zalocrm"})
	if c != "zalocrm" || tool != "contact_search" {
		t.Fatalf("connector field %s %s", c, tool)
	}
	c, tool = ResolveCall(llm.ToolCall{Name: "zalocrm.contact_search"})
	if c != "zalocrm" || tool != "contact_search" {
		t.Fatalf("dot form %s %s", c, tool)
	}
}

func TestSanitize_DropsOrphanToolAndUnmatchedUse(t *testing.T) {
	msgs := []llm.Message{
		{Role: "tool", ToolCallID: "orphan", Content: "nope"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "ok", Name: "zalocrm__contact_search"}}},
		{Role: "tool", ToolCallID: "ok", Content: "found"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "missing", Name: "zalocrm__x"}}},
		{Role: "user", Content: "next"},
	}
	got := Sanitize(msgs)
	for _, m := range got {
		if m.Role == "tool" && m.ToolCallID == "orphan" {
			t.Fatal("orphan tool survived")
		}
		if m.Role == "assistant" {
			for _, c := range m.ToolCalls {
				if c.ID == "missing" {
					t.Fatal("unmatched tool_use survived")
				}
			}
		}
	}
	var tools int
	for _, m := range got {
		if m.Role == "tool" {
			tools++
		}
	}
	if tools != 1 {
		t.Fatalf("tool rows %d %#v", tools, got)
	}
}

func TestSanitizeAfterCap_DropsSplitToolPair(t *testing.T) {
	msgs := []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "x", Name: "zalocrm__contact_search"}}},
		{Role: "tool", ToolCallID: "x", Content: "found"},
	}
	for i := 0; i < 49; i++ {
		msgs = append(msgs, llm.Message{Role: "user", Content: "u"})
	}
	got := Sanitize(CapLast(msgs, HistoryCap))
	for _, m := range got {
		if m.Role == "tool" {
			t.Fatalf("cap-split tool row leaked: %#v", got)
		}
	}
}

func TestCapLast(t *testing.T) {
	var msgs []llm.Message
	for i := 0; i < 60; i++ {
		msgs = append(msgs, llm.Message{Role: "user", Content: "x"})
	}
	got := CapLast(msgs, HistoryCap)
	if len(got) != HistoryCap {
		t.Fatalf("len %d", len(got))
	}
}

func TestToLLM_RoundTripToolUse(t *testing.T) {
	content := EncodeAssistant("", []llm.ToolCall{{ID: "1", Name: "zalocrm__contact_search"}})
	tool := EncodeTool("1", `{"ok":true}`)
	msgs := ToLLM([]*store.Message{
		{Role: "assistant", Content: content},
		{Role: "tool", Content: tool},
	})
	if len(msgs) != 2 || len(msgs[0].ToolCalls) != 1 || msgs[1].ToolCallID != "1" {
		t.Fatalf("%#v", msgs)
	}
	if !strings.Contains(msgs[1].Content, security.UntrustedBegin) || !strings.Contains(msgs[1].Content, security.UntrustedEnd) {
		t.Fatalf("expected untrusted wrap %#v", msgs[1])
	}
}
