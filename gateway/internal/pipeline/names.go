// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"strings"

	"github.com/mqglobal/goso/gateway/internal/llm"
)

// AdvertiseName is the tool name sent to the LLM: connector__tool.
func AdvertiseName(connector, tool string) string {
	connector = strings.TrimSpace(connector)
	tool = strings.TrimSpace(tool)
	if connector == "" {
		return tool
	}
	return connector + "__" + tool
}

// SplitAdvertised parses connector__tool or connector.tool.
func SplitAdvertised(name string) (connector, tool string, ok bool) {
	name = strings.TrimSpace(name)
	if i := strings.Index(name, "__"); i > 0 {
		return name[:i], name[i+2:], true
	}
	if i := strings.Index(name, "."); i > 0 {
		return name[:i], name[i+1:], true
	}
	return "", name, false
}

// IsOrchestrationTool reports spawn / delegate / team_tasks (no connector).
func IsOrchestrationTool(name string) bool {
	n := strings.TrimSpace(name)
	if i := strings.Index(n, "__"); i > 0 {
		n = n[i+2:]
	} else if i := strings.Index(n, "."); i > 0 {
		n = n[i+1:]
	}
	switch n {
	case "spawn", "delegate", "team_tasks":
		return true
	default:
		return false
	}
}

const (
	ToolMemorySearch = "memory_search"
	ToolMemoryExpand = "memory_expand"
)

func advertisedBase(name string) string {
	n := strings.TrimSpace(name)
	if i := strings.Index(n, "__"); i > 0 {
		return n[i+2:]
	}
	if i := strings.Index(n, "."); i > 0 {
		return n[i+1:]
	}
	return n
}

// IsMemoryTool reports memory_search / memory_expand (no connector).
func IsMemoryTool(name string) bool {
	switch advertisedBase(name) {
	case ToolMemorySearch, ToolMemoryExpand:
		return true
	default:
		return false
	}
}

// IsSessionTool reports sessions_list / sessions_history (no connector).
func IsSessionTool(name string) bool {
	switch advertisedBase(name) {
	case ToolSessionsList, ToolSessionsHistory:
		return true
	default:
		return false
	}
}

// ResolveCall maps a ToolCall to connector + tool names for Runtime.CallTool.
func ResolveCall(call llm.ToolCall) (connector, tool string) {
	if c, t, ok := SplitAdvertised(call.Name); ok {
		if call.Connector != "" {
			return call.Connector, t
		}
		return c, t
	}
	if call.Connector != "" {
		return call.Connector, call.Name
	}
	return "", call.Name
}
