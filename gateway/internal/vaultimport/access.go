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
		return docAccess{scope: personalScope(opt), agentID: ref}, ""
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

// GoClaw shows a personal row to its agent in every chat, whoever the user
// is; it does not filter personal rows by user or chat. goso agent scope is
// exactly that audience, so it is the default. PersonalScopeStrict keeps
// goso's personal scope instead, which also hides the doc in channel threads.
const (
	PersonalScopeAgent  = "agent"
	PersonalScopeStrict = "personal"
)

func personalScope(opt Options) string {
	if strings.EqualFold(strings.TrimSpace(opt.PersonalScope), PersonalScopeStrict) {
		return store.VaultScopePersonal
	}
	return store.VaultScopeAgent
}

func checkPersonalScope(s string) error {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", PersonalScopeAgent, PersonalScopeStrict:
		return nil
	}
	return errConfig("personal scope must be agent or personal")
}

// ownerLabel names a goso owner inside a qualified path: the agent key for
// an agent or personal doc, the team id for a team doc. Ids are the fallback
// when a key has no path-safe characters.
func ownerLabel(st store.StoreIface) func(docAccess) string {
	cache := map[string]string{}
	return func(a docAccess) string {
		switch a.scope {
		case store.VaultScopeAgent, store.VaultScopePersonal:
			if v, ok := cache[a.agentID]; ok {
				return v
			}
			label := pathToken(a.agentID)
			if ag, err := st.GetAgent(a.agentID); err == nil && ag != nil {
				if k := pathToken(ag.AgentKey); k != "" {
					label = k
				}
			}
			cache[a.agentID] = label
			return label
		case store.VaultScopeTeam:
			return pathToken(a.teamID)
		}
		return ""
	}
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
