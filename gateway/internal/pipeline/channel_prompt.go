// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"strings"

	"github.com/mqglobal/goso/gateway/internal/guardrail"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// ChannelSystem is the one system message for a live channel reply (Zalo,
// Telegram, Slack, …): the agent's instructions, then this customer's memory
// from other sessions, then the bootstrap files — the same order and the same
// single-message rule as Runner.attachPrompt, since 9router/gcli drops every
// system message after the first. Recall is scoped to the session's agent and
// customer key (store.CustomerKeyForLabel) and fails closed: a session with no
// customer key recalls nothing. The channel sends its whole thread as history,
// so no same-session summary is added. The no-invention guardrail
// (guardrail.Reply) always closes the message, for every agent, so the result
// is never empty.
func ChannelSystem(st store.StoreIface, agent *store.Agent, sess *store.Session, userText string) string {
	var parts []string
	if agent != nil {
		parts = append(parts, strings.TrimSpace(agent.Instructions))
	}
	if sess != nil && agent != nil && IsChannelSession(sess) {
		note, _ := RecallFor(st, store.RecallQuery{
			TenantID:       sess.TenantID,
			AgentID:        sess.AgentID,
			ExcludeSession: sess.ID,
			Query:          userText,
			CustomerScoped: true,
			CustomerKey:    CustomerKey(sess),
		})
		parts = append(parts, strings.TrimSpace(note))
	}
	parts = append(parts, strings.TrimSpace(BootstrapText()))
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return guardrail.Append(strings.Join(kept, "\n\n"))
}
