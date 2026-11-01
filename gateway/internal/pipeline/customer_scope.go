// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// toolScope is the memory/session visibility of the session that called a
// memory_* or sessions_* tool. Web/API callers (and calls with no session)
// see the whole tenant, as before. Channel callers see only sessions and notes
// of their own agent + customer key, plus curated (non-extracted) KG entities.
type toolScope struct {
	channel  bool
	agentID  string
	customer string
}

func scopeForCaller(st store.StoreIface, callerSessionID string) toolScope {
	id := strings.TrimSpace(callerSessionID)
	if st == nil || id == "" {
		return toolScope{}
	}
	sess, err := st.GetSession(id)
	if err != nil || sess == nil || !IsChannelSession(sess) {
		return toolScope{}
	}
	return toolScope{channel: true, agentID: sess.AgentID, customer: CustomerKey(sess)}
}

// sessionVisible reports whether a channel caller may read session sess.
// The customer key is the label, so this matches the stamping rule.
func (sc toolScope) sessionVisible(sess *store.Session) bool {
	if !sc.channel {
		return true
	}
	return sess != nil && sc.customer != "" && sess.AgentID == sc.agentID &&
		CustomerKey(sess) == sc.customer
}

// hitVisible filters one memory_search hit for a channel caller. Fail closed:
// anything that cannot be attributed to this customer is dropped.
func (sc toolScope) hitVisible(st store.StoreIface, h store.KGSearchHit) bool {
	if !sc.channel {
		return true
	}
	if sc.customer == "" {
		return false
	}
	if h.Tier == store.TierL2 {
		e, err := st.GetKGEntity(h.ID)
		return err == nil && e != nil && e.Source != store.KGSourceExtracted
	}
	sess, err := st.GetSession(h.SessionID)
	if err != nil || !sc.sessionVisible(sess) {
		return false
	}
	if h.Kind == store.KindMessage {
		return true
	}
	// Notes carry their own stamp; it wins over the (mutable) session label.
	m, err := st.GetMemory(h.ID)
	return err == nil && m != nil && m.CustomerKey == sc.customer
}
