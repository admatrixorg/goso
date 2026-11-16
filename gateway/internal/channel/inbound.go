// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/store"
)

type inboundDeps struct {
	Store  store.StoreIface
	LLM    llm.Provider
	Meter  *billing.Store
	Sender func(ctx context.Context, dest, text string) error
}

func writeOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func writeOKWarning(w http.ResponseWriter, warning string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "warning": warning})
}

// resolveInboundLLM picks the bound agent's provider/model. A Resolve miss
// keeps fallback so the webhook still returns 200 (LLM errors become reply text).
func resolveInboundLLM(st store.StoreIface, agent *store.Agent, fallback llm.Provider) llm.Provider {
	provider := fallback
	if provider == nil {
		provider = llm.Echo{}
	}
	if agent == nil {
		return provider
	}
	if p, err := llm.Resolve(st, agent.LLMProvider, agent.Model, provider); err == nil && p != nil {
		return p
	}
	return provider
}

// replyInbound answers one message of an older single-bot webhook. sender is
// the sender id when the webhook gives one for a direct chat (it selects the
// user context files), else "".
func replyInbound(w http.ResponseWriter, r *http.Request, d inboundDeps, agentKey, displayName, dest, sender, text string) {
	if dest == "" || text == "" {
		writeOK(w)
		return
	}
	agent := ensureNamedAgent(d.Store, agentKey, displayName)
	sight := Sighting{Channel: agentKey, Dest: dest, Kind: normalizeKind(dest, "")}
	if agent != nil {
		sight.AgentID = agent.ID
		sight.TenantID = agent.TenantID
	}
	ObserveDefault(sight)
	if BufferIfNeeded(nil, agent, agentKey, dest) {
		writeOK(w)
		return
	}
	sess := ensureLabeledSession(d.Store, agent, agentKey+":"+dest)
	if sess == nil {
		writeOKWarning(w, "session unavailable")
		return
	}

	ctx := r.Context()
	if sender != "" {
		ctx = withContextUser(ctx, agentKey, "direct", sender, dest)
	}
	reply := answerTurn(ctx, d.Store, agent, sess, d.LLM, d.Meter, text)

	if d.Sender != nil {
		if err := d.Sender(r.Context(), dest, reply); err != nil {
			writeOKWarning(w, err.Error())
			return
		}
	}
	writeOK(w)
}

// answerTurn records one inbound channel message, runs the agent and records
// the reply. With a TurnRunner wired (serve), the turn goes through the full
// agent runtime on the channel session: tools, connectors, context files
// (user key from ctx, see withContextUser), hooks, approvals, customer-scoped
// recall and the no-invention guardrail. Without one it falls back to one
// direct model call (answerTurnDirect). Errors become the reply text, as
// before. The user message is stored exactly once, also when the turn fails.
// An archived session or a disabled agent does not skip the store or the reply.
func answerTurn(ctx context.Context, st store.StoreIface, agent *store.Agent, sess *store.Session, fallback llm.Provider, meter *billing.Store, text string) string {
	reply, err := answerTurnErr(ctx, st, agent, sess, fallback, meter, text)
	if err != nil {
		return fmt.Sprintf("LLM error: %v", err)
	}
	return reply
}

// answerTurnErr is answerTurn for a channel that must not show a model error
// to its customer: on error nothing is stored as the reply and err is set.
func answerTurnErr(ctx context.Context, st store.StoreIface, agent *store.Agent, sess *store.Session, fallback llm.Provider, meter *billing.Store, text string) (string, error) {
	if run := currentTurnRunner(); run != nil && agent != nil && sess != nil {
		return answerTurnRuntime(ctx, run, st, agent, sess, meter, text)
	}
	return answerTurnDirect(ctx, st, agent, sess, fallback, meter, text)
}

// answerTurnRuntime runs the turn through the agent runtime. The runtime
// stores the user message; when it failed before doing so, it is stored here.
// Usage is metered on the agent that answered; a provider that reports no
// usage is estimated, as the HTTP chat path does.
func answerTurnRuntime(ctx context.Context, run TurnRunner, st store.StoreIface, agent *store.Agent, sess *store.Session, meter *billing.Store, text string) (string, error) {
	start := time.Now()
	res, err := run(ctx, sess.ID, text)
	if err != nil {
		if !userStoredSince(st, sess.ID, text, start) {
			_, _ = st.AddMessage(store.Message{SessionID: sess.ID, Role: "user", Content: text})
		}
		return "", err
	}
	usage := res.Usage
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		usage = llm.EstimateUsage([]llm.Message{{Role: "user", Content: text}}, res.Reply)
	}
	billed := res.AgentID
	if billed == "" {
		billed = agent.ID
	}
	trackUsage(meter, billed, res.Provider, usage)
	return customerReply(res), nil
}

// userStoredSince reports whether the session got this user message during
// the failed turn.
func userStoredSince(st store.StoreIface, sessionID, text string, since time.Time) bool {
	msgs, err := st.ListMessages(sessionID)
	if err != nil {
		return false
	}
	floor := since.Add(-2 * time.Second)
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m == nil || m.CreatedAt.Before(floor) {
			break
		}
		if m.Role == "user" && m.Content == text {
			return true
		}
	}
	return false
}

// answerTurnDirect is the fallback without the runtime: one system message
// (pipeline.ChannelSystem: agent instructions, this customer's memory,
// bootstrap, guardrail) followed by the whole thread, one model call, no
// tools. Memory never blocks the reply: a recall failure only drops the note.
func answerTurnDirect(ctx context.Context, st store.StoreIface, agent *store.Agent, sess *store.Session, fallback llm.Provider, meter *billing.Store, text string) (string, error) {
	_, _ = st.AddMessage(store.Message{SessionID: sess.ID, Role: "user", Content: text})

	provider := resolveInboundLLM(st, agent, fallback)
	history, _ := st.ListMessages(sess.ID)
	msgs := make([]llm.Message, 0, len(history)+1)
	if sys := pipeline.ChannelSystem(st, agent, sess, text); sys != "" {
		msgs = append(msgs, llm.Message{Role: "system", Content: sys})
	}
	for _, m := range history {
		msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
	}
	reply, usage, err := llm.ChatUsage(ctx, provider, msgs)
	if err != nil {
		return "", err
	}
	if agent != nil {
		trackUsage(meter, agent.ID, provider.Name(), usage)
	}
	_, _ = st.AddMessage(store.Message{SessionID: sess.ID, Role: "assistant", Content: reply})
	return reply, nil
}

// ensureNamedAgent finds or creates the default tenant's agent with this key.
// agent_key is unique per tenant, so the lookup never takes another tenant's
// agent that happens to share the key.
func ensureNamedAgent(st store.StoreIface, key, display string) *store.Agent {
	if a := store.FindAgentByKey(st, store.DefaultTenant, key); a != nil {
		return a
	}
	a, _ := st.CreateAgent(store.Agent{TenantID: store.DefaultTenant, AgentKey: key, DisplayName: display})
	return a
}

// ensureLabeledSession finds or creates the agent's session with label. The
// session takes the agent's tenant: the store refuses a session whose tenant
// differs from its agent's.
func ensureLabeledSession(st store.StoreIface, agent *store.Agent, label string) *store.Session {
	if st == nil || agent == nil {
		return nil
	}
	for _, s := range st.ListSessions() {
		if s.AgentID == agent.ID && s.Label == label {
			return s
		}
	}
	sess, _ := st.CreateSession(store.Session{AgentID: agent.ID, TenantID: agent.TenantID, Label: label})
	return sess
}
