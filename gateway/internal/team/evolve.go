// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package team

import (
	"errors"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	RuleHighToolError = "high-tool-error"
	RuleUnusedTools   = "unused-tools"

	// Prefixes appended on apply. Must not name display_name, agent_key, or identity.
	PrefixHighError   = "Validate tool arguments before calling. Prefer fewer retries on failure."
	PrefixUnusedTools = "Prefer using advertised tools when they match the user request."
)

// Suggestion is one deterministic self-evolution hint.
type Suggestion struct {
	ID     string `json:"id"`
	Rule   string `json:"rule"`
	Text   string `json:"text"`
	Status string `json:"status"`
}

// Suggestions builds deterministic hints. Never proposes renaming or identity edits.
func Suggestions(st store.StoreIface, agentID string) []Suggestion {
	if st == nil || agentID == "" {
		return []Suggestion{}
	}
	m := st.GetAgentMetrics(agentID)
	out := []Suggestion{}
	if m.ChatRuns >= 3 && m.ToolErrors*2 >= m.ChatRuns {
		out = append(out, Suggestion{
			ID:     RuleHighToolError,
			Rule:   RuleHighToolError,
			Text:   "Tool failure rate is high. " + PrefixHighError,
			Status: statusOf(st, agentID, RuleHighToolError),
		})
	}
	unused := unusedAdvertised(m)
	if m.ChatRuns >= 1 && len(unused) > 0 {
		out = append(out, Suggestion{
			ID:     RuleUnusedTools,
			Rule:   RuleUnusedTools,
			Text:   "Some advertised tools were never called. " + PrefixUnusedTools,
			Status: statusOf(st, agentID, RuleUnusedTools),
		})
	}
	if out == nil {
		out = []Suggestion{}
	}
	return out
}

func unusedAdvertised(m store.AgentMetrics) []string {
	var unused []string
	for _, name := range m.Advertised {
		if m.ToolUses[name] == 0 {
			unused = append(unused, name)
		}
	}
	return unused
}

func statusOf(st store.StoreIface, agentID, sid string) string {
	if st.EvolutionApplied(agentID, sid) {
		return "applied"
	}
	return "pending"
}

// ForbiddenWrite reports whether text treats protected fields as write targets.
func ForbiddenWrite(s string) bool {
	low := strings.ToLower(s)
	for _, tok := range []string{"display_name", "agent_key", "identity"} {
		if strings.Contains(low, tok) {
			return true
		}
	}
	return false
}

// Apply appends a prompt prefix. Rejects rename / identity edits.
func Apply(st store.StoreIface, agentID, sid string) (*store.Agent, error) {
	if st == nil {
		return nil, errors.New("store required")
	}
	sid = strings.TrimSpace(sid)
	if ForbiddenWrite(sid) {
		return nil, errors.New("apply rejected: cannot change protected fields")
	}
	a, err := st.GetAgent(agentID)
	if err != nil {
		return nil, err
	}
	var prefix string
	switch sid {
	case RuleHighToolError:
		prefix = PrefixHighError
	case RuleUnusedTools:
		prefix = PrefixUnusedTools
	default:
		return nil, errors.New("unknown suggestion")
	}
	if ForbiddenWrite(prefix) {
		return nil, errors.New("apply rejected: cannot change protected fields")
	}
	next := a.Instructions
	if next != "" && !strings.Contains(next, prefix) {
		next = strings.TrimSpace(next + "\n" + prefix)
	} else if next == "" {
		next = prefix
	}
	updated, err := st.UpdateAgent(store.Agent{
		ID:                a.ID,
		Instructions:      next,
		OrchestrationMode: a.OrchestrationMode,
		Model:             a.Model,
	})
	if err != nil {
		return nil, err
	}
	_ = st.MarkEvolutionApplied(agentID, sid)
	return updated, nil
}
