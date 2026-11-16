// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vaultimport

import (
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func seedAgent(t *testing.T, st store.StoreIface, key string) string {
	t.Helper()
	a, err := st.CreateAgent(store.Agent{AgentKey: key, DisplayName: "Minh An", TenantID: destTenant})
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func TestImportCarriesAgentAndTeamScope(t *testing.T) {
	st := store.New()
	agentX := seedAgent(t, st, "minh-an-x")
	team, err := st.CreateTeam(store.Team{Name: "Ban hang", LeadAgentID: agentX, TenantID: destTenant})
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateAgent(store.Agent{AgentKey: "khac-tenant", DisplayName: "Khac", TenantID: "tenant-khac"})
	if err != nil {
		t.Fatal(err)
	}
	ex := &Export{Documents: []Document{
		{ID: "d-agent", TenantID: sourceTenant, Scope: "shared", AgentID: "gc-agent-x", Path: "tenants/minh-an/notes/agent.md", Title: "Agent doc", UpdatedAt: "2026-03-01T04:05:06Z"},
		{ID: "d-team", TenantID: sourceTenant, Scope: "team", TeamID: "gc-team", Path: "tenants/minh-an/notes/team.md", Title: "Team doc", UpdatedAt: "2026-03-01T04:05:06Z"},
		{ID: "d-tenant", TenantID: sourceTenant, Scope: "shared", Path: "tenants/minh-an/notes/chung.md", Title: "Chung", UpdatedAt: "2026-03-01T04:05:06Z"},
		{ID: "d-unmapped-agent", TenantID: sourceTenant, Scope: "custom", AgentID: "gc-agent-lost", Path: "tenants/minh-an/notes/lost.md", Title: "Lost", UpdatedAt: "2026-03-01T04:05:06Z"},
		{ID: "d-unmapped-team", TenantID: sourceTenant, Scope: "team", TeamID: "gc-team-lost", Path: "tenants/minh-an/notes/lost-team.md", Title: "Lost team", UpdatedAt: "2026-03-01T04:05:06Z"},
		{ID: "d-foreign-agent", TenantID: sourceTenant, Scope: "shared", AgentID: "gc-agent-foreign", Path: "tenants/minh-an/notes/foreign.md", Title: "Foreign", UpdatedAt: "2026-03-01T04:05:06Z"},
	}}
	for _, d := range ex.Documents {
		ex.Versions = append(ex.Versions, Version{DocID: d.ID, Version: 1, Content: "noi dung " + d.ID, CreatedAt: "2026-03-01T04:05:06Z"})
	}
	opt := Options{
		SourceTenant: sourceTenant, Root: tenantRoot, DestTenant: destTenant, Log: discardLog{},
		AgentMap: map[string]string{"gc-agent-x": agentX, "gc-agent-foreign": other.ID},
		TeamMap:  map[string]string{"gc-team": team.ID},
	}
	rep, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Inserted != 3 || rep.Rejected != 3 {
		t.Fatalf("counts %+v", rep)
	}
	for path, reason := range map[string]string{
		"tenants/minh-an/notes/lost.md":      reasonAgentUnmapped,
		"tenants/minh-an/notes/lost-team.md": reasonTeamUnmapped,
		"tenants/minh-an/notes/foreign.md":   reasonAgentUnmapped,
	} {
		if !hasReason(rep, "doc", path, reason) {
			t.Fatalf("%s: want %s in %+v", path, reason, rep.Rejections)
		}
	}
	check := func(path, scope, agent, teamID string) {
		t.Helper()
		d, err := st.FindVaultDocByPath(destTenant, path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if d.Scope != scope || d.AgentID != agent || d.TeamID != teamID {
			t.Fatalf("%s: scope=%q agent=%q team=%q", path, d.Scope, d.AgentID, d.TeamID)
		}
	}
	check("notes/agent.md", store.VaultScopeAgent, agentX, "")
	check("notes/team.md", store.VaultScopeTeam, "", team.ID)
	check("notes/chung.md", store.VaultScopeTenant, "", "")

	// A second run is a no-op: the scope fields are part of "unchanged".
	again, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	if again.Inserted != 0 || again.Updated != 0 || again.Skipped != 3 {
		t.Fatalf("rerun %+v", again)
	}
}
