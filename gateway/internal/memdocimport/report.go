// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memdocimport

import "fmt"

// Report is one dry-run or import. Counts are per source tenant and per table.
// For each table, read = will_write + the reason counters.
// will_write is how many memory_documents rows were inserted, or would be
// inserted when dry_run is set. A second run reports will_write 0.
// system_names is set only on a dry-run, and only the file names of _system/
// rows whose user_id is empty. Bodies are not in this report.
type Report struct {
	DryRun   bool           `json:"dry_run"`
	Inserted int            `json:"inserted"`
	Sessions int            `json:"sessions"`
	Denied   int            `json:"denied"`
	Tenants  []TenantReport `json:"tenants"`
}

// TenantReport is one GoClaw tenant id.
type TenantReport struct {
	Source   string                 `json:"source"`
	Slug     string                 `json:"slug,omitempty"`
	Dest     string                 `json:"dest,omitempty"`
	Tables   map[string]*TableCount `json:"tables"`
	Sessions int                    `json:"sessions"`
	// SystemNames is not part of the JSON report. The command writes it to
	// --system-names-out. Stdout stays counts only.
	SystemNames []string `json:"-"`
}

// TableCount is memory_documents or memory_chunks.
type TableCount struct {
	Read      int      `json:"read"`
	WillWrite int      `json:"will_write"`
	Vault     int      `json:"vault,omitempty"`
	Durable   int      `json:"durable,omitempty"`
	Skipped   *Reasons `json:"skipped"`
}

// Reasons are the skip counters. expired, customer-private, and
// missing-relation-endpoint stay at 0 for this table: documents do not expire,
// a non-empty user_id is imported as a durable note, and this import has no
// graph edges. team/scope stays as a guard even though those columns are empty
// in the measured export.
type Reasons struct {
	Expired         int `json:"expired"`
	CustomerPrivate int `json:"customer-private"`
	TeamScope       int `json:"team/scope"`
	MQ              int `json:"mq"`
	Unmapped        int `json:"unmapped"`
	MissingEndpoint int `json:"missing-relation-endpoint"`
	System          int `json:"system"`
	Path            int `json:"path"`
	Chunks          int `json:"chunks"`
	Hash            int `json:"hash"`
	NativeExists    int `json:"native-exists"`
	OtherTenant     int `json:"other-tenant"`
	Binary          int `json:"binary"`
	TooLarge        int `json:"too-large"`
}

func (r *Reasons) sum() int {
	if r == nil {
		return 0
	}
	return r.Expired + r.CustomerPrivate + r.TeamScope + r.MQ + r.Unmapped +
		r.MissingEndpoint + r.System + r.Path + r.Chunks + r.Hash +
		r.NativeExists + r.OtherTenant + r.Binary + r.TooLarge
}

func (t *TableCount) balanced() bool {
	if t == nil || t.Skipped == nil {
		return false
	}
	return t.Read == t.WillWrite+t.Skipped.sum() && t.WillWrite == t.Vault+t.Durable
}

func checkReport(rep *Report) error {
	if rep == nil {
		return fmt.Errorf("memdocimport: report is missing")
	}
	inserted := 0
	sessions := 0
	for _, tenant := range rep.Tenants {
		inserted += tenant.Tables[tableDocs].WillWrite
		sessions += tenant.Sessions
		for _, name := range []string{tableDocs, tableChunks} {
			tab := tenant.Tables[name]
			if !tab.balanced() {
				return fmt.Errorf("memdocimport: %s tally unbalanced", name)
			}
		}
	}
	if inserted != rep.Inserted || sessions != rep.Sessions {
		return fmt.Errorf("memdocimport: report totals do not match per-tenant sections")
	}
	return nil
}

const (
	tableDocs   = "memory_documents"
	tableChunks = "memory_chunks"
)

func newTenant(source, slug, dest string) *TenantReport {
	return &TenantReport{
		Source: source,
		Slug:   slug,
		Dest:   dest,
		Tables: map[string]*TableCount{
			tableDocs:   {Skipped: &Reasons{}},
			tableChunks: {Skipped: &Reasons{}},
		},
	}
}
