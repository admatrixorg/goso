// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"net/http"

	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/tenant"
	"github.com/mqglobal/goso/gateway/internal/webhook"
)

func requestTenant(r *http.Request) string {
	return tenant.Resolve(r)
}

func hideWrongTenant(w http.ResponseWriter, rowTenant, want string) bool {
	if store.SameTenant(rowTenant, want) {
		return false
	}
	writeErr(w, http.StatusNotFound, "not found")
	return true
}

func agentsInTenant(list []*store.Agent, tid string) []*store.Agent {
	out := make([]*store.Agent, 0, len(list))
	for _, a := range list {
		if a != nil && store.SameTenant(a.TenantID, tid) {
			out = append(out, a)
		}
	}
	return out
}

func sessionsInTenant(list []*store.Session, tid string) []*store.Session {
	out := make([]*store.Session, 0, len(list))
	for _, s := range list {
		if s != nil && store.SameTenant(s.TenantID, tid) {
			out = append(out, s)
		}
	}
	return out
}

func teamsInTenant(list []*store.Team, tid string) []*store.Team {
	out := make([]*store.Team, 0, len(list))
	for _, t := range list {
		if t != nil && store.SameTenant(t.TenantID, tid) {
			out = append(out, t)
		}
	}
	return out
}

func vaultDocsInTenant(list []*store.VaultDoc, tid string) []*store.VaultDoc {
	out := make([]*store.VaultDoc, 0, len(list))
	for _, d := range list {
		if d != nil && store.SameTenant(d.TenantID, tid) {
			out = append(out, d)
		}
	}
	return out
}

func memoriesInTenant(list []*store.Memory, tid string) []*store.Memory {
	out := make([]*store.Memory, 0, len(list))
	for _, m := range list {
		if m != nil && store.SameTenant(m.TenantID, tid) {
			out = append(out, m)
		}
	}
	return out
}

func providersInTenant(list []*store.LLMProvider, tid string) []*store.LLMProvider {
	out := make([]*store.LLMProvider, 0, len(list))
	for _, p := range list {
		if p != nil && store.SameTenant(p.TenantID, tid) {
			out = append(out, p)
		}
	}
	return out
}

func webhooksInTenant(list []webhook.Public, tid string) []webhook.Public {
	out := make([]webhook.Public, 0, len(list))
	for _, p := range list {
		if store.SameTenant(p.TenantID, tid) {
			out = append(out, p)
		}
	}
	return out
}

func sessionVisible(st store.StoreIface, sid, tid string) (*store.Session, error) {
	sess, err := st.GetSession(sid)
	if err != nil {
		return nil, err
	}
	if !store.SameTenant(sess.TenantID, tid) {
		return nil, store.ErrNotFound
	}
	return sess, nil
}

func agentVisible(st store.StoreIface, id, tid string) (*store.Agent, error) {
	a, err := st.GetAgent(id)
	if err != nil {
		return nil, err
	}
	if !store.SameTenant(a.TenantID, tid) {
		return nil, store.ErrNotFound
	}
	return a, nil
}

func teamVisible(st store.StoreIface, id, tid string) (*store.Team, error) {
	t, err := st.GetTeam(id)
	if err != nil {
		return nil, err
	}
	if !store.SameTenant(t.TenantID, tid) {
		return nil, store.ErrNotFound
	}
	return t, nil
}
