// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package mcpimport

import (
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Deleter removes one connector and the agent links that point at it.
type Deleter interface {
	Store
	DeleteConnector(name string) error
}

// RollbackReport counts import-map rows for one goso tenant. It does not
// name connectors.
type RollbackReport struct {
	Rollback      bool `json:"rollback"`
	DryRun        bool `json:"dry_run"`
	Connectors    int  `json:"connectors"`
	Links         int  `json:"links"`
	DeniedTenants int  `json:"denied_tenants"`
}

// Rollback deletes connectors whose schema_version marker is
// goclaw-src:<tenant>: and the agent links that point at them.
// Dry-run counts those rows and does not delete. A native connector and a
// connector mapped to another tenant are left in place.
func Rollback(db Deleter, opt Options) (RollbackReport, error) {
	dest := strings.TrimSpace(opt.Tenant)
	if dest == "" {
		return RollbackReport{}, errConfig("tenant is required")
	}
	if !store.TenantOK(dest) {
		return RollbackReport{}, errConfig("tenant is invalid")
	}
	if len(opt.Deny) == 0 {
		return RollbackReport{}, errConfig("deny file")
	}
	if n := deniedFlags(opt.Deny, dest); n > 0 {
		return RollbackReport{Rollback: true, DryRun: opt.DryRun, DeniedTenants: n}, errConfig("tenant is denied")
	}
	if db == nil {
		return RollbackReport{}, errConfig("store is required")
	}
	prefix := sourceMarker(dest, "")
	targets := map[string]struct{}{}
	for _, c := range db.ListConnectors() {
		if c == nil || !strings.HasPrefix(c.SchemaVersion, prefix) {
			continue
		}
		targets[c.Name] = struct{}{}
	}
	links := 0
	for _, a := range db.ListAgents() {
		if a == nil {
			continue
		}
		names, err := db.ListAgentConnectors(a.ID)
		if err != nil {
			continue
		}
		for _, name := range names {
			if _, ok := targets[name]; ok {
				links++
			}
		}
	}
	rep := RollbackReport{
		Rollback:   true,
		DryRun:     opt.DryRun,
		Connectors: len(targets),
		Links:      links,
	}
	if opt.DryRun {
		return rep, nil
	}
	for name := range targets {
		if err := db.DeleteConnector(name); err != nil {
			return RollbackReport{}, errConfig("rollback failed")
		}
	}
	return rep, nil
}

var (
	_ Deleter = (*store.Store)(nil)
	_ Deleter = (*store.SQLiteStore)(nil)
	_ Deleter = (*store.PostgresStore)(nil)
)
