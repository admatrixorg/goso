// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Access is the scope a write stamps on a doc. The zero value keeps the scope
// of an existing doc and makes a new doc tenant-wide.
type Access struct {
	Scope       string `json:"scope,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	TeamID      string `json:"team_id,omitempty"`
	CustomerKey string `json:"customer_key,omitempty"`
}

// IsZero reports an access that changes nothing.
func (a Access) IsZero() bool {
	return strings.TrimSpace(a.Scope) == "" && strings.TrimSpace(a.AgentID) == "" &&
		strings.TrimSpace(a.TeamID) == "" && strings.TrimSpace(a.CustomerKey) == ""
}

// ErrAccessOwner means agent_id or team_id is not in the request's tenant.
var ErrAccessOwner = errors.New("vault scope owner not in tenant")

// ResolveAccess validates a write's scope for tenant. The agent may be given
// by id or agent_key; it is stored by id. An agent or team of another tenant,
// or one that does not exist, is ErrAccessOwner. A bad scope is
// store.ErrVaultScope. Both are client errors.
func ResolveAccess(st store.StoreIface, tenant string, a Access) (Access, error) {
	if a.IsZero() {
		return Access{}, nil
	}
	ck := strings.TrimSpace(a.CustomerKey)
	if strings.TrimSpace(a.Scope) == "" && strings.TrimSpace(a.AgentID) == "" && strings.TrimSpace(a.TeamID) == "" {
		return Access{CustomerKey: ck}, nil
	}
	scope, agentID, teamID, err := store.NormalizeVaultAccess(a.Scope, a.AgentID, a.TeamID)
	if err != nil {
		return Access{}, err
	}
	tenant = store.NormalizeTenant(tenant)
	if agentID != "" {
		ag := findAgent(st, tenant, agentID)
		if ag == nil {
			return Access{}, fmt.Errorf("%w: agent %q", ErrAccessOwner, agentID)
		}
		agentID = ag.ID
	}
	if teamID != "" {
		t, err := st.GetTeam(teamID)
		if err != nil || t == nil || !store.SameTenant(t.TenantID, tenant) {
			return Access{}, fmt.Errorf("%w: team %q", ErrAccessOwner, teamID)
		}
		teamID = t.ID
	}
	return Access{Scope: scope, AgentID: agentID, TeamID: teamID, CustomerKey: ck}, nil
}

func findAgent(st store.StoreIface, tenant, ref string) *store.Agent {
	if ag, err := st.GetAgent(ref); err == nil && ag != nil {
		if store.SameTenant(ag.TenantID, tenant) {
			return ag
		}
		return nil
	}
	return store.FindAgentByKey(st, tenant, ref)
}

// TeamsOfAgent lists the ids of tenant teams the agent leads or belongs to.
func TeamsOfAgent(st store.StoreIface, tenant, agentID string) []string {
	agentID = strings.TrimSpace(agentID)
	if st == nil || agentID == "" {
		return nil
	}
	tenant = store.NormalizeTenant(tenant)
	seen := map[string]struct{}{}
	for _, t := range st.ListTeams() {
		if t == nil || !store.SameTenant(t.TenantID, tenant) {
			continue
		}
		member := t.LeadAgentID == agentID
		if !member {
			ms, err := st.ListTeamMembers(t.ID)
			if err != nil {
				continue
			}
			for _, m := range ms {
				if m != nil && m.AgentID == agentID {
					member = true
					break
				}
			}
		}
		if member {
			seen[t.ID] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// ViewerForAgent is the vault view of one agent. A channel thread adds the
// customer rule. An agent of another tenant gets no agent or team docs.
func ViewerForAgent(st store.StoreIface, tenant, agentRef string, channel bool, customerKey string) store.VaultViewer {
	v := store.VaultViewer{Channel: channel, CustomerKey: strings.TrimSpace(customerKey)}
	agentRef = strings.TrimSpace(agentRef)
	if st == nil || agentRef == "" {
		return v
	}
	tenant = store.NormalizeTenant(tenant)
	ag := findAgent(st, tenant, agentRef)
	if ag == nil {
		return v
	}
	v.AgentID = ag.ID
	v.TeamIDs = TeamsOfAgent(st, tenant, ag.ID)
	return v
}
