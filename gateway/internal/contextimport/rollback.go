// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package contextimport

import (
	"errors"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// RollbackReport is the rollback summary. Deleted counts context files removed.
// Kept counts mapped files whose body changed after import; those stay.
type RollbackReport struct {
	DryRun        bool `json:"dry_run"`
	Deleted       int  `json:"deleted"`
	Kept          int  `json:"kept"`
	DeniedTenants int  `json:"denied_tenants"`
}

// Rollback removes mapped context files for dest whose body still matches the
// imported sum. A file that is not in the map is left in place. A second run
// deletes nothing. Apply false only counts.
func Rollback(st store.StoreIface, dest, denyFile string, apply bool) (*RollbackReport, error) {
	ids, err := loadDenylist(denyFile)
	if err != nil {
		return nil, err
	}
	dest = strings.TrimSpace(dest)
	if !store.TenantOK(dest) {
		return nil, errConfig("tenant is required")
	}
	if n := deniedCount(ids, dest); n > 0 {
		return &RollbackReport{DryRun: !apply, DeniedTenants: n}, &DenyError{Count: n}
	}
	if st == nil {
		return nil, errConfig("store is required")
	}
	maps, err := openMapper(st, false)
	if err != nil {
		return nil, err
	}
	rows, err := maps.list(dest)
	if err != nil {
		return nil, err
	}
	rep := &RollbackReport{DryRun: !apply}
	for _, row := range rows {
		f, err := st.GetContextFile(dest, row.Scope, row.Owner, row.Name)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				if apply {
					if err := maps.del(row.DestTenant, row.SourceTable, row.SourceKey); err != nil {
						return nil, err
					}
				}
				continue
			}
			return nil, err
		}
		if f == nil || bodySum(f.Body) != row.Sum || !store.SameTenant(f.TenantID, dest) {
			rep.Kept++
			continue
		}
		if !apply {
			rep.Deleted++
			continue
		}
		if err := st.DeleteContextFile(dest, row.Scope, row.Owner, row.Name); err != nil {
			return nil, err
		}
		if err := maps.del(row.DestTenant, row.SourceTable, row.SourceKey); err != nil {
			return nil, err
		}
		rep.Deleted++
	}
	return rep, nil
}
