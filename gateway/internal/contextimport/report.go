// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package contextimport

import (
	"fmt"
	"sort"
	"strings"
)

// Report is the run summary. Inserted, updated, skipped, and rejected count
// export rows. Agents and Users split those rows by the tenant_id on the
// row. For each section, input = inserted + updated + skipped + rejected,
// and the reason counts sum to skipped + rejected.
type Report struct {
	Inserted   int             `json:"inserted"`
	Updated    int             `json:"updated"`
	Skipped    int             `json:"skipped"`
	Rejected   int             `json:"rejected"`
	DryRun     bool            `json:"dry_run"`
	Rejections []Rejection     `json:"rejections,omitempty"`
	Agents     []TenantSection `json:"agents"`
	Users      []TenantSection `json:"users"`
}

// Rejection is one dropped row. Name and Owner are the file name and the
// agent_key or user_key. Body is not included.
type Rejection struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Owner  string `json:"owner"`
	Reason string `json:"reason"`
}

// TenantSection is one source tenant inside the agent or user section.
type TenantSection struct {
	SourceTenant string         `json:"source_tenant"`
	Input        int            `json:"input"`
	Inserted     int            `json:"inserted"`
	Updated      int            `json:"updated"`
	Skipped      int            `json:"skipped"`
	Rejected     int            `json:"rejected"`
	Reasons      map[string]int `json:"reasons,omitempty"`
}

func (s TenantSection) balanced() bool {
	if s.Input != s.Inserted+s.Updated+s.Skipped+s.Rejected {
		return false
	}
	n := 0
	for _, c := range s.Reasons {
		if c < 0 {
			return false
		}
		n += c
	}
	return n == s.Skipped+s.Rejected
}

func (r *Report) check() error {
	if r == nil {
		return fmt.Errorf("contextimport: report is missing")
	}
	if err := checkSections("agent", r.Agents); err != nil {
		return err
	}
	if err := checkSections("user", r.Users); err != nil {
		return err
	}
	ins, upd, skp, rej := 0, 0, 0, 0
	for _, s := range r.Agents {
		ins += s.Inserted
		upd += s.Updated
		skp += s.Skipped
		rej += s.Rejected
	}
	for _, s := range r.Users {
		ins += s.Inserted
		upd += s.Updated
		skp += s.Skipped
		rej += s.Rejected
	}
	if r.Inserted != ins || r.Updated != upd || r.Skipped != skp || r.Rejected != rej {
		return fmt.Errorf("contextimport: report totals do not match per-tenant sections")
	}
	return nil
}

func checkSections(kind string, sections []TenantSection) error {
	for _, s := range sections {
		if !s.balanced() {
			return fmt.Errorf("contextimport: %s tally unbalanced", kind)
		}
	}
	return nil
}

type rowOutcome struct {
	kind   string
	tenant string
	action string
	reason string
	name   string
	owner  string
}

type tally struct {
	log  Logger
	rows []rowOutcome
}

func (t *tally) add(kind, tenant, action, reason, name, owner string) {
	t.rows = append(t.rows, rowOutcome{
		kind:   kind,
		tenant: strings.TrimSpace(tenant),
		action: action,
		reason: reason,
		name:   name,
		owner:  owner,
	})
}

func (t *tally) skip(kind, tenant, name, owner, reason string) {
	t.add(kind, tenant, "skipped", reason, name, owner)
}

func (t *tally) reject(kind, tenant, name, owner, reason string) {
	t.add(kind, tenant, "rejected", reason, name, owner)
	if t.log == nil {
		return
	}
	t.log.Printf("contextimport: rejected %s name=%q owner=%q reason=%s", kind, name, owner, reason)
}

func (t *tally) abort(dry bool, err error) (*Report, error) {
	rep, ferr := t.finish(dry, -1, -1)
	if ferr != nil {
		return rep, ferr
	}
	return rep, err
}

func (t *tally) finish(dry bool, agents, users int) (*Report, error) {
	rep := &Report{DryRun: dry, Agents: []TenantSection{}, Users: []TenantSection{}}
	agentN, userN := 0, 0
	for _, row := range t.rows {
		switch row.action {
		case "inserted", "updated", "skipped", "rejected":
		default:
			return rep, fmt.Errorf("contextimport: outcome action")
		}
		switch row.kind {
		case "agent":
			agentN++
		case "user":
			userN++
		default:
			return rep, fmt.Errorf("contextimport: outcome kind")
		}
		if row.action == "rejected" {
			rep.Rejections = append(rep.Rejections, Rejection{
				Kind: row.kind, Name: row.name, Owner: row.owner, Reason: row.reason,
			})
		}
	}
	if agents >= 0 && agentN != agents {
		return rep, fmt.Errorf("contextimport: agent outcomes %d != input %d", agentN, agents)
	}
	if users >= 0 && userN != users {
		return rep, fmt.Errorf("contextimport: user outcomes %d != input %d", userN, users)
	}
	rep.Agents = foldOutcomes(t.rows, "agent")
	rep.Users = foldOutcomes(t.rows, "user")
	for _, s := range rep.Agents {
		rep.Inserted += s.Inserted
		rep.Updated += s.Updated
		rep.Skipped += s.Skipped
		rep.Rejected += s.Rejected
	}
	for _, s := range rep.Users {
		rep.Inserted += s.Inserted
		rep.Updated += s.Updated
		rep.Skipped += s.Skipped
		rep.Rejected += s.Rejected
	}
	if err := rep.check(); err != nil {
		return rep, err
	}
	return rep, nil
}

func foldOutcomes(rows []rowOutcome, kind string) []TenantSection {
	var order []string
	by := map[string]*TenantSection{}
	for _, row := range rows {
		if row.kind != kind {
			continue
		}
		s := by[row.tenant]
		if s == nil {
			s = &TenantSection{SourceTenant: row.tenant}
			by[row.tenant] = s
			order = append(order, row.tenant)
		}
		s.Input++
		switch row.action {
		case "inserted":
			s.Inserted++
		case "updated":
			s.Updated++
		case "skipped":
			s.Skipped++
			addReason(s, row.reason)
		case "rejected":
			s.Rejected++
			addReason(s, row.reason)
		}
	}
	sort.Strings(order)
	out := make([]TenantSection, 0, len(order))
	for _, key := range order {
		out = append(out, *by[key])
	}
	return out
}

func addReason(s *TenantSection, reason string) {
	if reason == "" {
		return
	}
	if s.Reasons == nil {
		s.Reasons = map[string]int{}
	}
	s.Reasons[reason]++
}
