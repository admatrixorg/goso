// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package team

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

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

// TickResult is one auto-adapt cycle outcome.
type TickResult struct {
	Action       string                    `json:"action"`
	Reason       string                    `json:"reason,omitempty"`
	SuggestionID string                    `json:"suggestion_id,omitempty"`
	Agent        *store.Agent              `json:"agent,omitempty"`
	Guardrails   store.EvolutionGuardrails `json:"guardrails"`
}

// AutoEnabled reports GOSO_EVOLUTION_AUTO=1/true/yes/on. Default off so demo is unchanged.
func AutoEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_EVOLUTION_AUTO"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func errorRate(runs, errs int) float64 {
	if runs <= 0 {
		return 0
	}
	return float64(errs) / float64(runs)
}

func qualityDropped(g store.EvolutionGuardrails, m store.AgentMetrics) bool {
	if g.BaselineChatRuns <= 0 {
		return false
	}
	return errorRate(m.ChatRuns, m.ToolErrors) > errorRate(g.BaselineChatRuns, g.BaselineToolErrors)
}

func suggestionLocked(s Suggestion, g store.EvolutionGuardrails) bool {
	if ForbiddenWrite(s.ID) || ForbiddenWrite(s.Rule) || ForbiddenWrite(s.Text) {
		return true
	}
	for _, k := range g.Locked {
		if strings.EqualFold(strings.TrimSpace(k), s.ID) || strings.EqualFold(strings.TrimSpace(k), s.Rule) {
			return true
		}
	}
	return false
}

func firstPendingUnlocked(st store.StoreIface, agentID string, g store.EvolutionGuardrails) (Suggestion, bool) {
	for _, s := range Suggestions(st, agentID) {
		if !strings.EqualFold(s.Status, "pending") {
			continue
		}
		if suggestionLocked(s, g) {
			continue
		}
		return s, true
	}
	return Suggestion{}, false
}

func rollbackInstructions(st store.StoreIface, agentID string, g store.EvolutionGuardrails) (*store.Agent, store.EvolutionGuardrails, error) {
	a, err := st.GetAgent(agentID)
	if err != nil {
		return nil, g, err
	}
	updated, err := st.UpdateAgent(store.Agent{
		ID:                a.ID,
		Instructions:      g.SnapshotInstructions,
		OrchestrationMode: a.OrchestrationMode,
		Model:             a.Model,
		LLMProvider:       a.LLMProvider,
	})
	if err != nil {
		return nil, g, err
	}
	g.SnapshotInstructions = ""
	g.AppliedSuggestionID = ""
	g.BaselineChatRuns = 0
	g.BaselineToolErrors = 0
	if err := st.PutEvolutionGuardrails(agentID, g); err != nil {
		return updated, g, err
	}
	return updated, g, nil
}

// Tick applies or rolls back one auto-adapt cycle. auto_adapt default false is a no-op.
func Tick(st store.StoreIface, agentID string) (*TickResult, error) {
	if st == nil {
		return nil, errors.New("store required")
	}
	a, err := st.GetAgent(agentID)
	if err != nil {
		return nil, err
	}
	g := store.NormalizeGuardrails(st.GetEvolutionGuardrails(agentID))
	out := &TickResult{Guardrails: store.PublicGuardrails(g), Agent: a}
	if !g.AutoAdapt {
		out.Action = "noop"
		out.Reason = "auto_adapt_off"
		return out, nil
	}
	m := st.GetAgentMetrics(agentID)
	if g.AppliedSuggestionID != "" && g.SnapshotInstructions != "" {
		if qualityDropped(g, m) {
			updated, next, err := rollbackInstructions(st, agentID, g)
			if err != nil {
				return nil, err
			}
			return &TickResult{
				Action:       "rolled_back",
				Reason:       "error_rate",
				SuggestionID: g.AppliedSuggestionID,
				Agent:        updated,
				Guardrails:   store.PublicGuardrails(next),
			}, nil
		}
		out.Action = "noop"
		out.Reason = "watching"
		out.SuggestionID = g.AppliedSuggestionID
		return out, nil
	}
	if m.ChatRuns < g.MinRuns {
		out.Action = "noop"
		out.Reason = "min_runs"
		return out, nil
	}
	sug, ok := firstPendingUnlocked(st, agentID, g)
	if !ok {
		out.Action = "noop"
		out.Reason = "no_pending"
		return out, nil
	}
	prev := a.Instructions
	updated, err := Apply(st, agentID, sug.ID)
	if err != nil {
		return nil, err
	}
	g.SnapshotInstructions = prev
	g.AppliedSuggestionID = sug.ID
	g.BaselineChatRuns = m.ChatRuns
	g.BaselineToolErrors = m.ToolErrors
	if err := st.PutEvolutionGuardrails(agentID, g); err != nil {
		return nil, err
	}
	return &TickResult{
		Action:       "applied",
		SuggestionID: sug.ID,
		Agent:        updated,
		Guardrails:   store.PublicGuardrails(g),
	}, nil
}

// TickAll runs Tick for every agent. Used by the optional in-process ticker.
func TickAll(st store.StoreIface) {
	if st == nil {
		return
	}
	for _, a := range st.ListAgents() {
		if a == nil || strings.TrimSpace(a.ID) == "" {
			continue
		}
		_, _ = Tick(st, a.ID)
	}
}

// Loop runs TickAll on a 1-minute ticker until ctx is done. Does not spawn OS cron.
func Loop(ctx context.Context, st store.StoreIface) {
	if ctx == nil {
		ctx = context.Background()
	}
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			TickAll(st)
		}
	}
}
