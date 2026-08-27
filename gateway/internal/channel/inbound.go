// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
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

func replyInbound(w http.ResponseWriter, r *http.Request, d inboundDeps, agentKey, displayName, dest, text string) {
	if dest == "" || text == "" {
		writeOK(w)
		return
	}
	agent := ensureNamedAgent(d.Store, agentKey, displayName)
	sess := ensureLabeledSession(d.Store, agent.ID, agentKey+":"+dest)

	_, _ = d.Store.AddMessage(store.Message{SessionID: sess.ID, Role: "user", Content: text})

	provider := d.LLM
	if provider == nil {
		provider = llm.Echo{}
	}
	history, _ := d.Store.ListMessages(sess.ID)
	var msgs []llm.Message
	for _, m := range history {
		msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
	}
	reply, usage, err := llm.ChatUsage(r.Context(), provider, msgs)
	if err != nil {
		reply = fmt.Sprintf("LLM error: %v", err)
	} else {
		trackUsage(d.Meter, agent.ID, provider.Name(), usage)
	}
	_, _ = d.Store.AddMessage(store.Message{SessionID: sess.ID, Role: "assistant", Content: reply})

	if d.Sender != nil {
		if err := d.Sender(r.Context(), dest, reply); err != nil {
			writeOKWarning(w, err.Error())
			return
		}
	}
	writeOK(w)
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
