// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vaultimport

import (
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	reasonAgentUnmapped = "agent-unmapped"
	reasonTeamUnmapped  = "team-unmapped"
)

// docAccess is the goso scope a GoClaw document lands in.
type docAccess struct {
	scope   string
	agentID string
	teamID  string
}

func (a docAccess) same(d *store.VaultDoc) bool {
	scope := d.Scope
	if scope == "" {
		scope = store.VaultScopeTenant
	}
	return scope == a.scope && d.AgentID == a.agentID && d.TeamID == a.teamID
}

// mapAccess maps a GoClaw document's scope, agent_id and team_id onto goso.
// GoClaw treats a document with an agent_id as that agent's and one with a
// team_id as that team's. A missing mapping is a reason code, so access is
// never widened to the whole tenant.
func mapAccess(doc Document, opt Options) (docAccess, string) {
	agent := strings.TrimSpace(doc.AgentID)
	team := strings.TrimSpace(doc.TeamID)
	lookup := func(m map[string]string, id string) string {
		if m == nil {
			return ""
		}
		if v := strings.TrimSpace(m[id]); v != "" {
			return v
		}
		return strings.TrimSpace(m[strings.ToLower(id)])
	}
	scope := strings.ToLower(strings.TrimSpace(doc.Scope))
	if scope == "personal" {
		ref := lookup(opt.AgentMap, agent)
		if agent == "" || ref == "" {
			return docAccess{}, reasonAgentUnmapped
		}
		return docAccess{scope: store.VaultScopePersonal, agentID: ref}, ""
	}
	if scope == "team" && team != "" || scope != "team" && agent == "" && team != "" {
		ref := lookup(opt.TeamMap, team)
		if ref == "" {
			return docAccess{}, reasonTeamUnmapped
		}
		return docAccess{scope: store.VaultScopeTeam, teamID: ref}, ""
	}
	if agent != "" {
		ref := lookup(opt.AgentMap, agent)
		if ref == "" {
			return docAccess{}, reasonAgentUnmapped
		}
		return docAccess{scope: store.VaultScopeAgent, agentID: ref}, ""
	}
	return docAccess{scope: store.VaultScopeTenant}, ""
}

// ownedMaps drops map entries whose goso agent or team is missing or belongs
// to another tenant, so a bad map rejects documents instead of leaking them.
func ownedMaps(st store.StoreIface, dest string, agents, teams map[string]string) (map[string]string, map[string]string) {
	outA := map[string]string{}
	for k, v := range agents {
		v = strings.TrimSpace(v)
		if a, err := st.GetAgent(v); err == nil && a != nil && store.SameTenant(a.TenantID, dest) {
			outA[strings.TrimSpace(k)] = a.ID
		}
	}
	outT := map[string]string{}
	for k, v := range teams {
		v = strings.TrimSpace(v)
		if t, err := st.GetTeam(v); err == nil && t != nil && store.SameTenant(t.TenantID, dest) {
			outT[strings.TrimSpace(k)] = t.ID
		}
	}
	return outA, outT
}
