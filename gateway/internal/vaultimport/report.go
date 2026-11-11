// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vaultimport

import (
	"fmt"
	"sort"
	"strings"
)

const (
	reasonUnchanged      = "unchanged"
	reasonTenantMismatch = "tenant-mismatch"
)

// TenantSection is one source tenant inside the doc or link section.
// Input is the number of export rows for that tenant.
// Input = Inserted + Updated + Skipped + Rejected.
// Reasons counts every skipped and rejected row once, by reason code.
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

// check reports a tally that does not add up. The error names the section,
// not a path or a document body.
func (r *Report) check() error {
	if r == nil {
		return fmt.Errorf("vaultimport: report is missing")
	}
	if err := checkSections("doc", r.Docs); err != nil {
		return err
	}
	if err := checkSections("link", r.Links); err != nil {
		return err
	}
	ins, upd, skp, rej := 0, 0, 0, 0
	for _, s := range r.Docs {
		ins += s.Inserted
		upd += s.Updated
		skp += s.Skipped
		rej += s.Rejected
	}
	for _, s := range r.Links {
		rej += s.Rejected
	}
	if r.Inserted != ins || r.Updated != upd || r.Skipped != skp || r.Rejected != rej {
		return fmt.Errorf("vaultimport: report totals do not match per-tenant sections")
	}
	return nil
}

func checkSections(kind string, sections []TenantSection) error {
	for _, s := range sections {
		if !s.balanced() {
			return fmt.Errorf("vaultimport: %s tally unbalanced", kind)
		}
	}
	return nil
}

type rowOutcome struct {
	kind   string
	tenant string
	action string
	reason string
	path   string
	target string
}

type tally struct {
	log  Logger
	rows []rowOutcome
}

func (t *tally) add(kind, tenant, action, reason, path, target string) {
	t.rows = append(t.rows, rowOutcome{
		kind:   kind,
		tenant: strings.TrimSpace(tenant),
		action: action,
		reason: reason,
		path:   path,
		target: target,
	})
	switch action {
	case "rejected":
		if target != "" {
			t.log.Printf("vaultimport: rejected %s path=%q target=%q reason=%s", kind, path, target, reason)
			return
		}
		t.log.Printf("vaultimport: rejected %s path=%q reason=%s", kind, path, reason)
	case "skipped":
		if reason == reasonTenantMismatch {
			t.log.Printf("vaultimport: skipped doc path=%q reason=%s", path, reason)
		}
	}
}

func (t *tally) reject(kind, tenant, path, target, reason string) {
	t.add(kind, tenant, "rejected", reason, path, target)
}

// abort returns a partial report and err. A tally error is not discarded.
func (t *tally) abort(dry bool, err error) (*Report, error) {
	rep, ferr := t.finish(dry, -1, -1)
	if ferr != nil {
		return rep, ferr
	}
	return rep, err
}

// finish builds the report. docs and links are the export lengths; negative
// skips that check for an aborted run.
func (t *tally) finish(dry bool, docs, links int) (*Report, error) {
	rep := &Report{DryRun: dry, Docs: []TenantSection{}, Links: []TenantSection{}}
	docN, linkN := 0, 0
	for _, row := range t.rows {
		switch row.action {
		case "inserted", "updated", "skipped", "rejected":
		default:
			return rep, fmt.Errorf("vaultimport: outcome action")
		}
		switch row.kind {
		case "doc":
			docN++
		case "link":
			linkN++
		default:
			return rep, fmt.Errorf("vaultimport: outcome kind")
		}
		if row.action == "rejected" {
			rep.Rejections = append(rep.Rejections, Rejection{
				Kind: row.kind, Path: row.path, Target: row.target, Reason: row.reason,
			})
		}
	}
	if docs >= 0 && docN != docs {
		return rep, fmt.Errorf("vaultimport: doc outcomes %d != input %d", docN, docs)
	}
	if links >= 0 && linkN != links {
		return rep, fmt.Errorf("vaultimport: link outcomes %d != input %d", linkN, links)
	}
	rep.Docs = foldOutcomes(t.rows, "doc")
	rep.Links = foldOutcomes(t.rows, "link")
	for _, s := range rep.Docs {
		rep.Inserted += s.Inserted
		rep.Updated += s.Updated
		rep.Skipped += s.Skipped
		rep.Rejected += s.Rejected
	}
	for _, s := range rep.Links {
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
