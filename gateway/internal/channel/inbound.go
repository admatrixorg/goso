// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

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

func replyInbound(w http.ResponseWriter, r *http.Request, d inboundDeps, agentKey, displayName, dest, text string) {
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
	sess := ensureLabeledSession(d.Store, agent.ID, agentKey+":"+dest)

	reply := answerTurn(r.Context(), d.Store, agent, sess, d.LLM, d.Meter, text)

	if d.Sender != nil {
		if err := d.Sender(r.Context(), dest, reply); err != nil {
			writeOKWarning(w, err.Error())
			return
		}
	}
	writeOK(w)
}

// answerTurn records one inbound channel message, asks the agent's LLM and
// records the reply. The prompt is one optional system message
// (pipeline.ChannelSystem: agent instructions, this customer's memory,
// bootstrap) followed by the whole thread. Memory never blocks the reply: a
// recall failure only drops the note. LLM errors become the reply text, as
// before.
func answerTurn(ctx context.Context, st store.StoreIface, agent *store.Agent, sess *store.Session, fallback llm.Provider, meter *billing.Store, text string) string {
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
		reply = fmt.Sprintf("LLM error: %v", err)
	} else if agent != nil {
		trackUsage(meter, agent.ID, provider.Name(), usage)
	}
	_, _ = st.AddMessage(store.Message{SessionID: sess.ID, Role: "assistant", Content: reply})
	return reply
}

func ensureNamedAgent(st store.StoreIface, key, display string) *store.Agent {
	for _, a := range st.ListAgents() {
		if a.AgentKey == key {
			return a
		}
	}
	a, _ := st.CreateAgent(store.Agent{AgentKey: key, DisplayName: display})
	return a
}

func ensureLabeledSession(st store.StoreIface, agentID, label string) *store.Session {
	for _, s := range st.ListSessions() {
		if s.AgentID == agentID && s.Label == label {
			return s
		}
	}
	sess, _ := st.CreateSession(store.Session{AgentID: agentID, Label: label})
	return sess
}
