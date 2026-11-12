// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agentimport

import (
	"fmt"
	"sort"
)

const (
	actionInserted = "inserted"
	actionSkipped  = "skipped"
	actionRejected = "rejected"

	reasonUnchanged        = "unchanged"
	reasonTenantMismatch   = "tenant-mismatch"
	reasonDuplicate        = "duplicate"
	reasonDeleted          = "deleted"
	reasonInactive         = "inactive"
	reasonNeedsDecision    = "needs-decision"
	reasonNotThisSpec      = "not-this-spec"
	reasonAgentKeyInvalid  = "agent-key-invalid"
	reasonAgentKeyTaken    = "agent-key-taken"
	reasonIDMissing        = "id-missing"
	reasonTenantMissing    = "tenant-missing"
	reasonBadValue         = "bad-value"
	reasonTooLong          = "too-long"
	reasonTooLarge         = "too-large"
	reasonStatusUnknown    = "status-unknown"
	reasonNameMissing      = "name-missing"
	reasonLeadMissing      = "lead-missing"
	reasonTeamMissing      = "team-missing"
	reasonAgentNotImported = "agent-not-imported"
	reasonRoleUnsupported  = "role-unsupported"
	reasonRoleDiffers      = "role-differs"
	reasonLeadDiffers      = "lead-differs"
	reasonDirectionUnknown = "direction-unknown"
	reasonSelfLink         = "self-link"
	reasonEndpointMissing  = "endpoint-not-imported"
	reasonLinkExists       = "link-exists"

	tableAgents      = "agents"
	tableTeams       = "agent_teams"
	tableMembers     = "agent_team_members"
	tableLinks       = "agent_links"
	tablePermissions = "agent_config_permissions"
	tableOverrides   = "user_agent_overrides"
	tableProfiles    = "user_agent_profiles"
	tableShares      = "agent_shares"
)

// ConfigError is an operator mistake. The text has no row body.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string {
	if e == nil {
		return ""
	}
	return e.Msg
}

// Section is one source tenant inside an imported table.
// Input = Inserted + Updated + Skipped + Rejected.
// Updated is always 0. Reasons sum to Skipped + Rejected.
type Section struct {
	SourceTenant string         `json:"source_tenant"`
	Input        int            `json:"input"`
	Inserted     int            `json:"inserted"`
	Updated      int            `json:"updated"`
	Skipped      int            `json:"skipped"`
	Rejected     int            `json:"rejected"`
	Reasons      map[string]int `json:"reasons,omitempty"`
}

// Held is a table this importer does not write, counted per source tenant.
type Held struct {
	Table        string `json:"table"`
	SourceTenant string `json:"source_tenant"`
	Input        int    `json:"input"`
	Reason       string `json:"reason"`
}

// Removed is what a rollback deleted, or would delete when dry_run is true.
// agent_links counts goso edges. A bidirectional export row is two edges.
type Removed struct {
	Agents      int `json:"agents"`
	Teams       int `json:"teams"`
	TeamMembers int `json:"team_members"`
	AgentLinks  int `json:"agent_links"`
}

// Report is the only stdout of the command. It has counts and reason codes.
type Report struct {
	DryRun       bool      `json:"dry_run"`
	Rollback     bool      `json:"rollback"`
	SourceTenant string    `json:"source_tenant,omitempty"`
	DestTenant   string    `json:"dest_tenant"`
	Agents       []Section `json:"agents,omitempty"`
	Teams        []Section `json:"teams,omitempty"`
	TeamMembers  []Section `json:"team_members,omitempty"`
	AgentLinks   []Section `json:"agent_links,omitempty"`
	NotImported  []Held    `json:"not_imported,omitempty"`
	Removed      *Removed  `json:"removed,omitempty"`
}

type outcome struct {
	table  string
	tenant string
	action string
	reason string
}

type tally struct {
	rows []outcome
	held []Held
}

func (t *tally) add(table, tenant, action, reason string) {
	t.rows = append(t.rows, outcome{table: table, tenant: tenant, action: action, reason: reason})
}

func (t *tally) hold(table, tenant, reason string) {
	for i := range t.held {
		if t.held[i].Table == table && t.held[i].SourceTenant == tenant && t.held[i].Reason == reason {
			t.held[i].Input++
			return
		}
	}
	t.held = append(t.held, Held{Table: table, SourceTenant: tenant, Input: 1, Reason: reason})
}

func (t *tally) report(base Report) (*Report, error) {
	base.Agents = t.sections(tableAgents)
	base.Teams = t.sections(tableTeams)
	base.TeamMembers = t.sections(tableMembers)
	base.AgentLinks = t.sections(tableLinks)
	base.NotImported = append([]Held(nil), t.held...)
	sort.Slice(base.NotImported, func(i, j int) bool {
		if base.NotImported[i].Table != base.NotImported[j].Table {
			return base.NotImported[i].Table < base.NotImported[j].Table
		}
		return base.NotImported[i].SourceTenant < base.NotImported[j].SourceTenant
	})
	if err := checkSections("agents", base.Agents); err != nil {
		return nil, err
	}
	if err := checkSections("teams", base.Teams); err != nil {
		return nil, err
	}
	if err := checkSections("team_members", base.TeamMembers); err != nil {
		return nil, err
	}
	if err := checkSections("agent_links", base.AgentLinks); err != nil {
		return nil, err
	}
	return &base, nil
}

func (t *tally) sections(table string) []Section {
	by := map[string]*Section{}
	var order []string
	for _, row := range t.rows {
		if row.table != table {
			continue
		}
		s := by[row.tenant]
		if s == nil {
			s = &Section{SourceTenant: row.tenant, Reasons: map[string]int{}}
			by[row.tenant] = s
			order = append(order, row.tenant)
		}
		s.Input++
		switch row.action {
		case actionInserted:
			s.Inserted++
		case actionSkipped:
			s.Skipped++
			if row.reason != "" {
				s.Reasons[row.reason]++
			}
		case actionRejected:
			s.Rejected++
			if row.reason != "" {
				s.Reasons[row.reason]++
			}
		}
	}
	sort.Strings(order)
	out := make([]Section, 0, len(order))
	for _, tenant := range order {
		s := *by[tenant]
		if len(s.Reasons) == 0 {
			s.Reasons = nil
		}
		out = append(out, s)
	}
	return out
}

func checkSections(kind string, sections []Section) error {
	for _, s := range sections {
		if s.Updated != 0 || s.Input != s.Inserted+s.Updated+s.Skipped+s.Rejected {
			return fmt.Errorf("agentimport: %s tally unbalanced", kind)
		}
		n := 0
		for _, c := range s.Reasons {
			if c < 0 {
				return fmt.Errorf("agentimport: %s tally unbalanced", kind)
			}
			n += c
		}
		if n != s.Skipped+s.Rejected {
			return fmt.Errorf("agentimport: %s tally unbalanced", kind)
		}
	}
	return nil
}

func sortStrings(v []string) { sort.Strings(v) }
